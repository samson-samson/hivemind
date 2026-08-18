# Hivemind（运维蜂巢）

> **不造引擎，造蜂群。** / *Don't build another agent. Build the hive.*

AIOps 多智能体故障协同平台：把 N 个工程师本地的 Claude Code / Codex 变成"一个互相感知、去重、持续蒸馏、可视化指挥的分布式排障大脑"。

**宣传页（中英双语）**：[docs/design/hivemind-promo.html](docs/design/hivemind-promo.html) · 设计文档（权威契约）：`docs/superpowers/specs/2026-08-12-hivemind-design.md`

## 为什么是 Hivemind / Why

你的团队已经有 agent 了——每个工程师桌面上都跑着本地 Claude Code / Codex。问题不是"agent 不够强"，而是 **agent 之间没有共享思考的基础设施**：

| 痛点 | 现状 | Hivemind 的答案 |
|---|---|---|
| 同样的查询五个人各跑一遍 | token 和时间成倍烧掉 | **查询级去重**：操作指纹 single-flight 合并 |
| 结论互相矛盾，没有共同事实 | 只能靠嗓门说服 | **证据先于聊天**：结构化证据进事实层，聊天不进 |
| 排过的障下次重来 | 知识死在个人会话里 | **发生过 ≠ 可复用**：双流水线蒸馏 + 认证链 |
| 只有建议，没有可审计决策 | 责任甩给疲惫的 on-call | **写操作永远人审**：证据门禁 + dry-run 默认 |

## 四大支柱 / Four pillars

1. **证据先于聊天** — 讨论可以无限，事实层必须有界。根因真假归人裁决。
2. **查询级去重** — 一次查询全队受益。排障不是互斥写入：无意重复最小化 + 受控冗余交叉验证。
3. **知识蒸馏** — 事故中只产候选，事故后认证；认证链 candidate→verified→certified→revoked。
4. **恢复闭环** — AI 定位，人拍板，机器执行必须有证据；无威胁模型不进写路径。

## 我们不做什么 / What we refuse to do

✕ 不造 agent 引擎（复用本地 Claude Code/Codex，薄 MCP 适配器） · ✕ 不做"全自主专家团"（默认受 IC + PMA + 证据门控约束） · ✕ 不让 LLM 直接发布知识（候选必须过认证链） · ✕ 无威胁模型处开放写路径（P0 只读）。

## 已跑通 / Verified end-to-end

真实阿里云 SLS 告警 → 自动开独立会议室 → headless-diagnoser 读真实日志 → LLM（GLM-5.2）产出结构化假设 → IC 决策 → 蒸馏认证 runbook → 相似事故秒级召回（词级 Jaccard）→ 恢复动作证据门禁 + dry-run。三闭环全部本机实测，零生产副作用。

- **云中立，阿里云为第一实现**（SLS/ARMS/ACK/ChaosBlade）。
- **开源项目 + 参考实现**，Apache-2.0。

> 开发约定与模型分工：`CLAUDE.md`

## 目录

```
control-plane/    Go 控制平面：IOM、工作图、查询协调、证据总线、护栏、安全
distiller/        Python：候选蒸馏、认证流水线、知识图谱
adapter/          MCP 适配器「发言人」+ 各 agent 接入示例
headless-agent/   无头调查员（数字员工）
console/          React/TS 指挥室前端
packs/            预制环境包：aliyun(旗舰) · k8s · prometheus · aws · azure · sls
adapter-protocol/ Capability Descriptor（gRPC + OpenAPI）
adapter-sdk/      connector 插件 SDK
adapter-generator/ descriptor → connector 生成器
conformance/      connector 验收套件
infra/            Terraform（阿里云）+ Helm
docs/             设计文档 + 实施简报
```

## 路线图

P0 只读协同账本（当前）→ P1 评测检索 → P2 候选生成 → P3 强类型可回滚动作 → P4 生态。
