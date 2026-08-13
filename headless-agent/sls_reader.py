"""SLS 只读数据采集（OpsHive headless-agent）。

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


def app_logs(logstore: str, hours: int = 24, query: str = "*", line: int = 5) -> list[SLSLog]:
    """应用日志（goodputai-dev 各 logstore）。"""
    return _get_logs(APP_PROJECT, logstore, hours, query=query, line=line)


def collect_incident_context(incident: dict, hours: int = 24) -> dict:
    """围绕一条事故记录采集诊断上下文（只读）。"""
    rec = incident.get("record", {})
    msg = str(rec.get("annotations", {}).get("message", ""))[:200]
    return {
        "incident": rec,
        "related_alerts": [a.get("alarm", {}) for a in list_alerts(hours=hours, line=3)],
        "gateway_errors": [l.summary for l in app_logs("gateway", hours=hours, query="error", line=5)],
    }
