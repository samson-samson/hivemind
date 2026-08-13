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
    return "\n".join(lines)


def main() -> int:
    ap = argparse.ArgumentParser(description="OpsHive 智能诊断（只读）")
    ap.add_argument("--index", type=int, default=0, help="诊断第几条事故（按时间倒序，默认最近一条）")
    ap.add_argument("--hours", type=int, default=168, help="采集时间窗（小时）")
    ap.add_argument("--report", type=str, default="", help="报告输出路径（默认 stdout）")
    ap.add_argument("--list", action="store_true", help="仅列出最近事故")
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

    inc = incidents[min(args.index, len(incidents) - 1)]
    print(f"[2/3] 采集诊断上下文（事故 {inc['time']}）…", file=sys.stderr)
    ctx = sls_reader.collect_incident_context(inc, hours=max(24, min(args.hours, 24)))
    ctx["incident_time"] = inc["time"]

    print(f"[3/3] LLM 智能诊断（{llm._config()[2]}）…", file=sys.stderr)
    result = llm.diagnose(ctx)
    result["incident_id"] = inc["time"]
    result["incident_message"] = str(
        (inc.get("record", {}).get("annotations", {}) if isinstance(inc.get("record"), dict) else {}).get("message", "")
    )[:200]

    report = fmt_report(result)
    print(report)
    if args.report:
        with open(args.report, "w") as f:
            f.write(report)
        with open(args.report.replace(".md", ".json"), "w") as f:
            json.dump(result, f, ensure_ascii=False, indent=2)
        print(f"\n报告已写入 {args.report}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
