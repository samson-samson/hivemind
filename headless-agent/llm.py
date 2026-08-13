"""LLM 调用层 —— OpenAI 兼容协议，复用本机 hermes 的 AI 配置。

- base_url: https://api.goodputai.cn/v1（来自 ~/.hermes/config.yaml）
- API key: GOODPUTAI_API_KEY（来自 ~/.hermes/.env）
- 默认模型: z-ai/glm-5.2（hermes 默认），可 OPSHIVE_LLM_MODEL 覆盖
"""
import json
import os
import urllib.request


def _config() -> tuple[str, str, str]:
    """读取 hermes 配置（key 不落盘、不打印）。"""
    env_path = os.path.expanduser("~/.hermes/.env")
    key = os.environ.get("GOODPUTAI_API_KEY", "")
    if not key and os.path.exists(env_path):
        for line in open(env_path):
            if line.startswith("GOODPUTAI_API_KEY="):
                key = line.strip().split("=", 1)[1].strip('"').strip("'")
                break
    base_url = os.environ.get("GOODPUTAI_BASE_URL", "https://api.goodputai.cn/v1")
    model = os.environ.get("OPSHIVE_LLM_MODEL", "z-ai/glm-5.2")
    if not key:
        raise RuntimeError("GOODPUTAI_API_KEY not found (check ~/.hermes/.env)")
    return base_url, key, model


def chat(messages: list[dict], temperature: float = 0.2, max_tokens: int = 4096) -> str:
    """一次 chat completion 调用，返回 assistant 文本。"""
    base_url, key, model = _config()
    body = json.dumps({
        "model": model,
        "messages": messages,
        "temperature": temperature,
        "max_tokens": max_tokens,
    }).encode()
    req = urllib.request.Request(
        f"{base_url}/chat/completions",
        data=body,
        headers={"Content-Type": "application/json", "Authorization": f"Bearer {key}"},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=180) as resp:
        data = json.loads(resp.read().decode())
    return data["choices"][0]["message"]["content"]


def diagnose(incident_context: dict) -> dict:
    """用 LLM 做智能诊断：症状 → 假设 → 证据 → 行动，输出结构化 JSON。

    只做只读分析与建议；不产生任何写操作。
    """
    sys_prompt = """你是 OpsHive 的资深 SRE 排障专家（Agentic AIOps）。
你会收到一条线上事故的真实上下文（事故记录、关联告警、相关错误日志）。
请完成智能诊断，输出严格 JSON（不要任何 markdown 包裹），结构：
{
  "summary": "一句话事故摘要",
  "severity_guess": "P1|P2|P3",
  "symptoms": ["观测到的异常症状"],
  "hypotheses": [{"rank": 1, "root_cause": "最可能根因", "evidence_for": ["支持证据"], "confidence": 0.0-1.0}],
  "contradictions": ["相互矛盾或已排除的假设"],
  "recommended_actions": [{"action": "止血/验证动作", "kind": "mitigate|verify|fix", "risk": "low|medium|high"}],
  "open_questions": ["还需确认的问题"],
  "evidence_gaps": ["当前证据链的盲区"]
}
注意：只基于给定证据推理，不编造；证据不足的假设请降低置信度并标注 evidence_gaps。"""
    user_prompt = "事故上下文（只读采集）：\n" + json.dumps(incident_context, ensure_ascii=False, indent=2)[:12000]
    out = chat([{"role": "system", "content": sys_prompt}, {"role": "user", "content": user_prompt}])
    # 容错：剥掉可能存在的 markdown 代码围栏
    out = out.strip()
    if out.startswith("```"):
        out = out.strip("`")
        if out.startswith("json"):
            out = out[4:]
    try:
        return json.loads(out)
    except json.JSONDecodeError:
        return {"summary": "LLM 输出非 JSON", "raw": out[:2000]}
