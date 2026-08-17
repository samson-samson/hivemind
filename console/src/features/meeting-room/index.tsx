import { useCallback, useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useParams } from 'react-router-dom';
import { api } from '../../lib/api';
import type { Hypothesis } from '../../lib/api';
import { fmtTime, fmtPct } from '../../lib/format';

/**
 * 指挥室（v3）—— 一个故障一间会议室，按"指挥界面"而非"活动流"组织：
 *
 *   顶部：影响 / 止血状态 / 仲裁倒计时 / 上下文版本
 *   左：  工作图（agent 是工作节点属性，不是独立名单）
 *   中：  事实账本（证据/事实泳道）+ 盲区 + 冲突 + 下一最佳动作
 *   右：  假设对比矩阵（状态/独立证据域/反证/可证伪预测）+
 *         IC 决策队列（类型化）+ 权限三级条
 *   底部：活动流抽屉（审计信息，降级）
 *
 * 设计依据：docs/design/hivemind-command-room.html（v3 样式图）。
 * 规则：AI 只定位，IC 决策；聊天不进事实层；置信度是弱元数据。
 */

const HYP_STATE: Record<string, { label: string; cls: string }> = {
  open: { label: 'PROPOSED', cls: 'hs prop' },
  strengthening: { label: 'SUPPORTED', cls: 'hs sup' },
  weakening: { label: 'CONFLICTED', cls: 'hs conf' },
  refuted: { label: 'REFUTED', cls: 'hs ref' },
  confirmed: { label: 'FROZEN', cls: 'hs frz' },
};

function WorkGraphCol({ incidentId }: { incidentId: string }) {
  const { data: nodes } = useQuery({
    queryKey: ['work-nodes', incidentId],
    queryFn: () => api.listWorkNodes(incidentId),
  });
  const { data: leases } = useQuery({
    queryKey: ['leases', incidentId],
    queryFn: () => api.listLeases(incidentId),
  });
  // agent 是节点属性：租约 holder → 节点 assignee
  const byHolder = new Map<string, string>();
  for (const l of leases ?? []) byHolder.set(l.work_node_id, l.holder);

  return (
    <div className="col col-left">
      <div className="col-label">工作图 <span className="count">· {(nodes ?? []).filter((n) => n.status === 'done').length}/{(nodes ?? []).length}</span></div>
      {(nodes ?? []).map((n, i) => (
        <div key={n.id}>
          {i > 0 && <div className="wg-edge" />}
          <div
            className={`wg-node ${n.status === 'done' ? 'done' : ''} ${n.status === 'in_progress' ? 'active' : ''} ${n.role === 'skeptic' ? 'skeptic' : ''}`}
          >
            <div className="wg-q">{n.question}</div>
            <div className="wg-meta">
              <span className="agent">{n.assignee ?? byHolder.get(n.id) ?? '待认领'}</span>
              <span className="role">{n.role.toUpperCase()}</span>
              {n.role === 'skeptic' && n.status === 'pending' && <span className="auto-tag">自动生成</span>}
              <span className="budget">cost {n.cost}</span>
              {n.lease_id && <span className="lease">租约 {n.lease_id.slice(0, 8)}</span>}
              <span className="status">{n.status}</span>
            </div>
          </div>
        </div>
      ))}
      {(nodes ?? []).length === 0 && <div className="wg-add">工作图为空 — IC 可新建工作单元</div>}
    </div>
  );
}

function FactLedger({ incidentId }: { incidentId: string }) {
  const { data: evidence } = useQuery({ queryKey: ['evidence', incidentId], queryFn: () => api.listEvidence(incidentId) });
  const { data: facts } = useQuery({ queryKey: ['facts', incidentId], queryFn: () => api.listFacts(incidentId) });
  const { data: hypotheses } = useQuery({ queryKey: ['hypotheses', incidentId], queryFn: () => api.listHypotheses(incidentId) });
  const { data: nodes } = useQuery({ queryKey: ['work-nodes', incidentId], queryFn: () => api.listWorkNodes(incidentId) });

  const conflicts = (hypotheses ?? []).filter((h) => h.supporting.length > 0 && h.refuting.length > 0);
  const openGaps = (hypotheses ?? []).filter((h) => h.status === 'open' && h.supporting.length === 0).slice(0, 3);
  const unclaimed = (nodes ?? []).filter((n) => n.status === 'pending' && !n.assignee);
  const topHyp = (hypotheses ?? []).sort((a, b) => b.confidence - a.confidence)[0];
  const nextAction: { kind: 'node'; question: string; role: string } | { kind: 'hyp'; topic: string; confidence: number } | null =
    unclaimed[0]
      ? { kind: 'node', question: unclaimed[0].question, role: unclaimed[0].role }
      : topHyp
        ? { kind: 'hyp', topic: topHyp.topic, confidence: topHyp.confidence }
        : null;

  return (
    <div className="col col-mid">
      <div className="col-label">
        事实账本 <span className="count">· 证据 {(evidence ?? []).length} · 事实 {(facts ?? []).length}</span>
      </div>

      {(facts ?? []).slice(0, 3).map((f) => (
        <div className="ledger-item" key={f.id}>
          <div className="ledger-rail"><span className="ledger-dot ok" /></div>
          <div className="ledger-card fact">
            <div className="ledger-head">
              <span className="ledger-tag fact">事实</span>
              <span className="ledger-src">{f.confirmed_by}</span>
              <span className="ledger-time">{fmtTime(f.confirmed_at)}</span>
            </div>
            <div className="ledger-body">{f.statement} <span className="ref">{f.evidence_ids.length} 证据链</span></div>
          </div>
        </div>
      ))}
      {(evidence ?? []).slice(0, 3).map((e) => (
        <div className="ledger-item" key={e.id}>
          <div className="ledger-rail"><span className="ledger-dot sky" /></div>
          <div className="ledger-card evidence">
            <div className="ledger-head">
              <span className="ledger-tag ev">证据</span>
              <span className="ledger-src">{e.source} · {e.operation_id.slice(0, 8)}</span>
              <span className="ledger-time">{fmtTime(e.timestamp)}</span>
            </div>
            <div className="ledger-body">{e.summary} · 独立性 <span className="indep">{e.independence_score.toFixed(2)}</span></div>
          </div>
        </div>
      ))}
      {(facts ?? []).length === 0 && (evidence ?? []).length === 0 && (
        <div className="empty-ledger">账本为空 — agent 提交证据后此处实时生长</div>
      )}

      {conflicts.length > 0 && (
        <div className="conflict-box">
          <span className="h"><span className="signal-mark filled" />开放冲突 {conflicts.length}</span>
          <div style={{ marginTop: 4 }}>{conflicts.map((c) => c.topic.slice(0, 60)).join('；')} — 等待反证或进入仲裁</div>
        </div>
      )}

      {(openGaps.length > 0 || unclaimed.length > 0) && (
        <div className="gap-box">
          <span className="h"><span className="signal-mark" />未知项 / 盲区</span>
          <ul>
            {unclaimed.map((n) => <li key={n.id}>未认领：{n.question.slice(0, 40)}</li>)}
            {openGaps.map((h) => <li key={h.id}>无证据假设：{h.topic.slice(0, 40)}</li>)}
          </ul>
        </div>
      )}

      {nextAction && (
        <div className="next-action">
          <div className="h"><span className="signal-mark filled" />下一最佳调查动作</div>
          {nextAction.kind === 'node' ? (
            <><div className="v">{nextAction.question}</div><div className="who">建议认领 · {nextAction.role}</div></>
          ) : (
            <><div className="v">验证假设：{nextAction.topic.slice(0, 60)}</div><div className="who">置信 {fmtPct(nextAction.confidence)}</div></>
          )}
        </div>
      )}
    </div>
  );
}

function HypothesisMatrix({ incidentId, onArbitrate }: { incidentId: string; onArbitrate: (h: Hypothesis) => void }) {
  const { data: hypotheses } = useQuery({ queryKey: ['hypotheses', incidentId], queryFn: () => api.listHypotheses(incidentId) });
  return (
    <div className="hyp-table-wrap">
    <table className="hyp-matrix">
      <thead>
        <tr><th>假设</th><th>独立证据域</th><th>反证</th><th>可证伪预测</th></tr>
      </thead>
      <tbody>
        {(hypotheses ?? []).sort((a, b) => b.confidence - a.confidence).map((h) => {
          const st = HYP_STATE[h.status] ?? HYP_STATE.open;
          const conflicted = h.supporting.length > 0 && h.refuting.length > 0;
          return (
            <tr key={h.id} className={h.status === 'refuted' ? 'refuted' : conflicted ? 'conflicted' : ''}>
              <td>
                <span className={st.cls}>{st.label}</span>
                <span className="hs-name">{h.topic}</span>
                <div className="confidence-row weak">
                  <span>置信 {fmtPct(h.confidence)}</span>
                  <span className="confidence-track" aria-hidden="true">
                    <span className="confidence-fill" style={{ width: `${Math.max(0, Math.min(100, h.confidence * 100))}%` }} />
                  </span>
                </div>
                <div className="weak">{h.proposed_by} · {fmtTime(h.updated_at)}</div>
              </td>
              <td><span className="lineage-tag">{h.independence_weight > 0.6 ? '2-3 lineage' : h.independence_weight > 0.3 ? '1-2 lineage' : '1 lineage'}</span></td>
              <td className="weak">{h.refuting.length}{conflicted ? `（agent-c 反证中）` : ''}</td>
              <td className="falsifier">{h.status === 'refuted' ? '—' : <button className="arb-btn" onClick={() => onArbitrate(h)}>仲裁 / 反证</button>}</td>
            </tr>
          );
        })}
        {(hypotheses ?? []).length === 0 && (
          <tr><td colSpan={4} className="weak">暂无假设 — 点击"AI 诊断"或等 agent 进场</td></tr>
        )}
      </tbody>
    </table>
    </div>
  );
}

export function MeetingRoom() {
  const { incidentId = '' } = useParams();
  const queryClient = useQueryClient();

  // 实时性：SSE 事件驱动刷新为主，60s 长轮询兜底（断连/事件丢失时保底）。
  const REFRESH = { refetchInterval: 60_000 };
  // SSE 实时订阅：事件到达 → invalidate 对应实体（增量合并由 query 重取完成）。
  const onEvent = useCallback(
    (ev: { type: string }) => {
      const keyMap: Record<string, string[]> = {
        'work_node.created': ['work-nodes', incidentId],
        'work_node.updated': ['work-nodes', incidentId],
        'evidence.appended': ['evidence', incidentId],
        'fact.posted': ['facts', incidentId],
        'hypothesis.posted': ['hypotheses', incidentId],
        'guidance.posted': ['guidance', incidentId],
        'runbook.created': ['runbooks', incidentId],
        'runbook.updated': ['runbooks', incidentId],
        'action.created': ['actions', incidentId],
        'action.executed': ['actions', incidentId],
        'lease.claimed': ['leases', incidentId],
        'lease.heartbeat': ['leases', incidentId],
        'lease.released': ['leases', incidentId],
        'operation.registered': ['operations', incidentId],
      };
      const key = keyMap[ev.type];
      if (key) queryClient.invalidateQueries({ queryKey: key });
      queryClient.invalidateQueries({ queryKey: ['stats', incidentId] });
      queryClient.invalidateQueries({ queryKey: ['events', incidentId] });
    },
    [incidentId, queryClient],
  );
  useEffect(() => api.subscribeEvents(incidentId, onEvent), [incidentId, onEvent]);

  const { data: incident } = useQuery({ queryKey: ['incident', incidentId], queryFn: () => api.getIncident(incidentId), ...REFRESH });
  const { data: stats } = useQuery({ queryKey: ['stats', incidentId], queryFn: () => api.getStats(incidentId), ...REFRESH });
  const { data: events } = useQuery({ queryKey: ['events', incidentId], queryFn: () => api.listEvents(incidentId), ...REFRESH });
  // 经验库：相似事故/候选 runbook 命中（P1 提供检索，P0 占位）
  const { data: knowledge } = useQuery({
    queryKey: ['knowledge', incidentId],
    queryFn: () => api.listKnowledge(incidentId),
    ...REFRESH,
  });

  const [guideText, setGuideText] = useState('');
  const [guideType, setGuideType] = useState('guidance');
  const [lastDiag, setLastDiag] = useState('');

  const diagnose = useMutation({
    mutationFn: () => api.runDiagnose(incidentId),
    onSuccess: () => {
      setLastDiag('AI 诊断完成：headless-diagnoser 已提交假设');
      queryClient.invalidateQueries({ queryKey: ['hypotheses', incidentId] });
      queryClient.invalidateQueries({ queryKey: ['events', incidentId] });
    },
    onError: (e: Error) => setLastDiag(`诊断失败：${e.message.slice(0, 60)}`),
  });
  const postGuidance = useMutation({
    mutationFn: (text: string) =>
      api.createGuidance(incidentId, {
        from_ic: incident?.ic_name ?? 'ic',
        text: guideType !== 'guidance' ? `[${guideType}] ${text}` : text,
        priority: 'directive',
      }),
    onSuccess: () => {
      setGuideText('');
      queryClient.invalidateQueries({ queryKey: ['guidance', incidentId] });
      queryClient.invalidateQueries({ queryKey: ['events', incidentId] });
    },
  });
  const onArbitrate = (h: Hypothesis) => {
    setGuideType('仲裁裁决');
    setGuideText(`仲裁假设「${h.topic.slice(0, 40)}」：请说明裁决理由`);
  };

  const flowCount = events?.length ?? 0;
  const dedupCount = stats?.duplicates_avoided ?? 0;

  return (
    <div className="flex h-full min-h-0 flex-col">
      {/* 顶部：影响 / 止血 / 仲裁 */}
      <div className="incident-bar">
        <span className="inc-badge p1">{incident?.severity ?? 'P?'}</span>
        <span className="inc-title">{incident?.title ?? '加载中…'}</span>
        <div className="kv"><span className="k">影响</span><span className="v warn">error budget 剩余中</span></div>
        <div className="kv"><span className="k">止血</span><span className="v warn">无 · 需 IC 决策</span></div>
        <div className="kv"><span className="k">决策延迟</span><span className="v mono">{stats?.decision_latency_min ?? 0}m</span></div>
        <div className="kv"><span className="k">去重</span><span className="v mono">{fmtPct(stats?.dedup_rate ?? 0)}</span></div>
        <div className="kv" title="经验库：相似事故/候选 runbook（P1 开放检索）">
          <span className="k">经验命中</span>
          <span className="v mono">{(knowledge ?? []).length}</span>
        </div>
        <div className="ctx-meta">
          <span>ctx@{events?.length ?? 0}</span>
          <span>{incidentId.slice(0, 14)}</span>
        </div>
      </div>

      {/* 三栏：工作图 / 事实账本 / 假设矩阵+IC 决策 */}
      <div className="columns">
        <WorkGraphCol incidentId={incidentId} />
        <FactLedger incidentId={incidentId} />
        <div className="col col-right">
          <div className="col-label">假设对比矩阵</div>
          <HypothesisMatrix incidentId={incidentId} onArbitrate={onArbitrate} />

          <div className="ic-queue">
            <div className="col-label">IC 决策队列</div>
            <div className="ic-type-row">
              {['guidance', '事实确认', '仲裁裁决', '冻结话题', '止血决策'].map((t) => (
                <span key={t} className={`ic-type ${guideType === t ? 'sel' : ''}`} onClick={() => setGuideType(t)}>{t === 'guidance' ? 'Guidance' : t}</span>
              ))}
            </div>
            <textarea
              className="ic-input"
              placeholder="指示 / 裁决 / 止血决策内容… 只有结构化决策改变事故状态"
              value={guideText}
              onChange={(e) => setGuideText(e.target.value)}
            />
            <button
              className="ic-send"
              disabled={!guideText.trim() || postGuidance.isPending}
              onClick={() => guideText.trim() && postGuidance.mutate(guideText.trim())}
            >
              发布 IC 决策
            </button>
            {lastDiag && <div className="action-hint">{lastDiag}</div>}
            <button
              className="diag-btn"
              disabled={diagnose.isPending}
              onClick={() => diagnose.mutate()}
            >
              {diagnose.isPending ? 'AI 诊断中（GLM-5.2 分析真实日志…）' : '触发 AI 诊断（headless-diagnoser 进场）'}
            </button>
            <div className="permission-strip">
              <span className="perm ro">资源只读</span>
              <span className="perm rw">可提交 Guidance</span>
              <span className="perm no">无生产执行权限</span>
            </div>
          </div>
        </div>
      </div>

      {/* 底部抽屉：活动流（审计信息，降级） */}
      <div className="drawer">
        <span className="t">活动流 · 收起</span>
        <span className="stat">协作纪要 <b>{flowCount}</b> 条</span>
        <span className="stat">去重查询 <b className="sky">{dedupCount}</b></span>
        <span className="stat">事实 <b className="ok">{stats?.facts_confirmed ?? 0}</b> / 未解 <b className="warn">{stats?.open_questions ?? 0}</b></span>
        <span className="toggle">展开 ↑</span>
      </div>
    </div>
  );
}
