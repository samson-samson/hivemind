# OACP — Hivemind Agent Communication Protocol

> 版本：v0.1（草案）· 状态：Draft · 定位：Hivemind 内部多智能体通信的**事实层协议**
> 运行时底座：Agent Swarms（[samson-samson/claude-code](../../docs/references/agent-swarms.md)）

## 1. 目标与原则

Hivemind 的"会议室"里，多个 agent（探索者/验证者/怀疑者/执行者/诊断器）需要有序协同。
通信机制**复用 Agent Swarms 原生实现**（SendMessage + 文件信箱 + Coordinator），
OACP 定义的是**消息在事实层（IOM）之上的语义**。

三条铁律：

1. **聊天不进事实层**——agent 的任意文本消息不改变事故状态；
   只有 `structured` 消息（证据/假设/决策）进入 IOM。
2. **每个故障独立会议室**——同指纹归并，异指纹隔离，消息按 `incident_id` 路由。
3. **AI 只定位，IC 决策**——`decision` 类消息只能由 IC（人类）发出。

## 2. 消息信封

所有 agent 间消息采用统一信封（与 SendMessage 载荷兼容）：

```json
{
  "protocol": "oacp/v0.1",
  "incident_id": "inc_xxxxxxxx",
  "from": "explorer-1",
  "to": "lead | * | uds:/path | bridge:<session-id>",
  "type": "message | broadcast | evidence | hypothesis | fact | decision | guidance | shutdown_request | shutdown_response | plan_approval_response",
  "request_id": "r_xxxxxxxx",
  "timestamp": "2026-08-17T00:00:00Z",
  "structured": false,
  "content": "…",
  "evidence_ref": null
}
```

### 字段说明

| 字段 | 规则 |
|---|---|
| `protocol` | 固定 `oacp/v0.1`，协议演进用 `oacp/v0.2` 向后兼容 |
| `incident_id` | 必填；控制平面据此路由到对应会议室 |
| `from` / `to` | Agent Swarms 寻址（teammate 名 / `*` / `uds:` / `bridge:`） |
| `type` | 见 §3 事实层分类 |
| `structured` | **true 时消息才进 IOM**；false 仅进会议纪要 |
| `evidence_ref` | 关联证据 ID（证据链溯源） |

## 3. 事实层分类（structured 消息才可进 IOM）

| type | 进 IOM 节点 | 谁可发 | 说明 |
|---|---|---|---|
| `evidence` | Evidence | 任意 agent | 必须带 `content` 结果与 `evidence_ref`（来源操作） |
| `hypothesis` | Hypothesis | 任意 agent | 必须带置信度（`content` 内 JSON） |
| `fact` | Fact | 任意 agent + IC 确认 | `structured: true` 且证据链闭合 |
| `guidance` | Guidance | **仅 IC** | 人的指示，agent 读到无需回复 |
| `decision` | 动作审批 | **仅 IC** | 止血/修复决策（AI 只提候选，见 `recommended_actions` 报告，不产生 decision） |
| `message` / `broadcast` | 会议纪要（不入 IOM） | 任意 | 自由文本，永不改变事故状态 |

## 4. 与 Agent Swarms 的映射

| Agent Swarms 机制 | OACP 用途 |
|---|---|
| `SendMessageTool`（teammate/`*`/`uds:`/`bridge:`） | 消息传输层 |
| Teammate Mailbox（`~/.claude/teams/{team}/inboxes/{agent}.json`） | 离线/异步投递 + 控制平面订阅点 |
| Coordinator Mode（`CLAUDE_CODE_COORDINATOR_MODE`） | Lead 创建/销毁 workers |
| Backends（tmux/iterm2/in-process/**hivemind**） | 执行环境；`hivemind` 后端把消息镜像到控制平面 |

## 5. 事实层过滤（控制平面侧）

控制平面订阅 mailbox/backend 镜像流，执行：

1. 校验信封（`protocol`/`incident_id`/`from` 合法性）；
2. `structured=false` → 仅追加会议纪要（不改变事故状态）；
3. `structured=true` → 按 `type` 映射为 IOM 节点（Evidence/Hypothesis/Fact/Guidance）；
4. `decision` 类校验发送者身份（必须 IC）；
5. 全部动作 append 到证据总线（可审计、可回放）。

## 6. 安全

- 跨机 `bridge:` 消息需要显式用户同意（沿用 fork 的 safetyCheck 语义）；
- `incident_id` 越权校验（agent 只能向自己所属会议室的 incident 发 structured 消息）；
- 所有消息可溯源（`request_id` + 证据总线 append-only）。
