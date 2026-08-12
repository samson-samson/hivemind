# P0 前端实施简报（moonshotai/kimi-k3）

> 角色：前端编码 · 模型：`moonshotai/kimi-k3` · 契约：`docs/superpowers/specs/2026-08-12-opshive-design.md`（v2.1）
> **开工前必读**：设计文档 §3（架构）/ §6（MCP 接入，了解信息源）/ §7（指挥室）。本简报自包含 P0 范围。
> **后端数据契约**：`docs/implementation/P0-backend-brief.md` §5（API 契约）——**以 control-plane 的 `api/openapi.yaml` 为准，不要臆造接口**。

## 0. P0 定位：指挥室 v1（只读协同账本的"眼睛"）

**范围**：事故总览 + 工作图 + 证据血缘图 + 协同态势 + 协商视图 + 时间线 + 知识面板 + 上下文飞轮。
**硬边界**：**P0 全是只读展示 + IC 基本操作（建事故/调工作图/发 Guidance）**。无审批执行 UI、无写操作 UI。

**验收**：一名 IC 打开控制台，能实时看到 N 个 agent 在查什么、哪里重复了、证据怎么汇聚、决策延迟多少。

## 1. 技术栈

- **Vite + React 18 + TypeScript**（严格模式）
- 路由：react-router · 状态：TanStack Query（服务端状态）+ zustand（UI 态）
- 图/拓扑：**reactflow**（工作图 DAG、证据血缘图）+ dagre 布局
- 实时：SSE（`/api/v1/incidents/{id}/events`）+ TanStack Query 的 SSE 集成
- 样式：Tailwind CSS + 一个轻量组件基座（不引入重型 UI 框架）
- 深色优先（运维大屏惯例），响应式

## 2. 页面与视图

```
console/src/
├── app/                 # 路由 + 布局（侧栏：活跃事故列表）
├── features/
│   ├── incident-list/   # 事故总览：指纹/状态/IC/告警集
│   ├── work-graph/      # 工作图 DAG：节点=工作单元，边=依赖/顺序
│   ├── evidence-lineage/# 证据血缘图：Operation→Evidence→Fact/Hypothesis
│   ├── swarm-view/      # 协同态势：agent 活动流 + 无意重复热力图 + 租约状态
│   ├── deliberation/    # 协商视图：提案/仲裁流（P0 展示结构，投票在 P1）
│   ├── timeline/        # 生命周期时间线：告警→分诊→调查→候选
│   ├── knowledge-panel/ # 知识面板：历史命中（P0 为 stub）+ 候选占位
│   └── ic-controls/     # IC 操作：建事故/调工作图/发 Guidance
└── lib/api/             # 生成的 API client（openapi-typescript）
```

### 2.1 视图要点

| 视图 | 关键交互 | 数据来源 |
|---|---|---|
| 事故总览 | 点击进详情；状态/IC/指纹展示 | `GET /incidents` |
| 工作图 | DAG 布局；节点显示 question/成本/负责人/租约态；IC 可拖拽调整 | `GET /incidents/{id}/work-nodes` |
| 证据血缘图 | 从 Operation 到 Fact/Hypothesis 的生长动画；独立性评分着色 | `GET /incidents/{id}/evidence` |
| 协同态势 | 热力图按 entity×时间窗着色，重复查询高亮为"已去重"；租约心跳动画 | `GET /incidents/{id}/leases` + events |
| 协商视图 | 提案列表 + 冲突标记 + 升级给谁（P0 只读结构） | events + evidence |
| 时间线 | 全生命周期横轴，点击节点定位到对应证据/工作单元 | `GET /incidents/{id}/events` |
| 知识面板 | 历史 runbook 命中卡片（P0 返回空集占位） | `GET /incidents/{id}/knowledge`（P1 提供，先留接口） |
| 上下文飞轮 | 顶部指示器：已确认事实数 / 未解问题数 / 决策延迟，随时间推进"飞轮在加速" | `GET /incidents/{id}/stats` |

## 3. 实时更新机制

- 连接 `GET /incidents/{id}/events`（SSE），事件类型：`incident.updated / work_node.updated / lease.changed / evidence.added / fact.confirmed / guidance.added`。
- 前端做**增量合并**（不是整页刷新）：新证据只在血缘图上追加节点，工作单元只更新受影响卡片。
- 断线重连 + 事件序号去重（后端保证单调）。

## 4. 关键设计原则（来自设计文档 §7）

- **现有产品画"事故"，我们画"人 × agent 的协同过程本身"**——协同态势/工作图/血缘图是主角，不是事故详情页的附件。
- 所有展示区分**事实**（`Fact`，已确认）与**假设**（`Hypothesis`，带置信度）——视觉上必须有区分，防误读。
- 复用检测要"可见"：被 single-flight 压掉的重复查询要以"已去重"形式留在热力图上，证明去重有效。

## 5. 验收标准

- [ ] 8 个视图齐全，SSE 实时增量更新不整页刷新
- [ ] 工作图 DAG + 证据血缘图可交互（缩放/拖拽/点击联动）
- [ ] 事实/假设视觉严格区分
- [ ] 热力图可一眼看出"哪里重复、哪里已去重"
- [ ] IC 三项操作（建事故/调工作图/发 Guidance）可用
- [ ] 深色主题、无横向滚动、控制台尺寸下布局合理

## 6. 明确不做（P1+）

审批执行流 · 提案投票 · 因果图算法 · runbook 展示（P2）· 移动端适配（P3）。
