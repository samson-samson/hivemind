# Hivemind（运维蜂巢）

AIOps 多智能体故障协同平台：把 N 个工程师本地的 Claude Code / Codex 变成"一个互相感知、去重、持续蒸馏、可视化指挥的分布式排障大脑"。

- **不造 agent 引擎**——复用本地 Claude Code/Codex，平台只做协同 + 知识。
- **四大支柱**：全局去重协同 · 知识沉淀 · 指挥室可视化 · 护栏内修复闭环。
- **云中立，阿里云为第一实现**（SLS/ARMS/ACK/ChaosBlade）。
- **开源项目 + 参考实现**。

> 设计文档（权威契约）：`docs/superpowers/specs/2026-08-12-hivemind-design.md`
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
