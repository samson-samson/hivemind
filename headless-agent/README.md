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
| `goodputai-dev` | 应用日志（gateway 等，按集群 namespace 过滤） |
| `k8s-log-*` | k8s 控制面日志（apiserver/audit/scheduler/kcm/ccm，带集群后缀） |

所有数据采集仅使用 List/Get 类只读 API。

## 用法

```bash
python3 diagnose.py --list              # 列出最近事故
python3 diagnose.py                     # 诊断最近一条（默认事故路径）
python3 diagnose.py --index 5           # 诊断第 5 条
python3 diagnose.py --alert <name>      # 从指定告警出发诊断（最常用）
python3 diagnose.py --report /tmp/r.md  # 同时输出 markdown + json
python3 diagnose.py --push <inc_id>     # 假设写入控制平面（会议室可见）
```

## 防串房 / 防误导（测试环境关键机制）

测试环境告警特别多且混杂，本采集器内建三层防护：

1. **环境锁定**：从告警的 `alarmObjects`（权威）优先解析集群环境，降级才正则告警名，
   据此过滤应用日志到本环境 namespace。`--alert dev-prod-同名子串` 不会跨环境串房
   （如 prod 事故不再把 dev + casdoor 日志当证据）。
2. **告警召回**：告警采集 `line=120`，覆盖一个完整 168h 窗口的全部告警（实测 ~100 条），
   避免高频告警（如 gpu-utilization 31 条）挤掉低频告警（排在第 79 位的 dev-worker）。
3. **告警质量标注**：诊断输出 `alert_quality`，识别并标注
   - 是否测试告警（delivery-test/投递验证回环）
   - 严重度是否虚高（level 配置与真实影响不匹配）
   - 是否路由重复（被多个通知策略重复触发、被重复计数放大）
   IC 据此分流"生产事故"与"测试噪声"，不被测试环境噪声误导。

## 流程

```
告警/事故(SLS) → 锁环境(alarmObjects→namespace)
  → 采集上下文(关联告警 + gateway错误 + k8s控制面错误)
  → GLM-5.2 诊断 → 结构化 JSON
  → 报告(症状/分层假设+置信度/矛盾排除/建议行动分级/⚠告警质量/证据盲区)
  → (可选) push 假设到控制平面
```

## 案例（2026-08-18 实测，dev 环境全量验证）

| 告警 | 环境 | 诊断结论 | 告警质量 |
|---|---|---|---|
| `inference-hang-worker(-dev)` | dev | P2，识别 flap 模式 + k8s 控制面 etcd 连接抖动→worker 挂起新假设 0.3 | 路由重复 ✅ |
| `inference-hang-frontend(-dev)` | dev | P2，区分前端连接 vs 后端引擎 | 路由重复 ✅ |
| `xid-errors` | dev | P1，网关 60s 超时取消主因 0.75 | — |
| `delivery-test-*` | dev | P3（降级），识别为 ARMS 投递验证测试 | 测试告警 ✅ / 严重度虚高 ✅ / 路由重复 ✅ |

**k8s 控制面证据的价值**：inference-hang 类诊断此前一直缺"集群层面证据盲区"，
接入 apiserver/kcm 日志后，模型挖到根因新方向——
`kube-apiserver 失败连接 zz-etcd0:2379` → 控制面间歇性不可用 →
kubelet 健康检查异常 → worker 挂起。没有 k8s 证据层推不出这条链路。

> **决策边界**：AI 完成根因定位与辅助分析；止血/修复方案必须由 IC 决策，AI 不自动执行任何写动作。
