# P0 后端实施简报（deepseek-v4-flash）

> 角色：后端编码 · 模型：`deepseek/deepseek-v4-flash` · 契约：`docs/superpowers/specs/2026-08-12-hivemind-design.md`（v2.1）
> **开工前必读**：设计文档 §3（架构）/ §4（IOM）/ §5（协议）/ §10（阿里云映射）。本简报自包含 P0 范围。

## 0. P0 定位：只读协同账本

**范围**：工作图 + 查询去重 + 证据血缘 + 人工 IC + 指挥室 v1（K8s + Prometheus + 一个 CMDB pack）。
**硬边界**：**P0 无任何写路径**——没有修复执行、没有 runbook 自动执行、没有自动 skill 生成。威胁模型（设计 §9）未落地前不许开写路径。

**验收**：同事故查询级去重率、单位工具成本信息增益、决策延迟可度量（出指标接口）。

## 1. 技术栈

- Go 1.23+ · 标准库 + `errgroup` + 一个 HTTP 框架（chi/gin 均可）+ gRPC（内部）
- 存储 P0 务实选型：**PostgreSQL**（关系 + 边表模拟图语义），证据总线用 append-only 表 + WAL。不引入图数据库（P1 再换 Neo4j/GDB）。
- 消息：内存事件总线即可（P0 单实例）；接口层出 SSE 供前端实时。

## 2. 模块清单（每个模块独立可测）

```
control-plane/
├── cmd/server/            # 入口：HTTP(gRPC-Gateway) + SSE
├── internal/iam/          # IOM 数据模型 + 存储（核心）
│   ├── model.go           # 节点/边类型定义（见 §3）
│   ├── store.go           # 图存储接口（PostgreSQL 实现）
│   └── fingerprint.go     # 事故指纹
├── internal/ingest/       # 告警进入：webhook → IOM 归一化
│   ├── alertmanager/      # Prometheus Alertmanager webhook
│   ├── k8s/               # K8s 事件/状态（只读）
│   └── cmdb/              # 资源与拓扑（只读，pack 抽象）
├── internal/workgraph/    # 工作图服务
│   ├── service.go         # 建/改/查工作单元，IC 调整
│   └── roles.go           # explorer/verifier/skeptic/executor
├── internal/lease/        # 咨询性租约
│   ├── service.go         # claim/heartbeat/expiry（advisory，非独占）
│   └── staleness.go       # 晚到结果标记 stale
├── internal/querycoord/   # 查询协调器（去重核心）
│   ├── fingerprint.go     # 操作指纹生成（见 §4）
│   └── singleflight.go    # 同指纹 single-flight + 兼容复用
├── internal/evidence/     # 证据总线
│   ├── ledger.go          # append-only 账本
│   └── lineage.go         # 证据血缘 DAG + 独立性评分
├── internal/ic/           # 人工 Incident Commander
│   └── guidance.go        # IC 决策/发言（Guidance 节点）
├── internal/api/          # OpenAPI/gRPC 定义 + handler
└── internal/stats/        # P0 指标：去重率/信息增益/决策延迟
```

## 3. IOM 数据模型（P0 子集）

节点（全部带 `id / source / timestamp / proof_trace`）：

| 类型 | 关键字段 |
|---|---|
| `Incident` | fingerprint, status, ic_id, time_range, alert_ids, symptom_set |
| `WorkNode` | question, scope, expected_evidence, cost, deadline, assignee, role, status, lease_id |
| `Operation` | fingerprint, registered_at, dedup_status(`single_flight\|reused\|fresh`), result_ref |
| `Evidence` | operation_id, lineage_dag, source, result, conclusion, independence_score |
| `Fact` | statement, evidence_chain, confirmed_by, is_confirmed |
| `Hypothesis` | topic, supporting[], refuting[], independence_weight, confidence, status |
| `Guidance` | from_ic, text, priority |

边：`has_symptom · involves_entity · executes(→Operation) · derived_from(证据血缘) · supports/refutes(→Hypothesis) · decides(→WorkNode)`

**规则**：任何节点的 `proof_trace` 必须可回放（指向证据账本的原始记录）。这是 P0 审计底线。

## 4. 操作指纹（去重的核心）

查询前由 `querycoord` 生成规范化指纹，**完全相同查询 single-flight，兼容查询复用，需新鲜度才重查**：

```json
{
  "target": "ack-xxx-cluster/k8s/pod/foo-7b9c",
  "data_source": "prometheus|sls|k8s|cmdb",
  "time_window": {"start": "...", "end": "..."},
  "query_ast": "..." ,            // 规范化查询 AST（含过滤条件）
  "tenant": "acme",
  "tool_version": "kubectl-v1.31",
  "data_snapshot": "..."
}
```

**独立性评分**（防从众）：按采集链路/权限域/底层数据源计算，**不按 agent 数**。同一数据源的重复观测不计叠加。

## 5. API 契约（前端 kimi-k3 依赖，P0 必备）

```
POST   /api/v1/incidents                      # 手动创建或 webhook 归一化
GET    /api/v1/incidents                      # 列表（指纹/状态/IC）
GET    /api/v1/incidents/{id}                 # 详情（含 symptom_set）
GET    /api/v1/incidents/{id}/context@vN      # 上下文包（工作项+证据+反证清单）
POST   /api/v1/incidents/{id}/work-nodes      # 建/调工作单元
GET    /api/v1/incidents/{id}/work-nodes      # 工作图 DAG
POST   /api/v1/incidents/{id}/leases          # 认领（advisory lease）
POST   /api/v1/incidents/{id}/leases/{lid}/heartbeat
DELETE /api/v1/incidents/{id}/leases/{lid}
POST   /api/v1/incidents/{id}/operations      # 登记查询（single-flight，返回去重结果）
POST   /api/v1/incidents/{id}/evidence        # 推证据（带血缘）
GET    /api/v1/incidents/{id}/evidence        # 证据/血缘/独立性
POST   /api/v1/incidents/{id}/guidance        # IC 发言/决策
GET    /api/v1/incidents/{id}/stats           # 去重率/信息增益/决策延迟
GET    /api/v1/incidents/{id}/events          # SSE 实时事件流
```

**约定**：REST/JSON + SSE；gRPC 仅内部。前端不得臆造接口，以 `api/openapi.yaml` 为准。

## 6. 验收标准

- [ ] 同一事故内，完全相同查询被 single-flight 压掉，`stats` 可量化"无意重复率"
- [ ] 证据带血缘 DAG + 独立性评分，可回放
- [ ] 工作图 CRUD + 咨询性租约（心跳超时释放；晚到结果标记 stale 不污染）
- [ ] IC 可建事故、调整工作图、发 Guidance
- [ ] K8s + Prometheus 告警 webhook 接入 → 自动归一化为 Incident
- [ ] 全程只读，无任何写操作代码路径

## 7. 明确不做（P1+）

蒸馏引擎 / 知识图谱检索 / runbook 认证 / 修复执行 / 能力 descriptor 协商 / 威胁模型落地。这些在 P1-P4。
