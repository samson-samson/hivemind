# Hivemind（运维蜂巢）开发约定

> AIOps 多智能体故障协同平台。开源 · 云中立 · 阿里云第一实现。
> **权威契约**：`docs/superpowers/specs/2026-08-12-hivemind-design.md`（v2.1）

## 实现分工（模型路由，经 zenmux.ai gateway）

| 角色 | Agent `model` 档 | 实际模型 | 负责目录 |
|---|---|---|---|
| 设计 / 评审 | `fable` | `anthropic/claude-opus-5` | `docs/`、协议评审 |
| 前端编码 | `opus` | `moonshotai/kimi-k3` | `console/` |
| 后端编码 | `sonnet`（或默认） | `deepseek/deepseek-v4-flash` | `control-plane/`、`adapter-protocol/`、`adapter-sdk/`、`adapter-generator/`、`packs/`、`conformance/`、`distiller/`、`headless-agent/` |
| 便宜杂活 | `haiku` | `deepseek/deepseek-v4-flash` | — |

**分工原则**：前端、后端、设计三方可并行开工，互不等待。**设计文档即契约**——三方无共享上下文，各自读设计文档对应章节 + 各自的 P0 实施简报开工。

## 四条铁律（v2 起，不可违反）

1. **排障不是互斥写入**——协同目标是"无意重复最小化 + 受控冗余交叉验证"，**不是"重复归零"**。
2. **发生过 ≠ 可复用**——蒸馏分两流水线：事故中只产候选，事故后认证；LLM 只生成候选，编译/策略/测试决定发布。
3. **不造 agent 引擎**——复用本地 Claude Code/Codex，平台只做薄 MCP 适配器。
4. **写操作永远人审**——无威胁模型不进入写路径；P0 无写路径。

## 关键机制速查（细节见设计文档）

- **协同**：工作图（`question+scope+预期证据+成本+截止`）· 咨询性租约 · 查询级 single-flight（操作指纹）· 数据血缘 DAG · 角色（探索/验证/怀疑/执行）· 高风险双路验证。
- **协商 PMA**：聊天不进事实层——只有结构化证据和决策改变事故状态；合并是决策支持排序（血缘独立性），根因真假归人裁决。
- **蒸馏**：两流水线；runbook 类型化 + 认证链（candidate→reviewed→validated→certified→revoked），失效由依赖/失败率/反例驱动非 TTL。
- **能力体系**：Skill → Expert → Expert Group（三级）；**不借鉴"全自主专家团"**，默认受 IC + PMA + 证据门控约束。
- **接入**：版本化 Capability Descriptor + 四档接入 + 接入成熟度五级 + conformance suite。
- **安全**：三层模型（行为安全沙箱 / 权限安全最小权限 / 网络安全出站管控）+ 威胁模型 + 管理后台。
- **指标**：无意重复率 · 有效独立验证覆盖率 · 单位工具成本信息增益 · 错误排除率 · 决策延迟 + 负向指标（误根因率/审批质量/知识污染率）。

## 路线图

- **P0 只读协同账本**（当前）：工作图 + 查询去重 + 证据血缘 + 人工 IC + 指挥室 v1。**无写路径**。
- P1 评测与检索 → P2 候选生成 → P3 强类型可回滚动作（单 K8s）→ P4 生态。

## 开工入口

- 后端（deepseek-v4-flash）：先读 `docs/implementation/P0-backend-brief.md` + 设计文档 §4/§5/§6。
- 前端（kimi-k3）：先读 `docs/implementation/P0-frontend-brief.md` + 设计文档 §4/§6/§7。
- 两端数据契约：以 `control-plane` 的 OpenAPI/gRPC 定义为准，前端不得臆造接口。
