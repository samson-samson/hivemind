#!/usr/bin/env python3
"""Hivemind 智能诊断入口（headless-agent）。

完整流程：真实事故（SLS itsm）→ 只读采集上下文 → LLM 诊断（hermes 配置）→ 报告。

用法：
  python3 diagnose.py                 # 诊断最近一条事故
  python3 diagnose.py --index 1       # 诊断第 N 条事故（按时间倒序）
  python3 diagnose.py --hours 24      # 采集时间窗（小时）
  python3 diagnose.py --report path   # 同时写报告文件
  python3 diagnose.py --list          # 仅列出最近事故，不诊断
"""
import argparse
import json
import sys
import urllib.request

import sls_reader
import llm


def fmt_report(result: dict) -> str:
    lines = []
    lines.append(f"# 智能诊断报告 {result.get('incident_id', '')}")
    lines.append("")
    lines.append(f"**摘要**：{result.get('summary', '—')}")
    lines.append(f"**严重度估计**：{result.get('severity_guess', '—')}")
    lines.append("")
    lines.append("## 症状")
    for s in result.get("symptoms", []):
        lines.append(f"- {s}")
    lines.append("")
    lines.append("## 根因假设（按置信度）")
    for h in result.get("hypotheses", []):
        lines.append(f"- [{h.get('confidence', '—')}] **{h.get('root_cause', '—')}**")
        for ev in h.get("evidence_for", []):
            lines.append(f"  - 证据：{ev}")
    lines.append("")
    lines.append("## 矛盾 / 已排除")
    for c in result.get("contradictions", []):
        lines.append(f"- {c}")
    lines.append("")
    lines.append("## 建议行动")
    for a in result.get("recommended_actions", []):
        lines.append(f"- ({a.get('kind', '?')}/{a.get('risk', '?')}) {a.get('action', '')}")
    lines.append("")
    # 告警质量：测试告警/level虚高/路由重复——测试环境"问题特别多"的关键分流信息
    q = result.get("alert_quality")
    if isinstance(q, dict) and any(q.get(k) for k in ("is_test_alert", "severity_overstated", "route_duplication")):
        lines.append("## ⚠ 告警质量问题")
        if q.get("is_test_alert"):
            lines.append(f"- **测试告警**：{q.get('test_alert_reason', '疑似投递验证/测试回环，不应按生产事故处理')}")
        if q.get("severity_overstated"):
            lines.append("- **严重度虚高**：告警 level 配置与真实影响不匹配，建议调低")
        if q.get("route_duplication"):
            lines.append("- **路由重复**：同一告警被多个通知策略重复触发，会被重复计数放大")
        if q.get("notes"):
            lines.append(f"- 备注：{q['notes']}")
        lines.append("")
    lines.append("## 未决问题与证据盲区")
    for q in result.get("open_questions", []):
        lines.append(f"- {q}")
    for g in result.get("evidence_gaps", []):
        lines.append(f"- 盲区：{g}")
    lines.append("")
    lines.append("---")
    lines.append("> **决策边界**：以上由 AI 完成根因定位与辅助分析；")
    lines.append("> **止血/修复方案必须由 IC（人类 Incident Commander）决策**，AI 不自动执行任何动作。")
    return "\n".join(lines)


def push_to_platform(cp_base: str, incident_id: str, result: dict, api_key: str = "") -> list[str]:
    """把诊断结果写入 Hivemind 控制平面 IOM：每个假设成为一条 Hypothesis 发言。

    语义：headless-diagnoser 作为"数字员工"，通过「发言人」协议提交提案；
    会议室右侧假设面板与协同诊断流会实时出现这些发言。

    认证：控制平面启用 API-key 认证（AuthMiddleware），push 必须带
    X-API-Key 头，否则 401 导致整个诊断在提交步静默失败。
    """
    headers = {"Content-Type": "application/json"}
    if api_key:
        headers["X-API-Key"] = api_key
    posted = []
    for i, h in enumerate(result.get("hypotheses", [])):
        body = json.dumps({
            "topic": f"[AI诊断] {h.get('root_cause', '')}",
            "supporting": [],
            "independence_weight": round(float(h.get("confidence", 0.1)), 2),
            "confidence": round(float(h.get("confidence", 0.1)), 2),
            "status": "supported" if h.get("confidence", 0) >= 0.5 else "proposed",
            "source": "headless-diagnoser",
        }).encode()
        req = urllib.request.Request(
            f"{cp_base}/api/v1/incidents/{incident_id}/hypotheses",
            data=body,
            headers=headers,
            method="POST",
        )
        with urllib.request.urlopen(req, timeout=30) as resp:
            created = json.loads(resp.read().decode())
            posted.append(created.get("id", f"hyp-{i}"))
        print(f"  → 提交假设 [{h.get('confidence')}] {h.get('root_cause', '')[:60]}", file=sys.stderr)
    return posted


def main() -> int:
    ap = argparse.ArgumentParser(description="Hivemind 智能诊断（只读）")
    ap.add_argument("--index", type=int, default=0, help="诊断第几条事故（按时间倒序，默认最近一条）")
    ap.add_argument("--hours", type=int, default=168, help="采集时间窗（小时）")
    ap.add_argument("--report", type=str, default="", help="报告输出路径（默认 stdout）")
    ap.add_argument("--list", action="store_true", help="仅列出最近事故")
    ap.add_argument("--push", metavar="INCIDENT_ID", default="", help="把诊断假设写入控制平面事故（会议室可见）")
    ap.add_argument("--cp-base", default="http://localhost:8081", help="控制平面 base URL")
    ap.add_argument("--api-key", default="", help="控制平面 API key（缺省读 HIVEMIND_API_KEY）")
    ap.add_argument("--alert", metavar="ALARM_NAME", default="", help="从指定告警（alertmanager-alarm-event）出发诊断")
    ap.add_argument("--incident-json", default="", help="被触发事故的 JSON（title/symptom_set），用于过滤采集，避免串房")
    args = ap.parse_args()

    print(f"[1/3] 读取最近事故（{args.hours}h 窗口）…", file=sys.stderr)
    incidents = sls_reader.list_incident_records(hours=args.hours, line=10)
    if not incidents:
        print("未找到事故记录。", file=sys.stderr)
        return 1
    for i, inc in enumerate(incidents):
        rec = inc.get("record", {})
        ann = rec.get("annotations", {}) if isinstance(rec, dict) else {}
        print(f"  [{i}] {inc['time']} | {str(ann.get('message', ''))[:80]}", file=sys.stderr)
    if args.list:
        return 0

    # 诊断目标：优先取被触发事故（incident-json），其次按告警名。
    incident_meta = None
    if args.incident_json:
        try:
            incident_meta = json.loads(args.incident_json)
        except json.JSONDecodeError:
            incident_meta = None

    if incident_meta:
        # 串房修复：按被触发事故的 title/症状关键词过滤告警采集，
        # 而不是"SLS 最新事故"。
        print(f"[2/3] 采集事故上下文（{incident_meta.get('id', '?')}：{str(incident_meta.get('title', ''))[:50]}）…", file=sys.stderr)
        title = str(incident_meta.get("title", ""))
        symptoms = incident_meta.get("symptom_set", [])
        keywords = [title] + [str(s)[:30] for s in symptoms]
        all_alerts = sls_reader.list_alerts(hours=max(24, args.hours), line=120)
        matches = [a for a in all_alerts
                   if any(k and k.lower() in str(a.get("alarm", {}).get("alarmName", "")).lower()
                          or k and k.lower() in str(a.get("alarm", {}).get("describe", "")).lower() for k in keywords if k)]
        # 从命中的告警名锁环境，避免采集到 dev/prod/casdoor 串房日志
        ns = ""
        for m in matches:
            ns = sls_reader.cluster_env_from_alarm(m.get("alarm", {}))
            if ns:
                break
        ctx = {
            "incident": {"id": incident_meta.get("id"), "title": title,
                         "symptom_set": symptoms, "fingerprint": incident_meta.get("fingerprint")},
            "_diagnosis_namespace": ns,
            "related_alerts": [m.get("alarm", {}) for m in matches[-3:]],
            "gateway_errors": [l.summary for l in sls_reader.app_logs(
                "gateway", hours=24, query="error", line=5, namespace=ns)],
            "k8s_control_plane_errors": (
                [l.summary for l in sls_reader.k8s_logs("apiserver", hours=24, line=3)]
                + [l.summary for l in sls_reader.k8s_logs("kcm", hours=24, line=3)]),
        }
        ctx["incident_time"] = "incident-triggered"
        source_label = incident_meta.get("id", title)
    elif args.alert:
        # 从告警事件出发：按告警名取上下文（dev 环境真实故障告警）
        print(f"[2/3] 采集告警上下文（{args.alert}）…", file=sys.stderr)
        matches = [a for a in sls_reader.list_alerts(hours=max(24, args.hours), line=120)
                   if args.alert in str(a.get("alarm", {}).get("alarmName", ""))]
        if not matches:
            print(f"未找到告警 {args.alert}", file=sys.stderr)
            return 1
        alarm = matches[-1].get("alarm", {})  # 取最近一条
        # 从告警名锁环境，避免采集到 dev/prod/casdoor 的串房日志
        ns = sls_reader.cluster_env_from_alarm(alarm)
        print(f"  锁定环境 namespace={ns or '(未解析，不过滤)'}", file=sys.stderr)
        ctx = {
            "incident": {"alarm": alarm},
            "_diagnosis_namespace": ns,
            "related_alerts": [m.get("alarm", {}) for m in matches[-3:]],
            "gateway_errors": [l.summary for l in sls_reader.app_logs(
                "gateway", hours=24, query="error", line=5, namespace=ns)],
            "k8s_control_plane_errors": (
                [l.summary for l in sls_reader.k8s_logs("apiserver", hours=24, line=3)]
                + [l.summary for l in sls_reader.k8s_logs("kcm", hours=24, line=3)]),
        }
        ctx["incident_time"] = matches[-1]["time"]
        source_label = args.alert
    else:
        inc = incidents[min(args.index, len(incidents) - 1)]
        print(f"[2/3] 采集诊断上下文（事故 {inc['time']}）…", file=sys.stderr)
        ctx = sls_reader.collect_incident_context(inc, hours=max(24, min(args.hours, 24)))
        ctx["incident_time"] = inc["time"]
        source_label = inc["time"]

    print(f"[3/3] LLM 智能诊断（{llm._config()[2]}）…", file=sys.stderr)
    result = llm.diagnose(ctx)
    result["incident_id"] = source_label
    result["incident_message"] = str(ctx.get("incident", {}).get("annotations", {}).get("message", "") if isinstance(ctx.get("incident"), dict) else "")[:200]

    report = fmt_report(result)
    print(report)
    if args.report:
        with open(args.report, "w") as f:
            f.write(report)
        with open(args.report.replace(".md", ".json"), "w") as f:
            json.dump(result, f, ensure_ascii=False, indent=2)
        print(f"\n报告已写入 {args.report}", file=sys.stderr)
    if args.push:
        import os
        api_key = args.api_key or os.environ.get("HIVEMIND_API_KEY", "hivemind-dev-key")
        print(f"[4/4] 提交假设到控制平面（{args.cp_base}/api/v1/incidents/{args.push}/hypotheses）…", file=sys.stderr)
        push_to_platform(args.cp_base, args.push, result, api_key)
    return 0


if __name__ == "__main__":
    sys.exit(main())
