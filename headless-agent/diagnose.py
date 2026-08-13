#!/usr/bin/env python3
"""OpsHive 智能诊断入口（headless-agent）。

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


def push_to_platform(cp_base: str, incident_id: str, result: dict) -> list[str]:
    """把诊断结果写入 OpsHive 控制平面 IOM：每个假设成为一条 Hypothesis 发言。

    语义：headless-diagnoser 作为"数字员工"，通过「发言人」协议提交提案；
    会议室右侧假设面板与协同诊断流会实时出现这些发言。
    """
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
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        with urllib.request.urlopen(req, timeout=30) as resp:
            created = json.loads(resp.read().decode())
            posted.append(created.get("id", f"hyp-{i}"))
        print(f"  → 提交假设 [{h.get('confidence')}] {h.get('root_cause', '')[:60]}", file=sys.stderr)
    return posted


def main() -> int:
    ap = argparse.ArgumentParser(description="OpsHive 智能诊断（只读）")
    ap.add_argument("--index", type=int, default=0, help="诊断第几条事故（按时间倒序，默认最近一条）")
    ap.add_argument("--hours", type=int, default=168, help="采集时间窗（小时）")
    ap.add_argument("--report", type=str, default="", help="报告输出路径（默认 stdout）")
    ap.add_argument("--list", action="store_true", help="仅列出最近事故")
    ap.add_argument("--push", metavar="INCIDENT_ID", default="", help="把诊断假设写入控制平面事故（会议室可见）")
    ap.add_argument("--cp-base", default="http://localhost:8081", help="控制平面 base URL")
    ap.add_argument("--alert", metavar="ALARM_NAME", default="", help="从指定告警（alertmanager-alarm-event）出发诊断")
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

    if args.alert:
        # 从告警事件出发：按告警名取上下文（dev 环境真实故障告警）
        print(f"[2/3] 采集告警上下文（{args.alert}）…", file=sys.stderr)
        matches = [a for a in sls_reader.list_alerts(hours=max(24, args.hours), line=20)
                   if args.alert in str(a.get("alarm", {}).get("alarmName", ""))]
        if not matches:
            print(f"未找到告警 {args.alert}", file=sys.stderr)
            return 1
        alarm = matches[-1].get("alarm", {})  # 取最近一条
        ctx = {
            "incident": {"alarm": alarm},
            "related_alerts": [m.get("alarm", {}) for m in matches[-3:]],
            "gateway_errors": [l.summary for l in sls_reader.app_logs("gateway", hours=24, query="error", line=5)],
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
        print(f"[4/4] 提交假设到控制平面（{args.cp_base}/incidents/{args.push}）…", file=sys.stderr)
        push_to_platform(args.cp_base, args.push, result)
    return 0


if __name__ == "__main__":
    sys.exit(main())
