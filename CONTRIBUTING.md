# 贡献指南（CONTRIBUTING）

欢迎为 Hivemind 贡献代码、文档、pack 与经验。本项目是**开源 + 参考实现**，云中立，阿里云为第一实现。

## 三方模型分工（本项目的协作方式）

| 角色 | 模型 | 负责 |
|---|---|---|
| 设计 / 评审 | `anthropic/claude-opus-5` | 架构、协议、评审（设计文档即契约） |
| 前端编码 | `moonshotai/kimi-k3` | `console/` |
| 后端编码 | `deepseek/deepseek-v4-flash` | `control-plane/`、`adapter-*`、`packs/`、`conformance/` |

原则：**设计文档是唯一契约**；三方并行、互不等待；合拢由协调者（main）做契约对账。

## 快速开始

```bash
# 后端（Go 1.23+）
cd control-plane && go build ./... && go test ./...

# 前端（Node 20+）
cd console && npm install && npm run build

# 智能诊断（Python 3.10+，需要本机 LLM 配置）
cd headless-agent && python3 diagnose.py --list
```

## 提 PR 前

1. **读设计文档**：`docs/superpowers/specs/2026-08-12-hivemind-design.md`（权威契约）。
2. **跑两端验证**：`go build/vet/test` + `npm run build` 全绿。
3. **遵守四条铁律**（见 CLAUDE.md）：排障不是互斥写入；发生过≠可复用；不造 agent 引擎；写操作永远人审。
4. **凭证纪律**：任何 AK/SK、token、密钥**不得**进入仓库。用 `.env.example` 声明，真值走环境变量。

## 提交规范

- 单次提交一个逻辑变更；`feat:` / `fix:` / `docs:` / `chore:` 前缀。
- 后端补测试（`go test`），前端保持 `npm run build` 零告警。
- PR 描述说明：改了什么、验证了什么、影响哪些契约（API/协议）。

## 开发流程（多智能体协作）

```
设计评审（opus-5）→ 拆 P0 简报（前端 kimi-k3 / 后端 deepseek-v4-flash）
  → 并行编码 → 协调者契约对账（api/openapi 对齐）→ 端到端验证 → 提交
```

每个 P 期（P0 只读协同账本 → P1 评测检索 → P2 候选生成 → P3 强类型动作）都有独立验收标准，见设计文档 §12。

## 报告问题

- Bug：给出复现步骤 + 两端版本 + 日志摘录。
- 协议/API 变更：先在设计文档更新契约，再动代码。
- 安全漏洞：**不要**开公开 issue，走 SECURITY.md 的流程。
