# Hivemind AI 接入配置

> 三类 AI 各有一条接入路径：**平台内置诊断器**（headless-agent）、
> **本地 agent**（你的 Claude Code/Codex，经 MCP 适配器进会议室）、
> **其他自研 agent**（OACP SDK）。配置项集中在此。

## 1. 平台内置诊断器（headless-agent）

读取顺序：环境变量 > `~/.hermes/.env`（复用本机 hermes 配置）。

| 配置项 | 环境变量 | 默认 | 说明 |
|---|---|---|---|
| API 地址 | `GOODPUTAI_BASE_URL` | `https://api.goodputai.cn/v1` | OpenAI 兼容端点 |
| API Key | `GOODPUTAI_API_KEY` | 取 `~/.hermes/.env` | 凭证不入库 |
| 诊断模型 | `OPSHIVE_LLM_MODEL` | `z-ai/glm-5.2` | 可换 `deepseek/deepseek-v4-pro` 等 |
| 诊断脚本路径 | `OPSHIVE_DIAGNOSE_SCRIPT` | `../headless-agent/diagnose.py` | 控制平面子进程调用 |
| 控制平面回连地址 | `OPSHIVE_PUBLIC_BASE` | `http://localhost:8081` | 可信配置，防回连 |

快速验证：
```bash
cd headless-agent && python3 diagnose.py --list   # 读 SLS 事故
python3 diagnose.py --push <incident-id>          # 诊断并提交假设到会议室
```

## 2. 本地 agent 接入（MCP 适配器「发言人」）

你的 Claude Code / Codex 装 `adapter/mcp-server/` 后，排查内容实时进会议室：

```jsonc
// ~/.claude.json 或项目 .mcp.json
{
  "mcpServers": {
    "hivemind": {
      "command": "python3",
      "args": ["/path/to/hivemind/adapter/mcp-server/server.py"],
      "env": { "HIVEMIND_CP_BASE": "http://localhost:8081" }
    }
  }
}
```

装好后本地 agent 获得 5 个工具（排查即共享）：
`incident.check_in` · `incident.work_claim` · `incident.push_evidence` ·
`incident.propose` · `incident.get_context`

语义：`push_evidence`/`propose` 走 OACP 事实层——只有结构化内容改变事故状态，
聊天文本只进会议纪要（聊天不进事实层）。

## 3. 自研 agent（OACP SDK，规划中）

Go/Python/TS SDK：强类型 OACP envelope + 身份签名 + context@vN diff +
租约心跳（见 `agent-runtime/protocol/OACP.md`）。SDK 与 conformance 套件在 P1。

## 4. 阿里云只读数据（headless-agent 采集用）

经本机 `aliyun` CLI（环境变量或 `aliyun configure`）：
`ALIBABA_CLOUD_ACCESS_KEY_ID` / `ALIBABA_CLOUD_ACCESS_KEY_SECRET`
建议 RAM 策略：`AliyunLogReadOnlyAccess`（只读 SLS）。

## 5. 凭证纪律

所有真值走环境变量；仓库只保留 `.env.example` 占位。任何 AK/SK/token 提交进
仓库即视为安全事故（见 SECURITY.md）。
