# Hivemind headless-agent（智能诊断）

平台自带的"数字员工"：只读采集阿里云 SLS 数据 → LLM 智能诊断 → 结构化报告。

## LLM 配置（复用本机 hermes）

读取 `~/.hermes/config.yaml` 与 `~/.hermes/.env`：

| 项 | 值 |
|---|---|
| base_url | `https://api.goodputai.cn/v1`（OpenAI 兼容） |
| API key | `GOODPUTAI_API_KEY`（hermes .env） |
| 默认模型 | `z-ai/glm-5.2`（可用 `OPSHIVE_LLM_MODEL` 覆盖） |

## 数据源（只读）

| 项目 | 内容 |
|---|---|
| `itsm-1987933858527387-cn-hangzhou` | 事故记录 + Alertmanager 告警事件 |
| `cms-alert-center-*` | CMS/ARMS 告警事件流 |
| `goodputai-dev` | 应用日志（gateway/compute-sglang/control-dynamo/…） |
| `k8s-log-*` | 集群日志（apiserver/audit/…） |

所有数据采集仅使用 List/Get 类只读 API。

## 用法

```bash
python3 diagnose.py --list              # 列出最近事故
python3 diagnose.py                     # 诊断最近一条
python3 diagnose.py --index 5           # 诊断第 5 条
python3 diagnose.py --report /tmp/r.md  # 输出 markdown + json
```

## 流程

```
事故记录(itsm) → 采集上下文(告警+错误日志) → GLM-5.2 诊断
→ 结构化 JSON：症状/分层假设(置信度)/矛盾排除/建议行动(分级)/证据盲区
```

## 案例（2026-08-13 实测）

真实事故：`litellm` Deployment 2 副本 16 分钟不可用（12 次告警）。
诊断结论（GLM-5.2）：① CrashLoopBackOff 0.55 —— OTel 日志导出 gRPC Unimplemented 错误若未容错可能致崩溃；② 资源不足 0.30；③ 镜像/配置 0.20。排除了跨集群 GPU 告警的误关联（反幻觉验证）。建议 verify 优先级行动 + 分级修复。完整报告见 `/tmp/hivemind-diagnosis.md`。
