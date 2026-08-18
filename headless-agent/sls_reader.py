"""SLS 只读数据采集（Hivemind headless-agent）。

通过本机 aliyun CLI（已配置 AK/SK）执行只读查询：
  - 事故记录（itsm_incident_record_log）
  - 告警事件（alertmanager-alarm-event）
  - 应用日志（goodputai-dev 各 logstore）

安全：本模块只调用 List/Get 类只读 API，绝不执行任何写操作。
"""
import json
import subprocess
from dataclasses import dataclass, field

REGION = "cn-hangzhou"

# 环境内已知项目（探测自 2026-08-13 只读盘点）
ITSM_PROJECT = "itsm-1987933858527387-cn-hangzhou"
CMS_ALERT_PROJECT = "cms-alert-center-1987933858527387-cn-hangzhou"
APP_PROJECT = "goodputai-dev"
K8S_PROJECT = "k8s-log-c017ea72526f14a22b83a2d8a0ec3f7e6"
# k8s 各 logstore 名带集群后缀（ListLogStores 实测）；用于补齐"Pod 驱逐/调度失败/
# OOMKilled"等 k8s 层根因证据，补足此前诊断一直缺的"集群层面证据盲区"。
K8S_CLUSTER_SUFFIX = "c017ea72526f14a22b83a2d8a0ec3f7e6"


def _clean(s: str) -> str:
    """去除 aliyun CLI 的 ANSI 颜色码。"""
    import re

    return re.sub(r"\x1b\[[0-9;]*m", "", s)


def _run(args: list[str], timeout: int = 60) -> str:
    r = subprocess.run(
        ["aliyun", *args, "--region", REGION],
        capture_output=True,
        text=True,
        timeout=timeout,
    )
    if r.returncode != 0:
        raise RuntimeError(f"aliyun CLI failed: {_clean(r.stderr)[:300]}")
    return _clean(r.stdout)


@dataclass
class SLSLog:
    """一条 SLS 日志（展开字段，剔除系统元数据）。"""

    time: str
    fields: dict = field(default_factory=dict)

    @property
    def summary(self) -> str:
        return json.dumps(self.fields, ensure_ascii=False)[:400]


def _get_logs(project: str, logstore: str, hours: int, query: str = "*", line: int = 5) -> list[SLSLog]:
    import time

    now = int(time.time())
    raw = _run(["sls", "GetLogs", "--project", project, "--logstore", logstore,
                "--from", str(now - hours * 3600), "--to", str(now),
                "--query", query, "--line", str(line), "--offset", "0"])
    try:
        rows = json.loads(raw)
    except json.JSONDecodeError:
        return []
    out = []
    for r in rows:
        fields = {k: v for k, v in r.items() if not k.startswith("__")}
        out.append(SLSLog(time=r.get("__time__", ""), fields=fields))
    return out


def list_incident_records(hours: int = 168, line: int = 5) -> list[dict]:
    """最近事故记录（itsm_incident_record_log），解析 content JSON。"""
    logs = _get_logs(ITSM_PROJECT, "itsm_incident_record_log", hours, line=line)
    out = []
    for lg in logs:
        content = lg.fields.get("content", "")
        try:
            rec = json.loads(content)
        except json.JSONDecodeError:
            rec = {"raw": content[:300]}
        out.append({"time": lg.time, "record": rec})
    return out


def list_alerts(hours: int = 168, line: int = 5) -> list[dict]:
    """最近告警事件（alertmanager-alarm-event），解析 context JSON。"""
    logs = _get_logs(ITSM_PROJECT, "alertmanager-alarm-event", hours, line=line)
    out = []
    for lg in logs:
        ctx = lg.fields.get("context", "")
        try:
            c = json.loads(ctx)
            alarm = c.get("alarm", {})
        except json.JSONDecodeError:
            alarm = {"raw": ctx[:300]}
        out.append({"time": lg.time, "alarm": alarm})
    return out


def app_logs(logstore: str, hours: int = 24, query: str = "*", line: int = 5,
             namespace: str = "", container: str = "") -> list[SLSLog]:
    """应用日志（goodputai-dev 各 logstore）。

    串房防护：SLS 的 k8s 日志用单下划线系统字段标记来源（_namespace_ /
    _container_name_ / _pod_name_）。一条诊断事故只该采自己环境/服务的日志，
    否则 prod 事故会把 dev + casdoor 的日志当证据，污染根因推理。

    - namespace: 只保留该 k8s namespace 的日志（如 "ultron-zjsl-cluster-prod"）。
    - container: 只保留该容器的日志（如 "main"，排除无关的 casdoor/auth 服务）。
    过滤在读取后做（SLS query 语法不稳，本机按字段穷举更可靠）。
    """
    logs = _get_logs(APP_PROJECT, logstore, hours, query=query, line=max(line, 60))
    out = []
    for lg in logs:
        if namespace and lg.fields.get("_namespace_", "") != namespace:
            continue
        if container and lg.fields.get("_container_name_", "") != container:
            continue
        out.append(lg)
    return out[:line]


def k8s_logs(component: str, hours: int = 24, query: str = "error OR warn OR fail OR BackOff OR Evict OR OOM", line: int = 5) -> list[SLSLog]:
    """k8s 控制面日志（apiserver/audit/scheduler/kcm/ccm）。

    logstore 名带集群后缀（实测 K8S_PROJECT 下有 apiserver-/audit-/scheduler-/kcm-/ccm-）。
    补足事故 k8s 层根因证据：Pod 驱逐、调度失败、OOMKilled、apiserver 拒绝等——
    这些是 inference-hang 类故障的关键旁证，此前诊断一直缺此层证据。
    logstore 不存在时优雅降级返回空，不抛异常中断诊断。
    """
    logstore = f"{component}-{K8S_CLUSTER_SUFFIX}"
    try:
        return _get_logs(K8S_PROJECT, logstore, hours, query=query, line=line)
    except RuntimeError:
        return []


def parse_cluster_env(alarm_name: str) -> str:
    """从 Alertmanager 告警名解析出 k8s namespace 对应的环境后缀。

    告警名形如 ultron-inference-hang-worker-zjsl-cluster-prod，
    末尾的 zjsl-cluster-{env} 即对应 SLS 日志里的 _namespace_
    ultron-zjsl-cluster-{env}。用来过滤应用日志，避免 dev/prod 串房。

    返回完整 namespace（如 "ultron-zjsl-cluster-prod"），解析不到返回空串。
    """
    import re

    m = re.search(r"((?:zjsl|center)-cluster-(?:prod|dev))", str(alarm_name))
    return f"ultron-{m.group(1)}" if m else ""


def cluster_env_from_alarm(alarm: dict) -> str:
    """从告警对象解析环境，优先读权威的 alarmObjects，降级正则告警名。

    alarmObjects[0].objectName 形如 "zjsl-cluster-prod"（阿里云告警的真实归属），
    比靠告警名后缀猜更可靠——像 ultron-delivery-test-* 这类无后缀告警名，
    alarmObjects 里仍带明确集群环境。两者取值一致时互为印证。
    """
    if not isinstance(alarm, dict):
        return ""
    objs = alarm.get("alarmObjects")
    if objs:
        try:
            arr = objs if isinstance(objs, list) else json.loads(objs)
            for o in arr:
                name = str(o.get("objectName", ""))
                env = parse_cluster_env(name)
                if env:
                    return env
        except (json.JSONDecodeError, AttributeError):
            pass
    return parse_cluster_env(alarm.get("alarmName", ""))


def collect_incident_context(incident: dict, hours: int = 24) -> dict:
    """围绕一条事故记录采集诊断上下文（只读）。

    串房防护：优先从事故关联告警名解析出集群环境，只采该环境的 gateway 日志。
    解析不到时退化为不过滤（宁可多采也不漏），但会在日志里标注以便 IC 甄别。
    """
    rec = incident.get("record", {})
    msg = str(rec.get("annotations", {}).get("message", ""))[:200]
    # 尝试从关联告警的 alarmObjects(权威) → 告警名 解析事故所属集群
    related = list_alerts(hours=hours, line=40)
    ns = ""
    for a in related:
        ns = cluster_env_from_alarm(a.get("alarm", {}))
        if ns:
            break
    gateway = app_logs("gateway", hours=hours, query="error", line=5, namespace=ns)
    # 补 k8s 控制面错误日志：apiserver 拒绝/调度失败/evict/OOM 等集群层根因证据
    k8s_errors = ([l.summary for l in k8s_logs("apiserver", hours=hours, line=3)]
                  + [l.summary for l in k8s_logs("kcm", hours=hours, line=3)])
    return {
        "incident": rec,
        "_diagnosis_namespace": ns,  # 标注本诊断锁定的环境，便于 IC 核对
        "related_alerts": [a.get("alarm", {}) for a in related[:3]],
        "gateway_errors": [l.summary for l in gateway],
        "k8s_control_plane_errors": k8s_errors,
    }
