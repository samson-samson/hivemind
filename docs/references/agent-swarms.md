# Agent Swarms（samson-samson/claude-code fork）参考

> 调研于 2026-08-17 · 仓库：https://github.com/samson-samson/claude-code
> 用途：Hivemind 内部多智能体通信的运行时底座。

## 核心机制

| 机制 | 位置 | 说明 |
|---|---|---|
| **SendMessageTool** | `src/tools/SendMessageTool/` | agent 间通信主工具（swarm protocol）。寻址：teammate 名 / `*` 广播 / `uds:<socket>` 本地 peer / `bridge:<session-id>` 跨会话（Remote Control） |
| **Teammate Mailbox** | `src/utils/teammateMailbox.ts` | 文件信箱：`~/.claude/teams/{team}/inboxes/{agent}.json`；写方写文件、收方见附件（TEAMMATE_MESSAGE_TAG）；带锁文件 |
| **Coordinator Mode** | `src/coordinator/coordinatorMode.ts` | `CLAUDE_CODE_COORDINATOR_MODE` 门控；主 agent 用 TeamCreate/TeamDelete/SendMessage/SyntheticOutput 协调 worker agents |
| **Backends** | `src/utils/swarm/backends/` | 可插拔执行后端：`tmux` / `iterm2` / `in-process`（registry 分发）——**Hivemind 新增 `hivemind` 后端**：teammates 真实执行 + 消息镜像到控制平面 |
| **Inter-Claude 跨会话** | `src/bridge/peerSessions.ts` | `postInterClaudeMessage(target, msg)`：bridge API 向另一 Claude 会话发纯文本消息；跨机注入需显式用户同意（safetyCheck，classifier 不可绕过） |
| **团队目录** | `~/.claude/teams/{teamName}/` | inboxes/、permissions/（权限同步 `permissionSync.ts`） |

## 与 Hivemind 的集成点

1. **运行时**：会议室 agents = swarm teammates（Lead 创建 explorer/verifier/skeptic workers）。
2. **协议**：OACP（`agent-runtime/protocol/OACP.md`）定义消息语义与事实层过滤；传输层复用 SendMessage + Mailbox。
3. **镜像**：`hivemind` backend 把消息镜像到控制平面证据总线 → 会议室可视化 + IOM 结构化。
4. **多机**：`bridge:` 跨会话/跨机（ListPeers 发现），跨机消息需用户同意。

## 许可注意

该仓库为 Claude Code 的 fork，接入 Hivemind 时须核对其开源许可条款（MIT/企业许可边界），开源分发时在 NOTICE 中声明。
