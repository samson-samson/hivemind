# 安全策略（SECURITY）

## 报告漏洞

**不要**在公开 issue 中披露安全漏洞。请发邮件至维护者（仓库 SECURITY 声明中的地址），或经 GitHub 私有漏洞披露流程。

## 凭证纪律（强制）

- 任何 AK/SK、API token、密码**严禁**提交进仓库（包括测试、日志、seed 脚本）。
- 环境变量承载真值；仓库内只用 `.env.example` 声明变量名与占位符。
- 提交前运行凭证审计：

```bash
git grep -InE '(AKID|LTAI|sk-|Bearer [A-Za-z0-9]{16,}|password\s*=\s*["'"'"'][^"'"'"']+)' -- . ':!*.lock'
```

## 威胁模型摘要（详见设计文档 §9）

| 威胁 | 对策 |
|---|---|
| prompt injection（日志/runbook/MCP 输出带毒） | 外部内容按"未信任数据"标注；仅结构化消息进 IOM |
| 证据投毒（proof_trace 篡改） | 证据总线哈希链 + append-only |
| 越权动作 | 三层安全：行为沙箱 / 最小权限（STS 按事故临时授予）/ 出站管控 |
| AI 自作主张 | 写操作永远人审；`decision` 类消息仅 IC 可发（OACP §3） |
| 跨机 bridge 消息注入 | 沿用 Agent Swarms 的 safetyCheck（显式用户同意，classifier 不可绕过） |
| 插件/pack 供应链 | 签名 + 锁版本 + SBOM + 紧急撤销 |

## 数据分级

- **原始日志**：留在客户侧 gateway（数据不出客户网络）。
- **结构化证据**（Evidence/Fact/Hypothesis）：可上报控制平面，用于协同与知识蒸馏。
- PII/密钥：出侧前脱敏管道处理。
