#!/usr/bin/env python3
"""Hivemind 蒸馏器 —— 把一次排障沉淀为候选 runbook（§5.2 事故后认证）。

流程：拉取事故数据（症状/确认假设/IC 决策）→ LLM 生成类型化候选 runbook
→ POST 控制平面（status=candidate）→ IC 确认后 certified（下次召回）。

用法：
  python3 distill.py <incident_id> [--cp-base http://localhost:8081] [--api-key KEY]
"""
import json
import sys
import urllib.request

sys.path.insert(0, "../headless-agent")
import llm  # noqa: E402  (复用 hermes AI 配置)

CP_BASE = "http://localhost:8081"
API_KEY = "hivemind-dev-key"


def _api(method: str, path: str, body: dict | None = None) -> dict | list:
    req = urllib.request.Request(
        f"{CP_BASE}{path}",
        data=json.dumps(body).encode() if body is not None else None,
        headers={"Content-Type": "application/json", "X-API-Key": API_KEY},
        method=method,
    )
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            return json.loads(r.read().decode() or "{}")
    except urllib.error.HTTPError as e:
        print(f"[distill] HTTP {e.code}: {e.read().decode()[:200]}", file=sys.stderr)
        return {"error": f"HTTP {e.code}"}


def collect(incident_id: str) -> dict:
    inc = _api("GET", f"/api/v1/incidents/{incident_id}")
    hyps = _api("GET", f"/api/v1/incidents/{incident_id}/hypotheses")
    guides = _api("GET", f"/api/v1/incidents/{incident_id}/guidance")
    evs = _api("GET", f"/api/v1/incidents/{incident_id}/evidence")
    ev_list = evs.get("evidence", []) if isinstance(evs, dict) else evs
    return {
        "incident": inc,
        "hypotheses": hyps if isinstance(hyps, list) else [],
        "guidance": guides if isinstance(guides, list) else [],
        "evidence": ev_list,
    }


PROMPT = """你是 Hivemind 的知识蒸馏器。把下面这次事故的排障过程沉淀为一份可复用的
runbook（类型化）。只输出严格 JSON（不要 markdown），结构：
{
  "title": "一句话标题（症状→根因）",
  "symptoms": ["适用症状关键词（用于召回，3-6 个短词）"],
  "root_cause": "命名根因 + 传播路径 + 作用机制",
  "diagnostic_steps": ["按序的诊断命令/查询，含预期结果"],
  "verification_actions": ["验证修复有效的动作"],
  "rollback": "回滚/补偿方案；无法回滚写 'N/A'",
  "success_criteria": "可度量的成功判定（指标阈值/时间窗）"
}
注意：只依据给定数据推理；一次事故成功不等于可复用，IC 会确认是否 certified。
事故数据：
"""


def distill(incident_id: str) -> dict:
    print(f"[1/3] 拉取事故数据（{incident_id}）…", file=sys.stderr)
    ctx = collect(incident_id)
    payload = {
        "incident": {"title": ctx["incident"].get("title"), "symptom_set": ctx["incident"].get("symptom_set")},
        "top_hypotheses": [{ "topic": h.get("topic"), "confidence": h.get("confidence"), "status": h.get("status") }
                           for h in ctx["hypotheses"][:3]],
        "ic_guidance": [g.get("text") for g in ctx["guidance"][-3:]],
        "evidence_count": len(ctx["evidence"]),
    }
    print(f"[2/3] LLM 生成候选 runbook（{llm._config()[2]}）…", file=sys.stderr)
    out = llm.chat([{"role": "system", "content": PROMPT},
                    {"role": "user", "content": json.dumps(payload, ensure_ascii=False, indent=2)[:10000]}])
    out = out.strip().strip("`")
    if out.startswith("json"):
        out = out[4:]
    rb = json.loads(out)
    print(f"[3/3] 提交候选 runbook 到控制平面…", file=sys.stderr)
    created = _api("POST", f"/api/v1/incidents/{incident_id}/runbooks", {
        **rb, "source": "distiller"})
    if isinstance(created, dict) and "error" in created:
        print(f"[distill] 失败: {created['error']}", file=sys.stderr)
        return created
    print(f"候选 runbook 已创建: {created.get('id')}（status=candidate，待 IC 确认）", file=sys.stderr)
    return created


if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("用法: python3 distill.py <incident_id> [--cp-base URL] [--api-key KEY]", file=sys.stderr)
        sys.exit(1)
    incident_id = sys.argv[1]
    args = sys.argv[2:]
    for i, a in enumerate(args):
        if a == "--cp-base" and i + 1 < len(args):
            CP_BASE = args[i + 1]
        if a == "--api-key" and i + 1 < len(args):
            API_KEY = args[i + 1]
    result = distill(incident_id)
    print(json.dumps(result, ensure_ascii=False, indent=2))
