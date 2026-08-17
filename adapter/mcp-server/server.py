#!/usr/bin/env python3
"""Hivemind MCP 适配器「发言人」—— 本地 agent（Claude Code/Codex）接入会议室。

最小 MCP server（JSON-RPC 2.0 over stdio，纯标准库，零依赖）。

装好后本地 agent 获得 5 个工具，排查内容实时进会议室：
  incident.check_in      登记身份（人负责制）
  incident.work_claim    认领工作单元（咨询性租约）
  incident.push_evidence 提交排查证据（事实层：structured）
  incident.propose       提出根因假设（事实层：structured）
  incident.get_context   拉取当前事故上下文包

语义：只有 push_evidence / propose（structured）改变事故状态；
聊天文本只进会议纪要（聊天不进事实层）。

用法见 docs/setup/ai-config.md §2。
"""
import json
import os
import sys
import urllib.request

CP_BASE = os.environ.get("HIVEMIND_CP_BASE", "http://localhost:8081")
AGENT_NAME = os.environ.get("HIVEMIND_AGENT_NAME", "local-agent")

TOOLS = [
    {
        "name": "incident.check_in",
        "description": "登记本地 agent 身份（人负责制，后续动作挂此人名）",
        "inputSchema": {"type": "object", "properties": {"operator": {"type": "string", "description": "操作人姓名"}}},
    },
    {
        "name": "incident.work_claim",
        "description": "认领一个工作单元（咨询性租约，不独占）",
        "inputSchema": {"type": "object", "properties": {
            "incident_id": {"type": "string"}, "work_node_id": {"type": "string"}, "assignee": {"type": "string"}},
            "required": ["incident_id", "work_node_id"]},
    },
    {
        "name": "incident.push_evidence",
        "description": "提交一条排查证据（进入会议室事实账本；必须给出你实际执行过的命令/查询与结果）",
        "inputSchema": {"type": "object", "properties": {
            "incident_id": {"type": "string"}, "result": {"type": "string", "description": "命令/查询的实际输出摘要"},
            "conclusion": {"type": "string", "description": "你从该结果得出的结论"}, "source": {"type": "string"}},
            "required": ["incident_id", "result"]},
    },
    {
        "name": "incident.propose",
        "description": "提出一个根因假设（进入会议室假设矩阵；请填写分叉签名与可证伪预测）",
        "inputSchema": {"type": "object", "properties": {
            "incident_id": {"type": "string"}, "topic": {"type": "string", "description": "假设一句话"},
            "subsystem": {"type": "string", "description": "受影响子系统（分叉签名）"},
            "causal_mechanism": {"type": "string", "description": "因果机制（分叉签名，同签会被判定为验证副本）"},
            "falsifier": {"type": "string", "description": "可证伪预测（怎样能推翻它）"},
            "confidence": {"type": "number", "description": "0-1"},
            "evidence_ids": {"type": "array", "items": {"type": "string"}}},
            "required": ["incident_id", "topic", "subsystem", "causal_mechanism", "falsifier"]},
    },
    {
        "name": "incident.get_context",
        "description": "拉取事故上下文包（工作单元/证据/假设/反证清单），避免重复排查",
        "inputSchema": {"type": "object", "properties": {"incident_id": {"type": "string"}}, "required": ["incident_id"]},
    },
]


def _http(method: str, path: str, body: dict | None = None) -> dict:
    req = urllib.request.Request(
        f"{CP_BASE}{path}",
        data=json.dumps(body).encode() if body is not None else None,
        headers={"Content-Type": "application/json"},
        method=method,
    )
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            return json.loads(r.read().decode() or "{}")
    except urllib.error.HTTPError as e:
        return {"error": f"HTTP {e.code}: {e.read().decode()[:200]}"}
    except Exception as e:  # noqa: BLE001
        return {"error": str(e)}


def _call_tool(name: str, args: dict) -> dict:
    inc = args.get("incident_id", "")
    try:
        if name == "incident.check_in":
            return {"ok": True, "agent": AGENT_NAME, "operator": args.get("operator", ""), "note": "身份已登记（人负责制）"}
        if name == "incident.work_claim":
            r = _http("POST", f"/api/v1/incidents/{inc}/leases", {
                "work_node_id": args.get("work_node_id"), "assignee": args.get("assignee", AGENT_NAME), "role": "explorer"})
            return r if "error" not in r else {"error": r["error"]}
        if name == "incident.push_evidence":
            # 事实层：structured。登记操作（供证据溯源）→ 推证据。
            op = _http("POST", f"/api/v1/incidents/{inc}/operations", {
                "query": {"target": args.get("target", "local-agent"), "data_source": "agent-local",
                          "query_ast": args.get("result", "")[:200], "time_window": {"start": ""}},
                "source": AGENT_NAME})
            op_id = op.get("operation", {}).get("id") or op.get("id")
            r = _http("POST", f"/api/v1/incidents/{inc}/evidence", {
                "operation_id": op_id or "op_local",
                "data_source": "agent-local",
                "result": args.get("result", "")[:2000],
                "conclusion": args.get("conclusion", ""),
                "source": args.get("source", AGENT_NAME)})
            return {"ok": "error" not in r, "evidence": r.get("id"), "detail": r.get("error", "已进入事实账本")}
        if name == "incident.propose":
            r = _http("POST", f"/api/v1/incidents/{inc}/hypotheses", {
                "topic": args.get("topic", ""),
                "subsystem": args.get("subsystem", ""),
                "causal_mechanism": args.get("causal_mechanism", ""),
                "falsifier": args.get("falsifier", ""),
                "confidence": args.get("confidence", 0.5),
                "supporting": args.get("evidence_ids", []),
                "source": args.get("source", AGENT_NAME)})
            if "error" in r and "duplicate branch" in str(r.get("error", "")):
                return {"ok": False, "error": f"该假设与已有分支同签（{r.get('existing')}）——属验证副本，请提供不同机制或证据"}
            return {"ok": "error" not in r, "hypothesis": r.get("id"), "detail": r.get("error", "已进入假设矩阵")}
        if name == "incident.get_context":
            wn = _http("GET", f"/api/v1/incidents/{inc}/work-nodes")
            ev = _http("GET", f"/api/v1/incidents/{inc}/evidence")
            hy = _http("GET", f"/api/v1/incidents/{inc}/hypotheses")
            return {"work_nodes": wn if isinstance(wn, list) else [], "evidence": ev if isinstance(ev, list) else [],
                    "hypotheses": hy if isinstance(hy, list) else []}
    except Exception as e:  # noqa: BLE001
        return {"error": str(e)}
    return {"error": "unknown tool"}


def main() -> int:
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            msg = json.loads(line)
        except json.JSONDecodeError:
            continue
        msg_id = msg.get("id")
        method = msg.get("method", "")
        if method == "initialize":
            _reply(msg_id, {"protocolVersion": "2025-03-26", "capabilities": {"tools": {}},
                            "serverInfo": {"name": "hivemind-speaker", "version": "0.1.0"}})
        elif method == "notifications/initialized":
            pass
        elif method == "tools/list":
            _reply(msg_id, {"tools": TOOLS})
        elif method == "tools/call":
            params = msg.get("params", {})
            name = params.get("name", "")
            args = params.get("arguments", {}) or {}
            result = _call_tool(name, args)
            if isinstance(result, dict) and "error" in result and not result.get("ok"):
                _reply(msg_id, {"content": [{"type": "text", "text": f"❌ {result['error']}"}], "isError": True})
            else:
                _reply(msg_id, {"content": [{"type": "text", "text": json.dumps(result, ensure_ascii=False, indent=2)}]})
        elif method == "ping":
            _reply(msg_id, {})
    return 0


def _reply(msg_id, result):
    sys.stdout.write(json.dumps({"jsonrpc": "2.0", "id": msg_id, "result": result}) + "\n")
    sys.stdout.flush()


if __name__ == "__main__":
    sys.exit(main())
