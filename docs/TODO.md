# Hivemind TODO（来自 Codex 审查 2026-08-17 + 路线图）

> 已修：批 A（v3 指挥室）/ B（分叉签名+frozen）/ C（诊断串房、fact 门控、心跳、
> 枚举、Store 深拷贝）/ E（work-node 契约、SSE 信封、auto_diagnose 契约、
> 指纹折叠、stale 过滤）

## P0（未完成，优先）

- [ ] **认证与 RBAC**（§9.1-9.2）：路由级鉴权中间件、事故级成员资格、IC 身份从
      服务端凭证解析（禁止客户端自报 `from_ic`/`source`）、租户强制作用域
- [ ] **诊断端点认证**（§8）：`POST /diagnose` 未认证即可触发子进程（读本机云
      凭证/调收费 LLM）——至少加 token 校验 + 频控
- [ ] **PMA 完整性**（§5.4）：propose→merge→adjudicate 状态机落地（提案指纹、
      冲突触发、仲裁记录、终止条件）；当前 hypothesis 直写无提案层

## P1

- [ ] **PG 持久化**（§9.5）：`Store` 的 PostgreSQL 实现（`DATABASE_URL` 切换）；
      当前内存重启丢数据
- [ ] **SSE 接入会议室**（§7）：前端 10s 轮询 → EventSource 增量合并；服务端
      event ID + Last-Event-ID 重放；修 seq 假语义（服务端单调）
- [ ] **getContext 快照**：前端并发拼 5 列表 → 后端 context@vN 单快照 + diff
- [ ] **事件总线丢事件**（bus.go）：注释与实现相反（丢最新非最早）；加丢事件
      计数与告警
- [ ] **无界列表**（§8）：分页、限流、SSE 连接上限、IdleTimeout
- [ ] **独立性并发**（§5.4）：同源证据并发评分丢失更新——串行化事务

## P2

- [ ] **蒸馏两流水线**（§5.2/5.6）：事故中候选（episode/timeline）→ 事故后认证
      （类型化 runbook + replay/canary 门禁）
- [ ] **OpenAPI DTO 生成**（§11.2）：api/openapi.yaml → 前端类型生成，消灭手写
      normalize 漂移（状态枚举/cost 等级 vs 整数/proof_trace 类型）
- [ ] **话题状态机落地**（§5.9）：信息增益判定、PLATEAU、四重预算、NO_NOVELTY
      冻结语义（模型字段已加 frozen）
- [ ] **claim verdict**（§5.10）：六层判定记录 + agent 声誉分场景校准
- [ ] **修复闭环**（§5.3/P4）：typed action + 护栏引擎 + 双人审批 + 回滚 +
      Chaos 复验（威胁模型落地后）

## 生态

- [ ] **Agent Conformance Suite**（§11.7）：越权/伪造IC/重复请求/租约/断连八项
- [ ] **Skill/runbook 市场**（§11.8）：manifest 校验、发布流水线、签名/SBOM
- [ ] **opshive swarm backend 联跑**：OACP 信封 → 控制平面真实镜像（当前原型）
