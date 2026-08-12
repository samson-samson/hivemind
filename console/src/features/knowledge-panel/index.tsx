import { useQuery } from '@tanstack/react-query';
import { useParams } from 'react-router-dom';
import { api } from '../../lib/api';

/**
 * 知识面板（P0 stub）：
 * - 历史命中：P1 才提供检索，P0 接口已留（GET /incidents/{id}/knowledge），返回空集占位。
 * - 候选占位：事故中只产候选，事故后才进入认证流水线（铁律 2）。
 */
export function KnowledgePanel() {
  const { incidentId = '' } = useParams();
  const { data: hits } = useQuery({
    queryKey: ['knowledge', incidentId],
    queryFn: () => api.listKnowledge(incidentId),
  });
  const { data: stats } = useQuery({
    queryKey: ['stats', incidentId],
    queryFn: () => api.getStats(incidentId),
  });

  return (
    <div className="grid max-w-5xl grid-cols-1 gap-4 p-5 lg:grid-cols-2">
      <section className="rounded-lg border border-zinc-800 bg-zinc-900/50 p-4">
        <h2 className="text-xs font-semibold uppercase tracking-wider text-zinc-500">
          历史命中（runbook / 复盘）
        </h2>
        {(hits ?? []).length === 0 ? (
          <div className="mt-3 rounded-lg border border-dashed border-zinc-700 p-6 text-center">
            <div className="text-sm text-zinc-400">暂无历史命中</div>
            <div className="mt-1 text-[10px] text-zinc-600">
              检索能力在 P1 提供；接口已预留（GET /incidents/{'{id}'}/knowledge），P0 返回空集。
            </div>
          </div>
        ) : (
          <ul className="mt-3 space-y-2">
            {(hits ?? []).map((h) => (
              <li key={h.id} className="rounded border border-zinc-800 bg-zinc-950 p-3 text-xs">
                {h.title}
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="rounded-lg border border-zinc-800 bg-zinc-900/50 p-4">
        <h2 className="text-xs font-semibold uppercase tracking-wider text-zinc-500">
          本次事故产出候选（事故后进入认证流水线）
        </h2>
        <ul className="mt-3 space-y-2">
          <CandidateCard
            title="候选：DB 连接池耗尽 × 发布变更 排查路径"
            body="若 hyp-1 被确认，本场排查路径（拐点定位 → 发布关联 → 池饱和交叉验证）将成为 runbook 候选。"
          />
          <CandidateCard
            title="候选：重复查询模式「DB 池序列多 agent 重复采集」"
            body="op-5 ≡ op-4 被 single-flight 去重，该模式将进入去重规则候选集。"
          />
        </ul>
        <div className="mt-3 rounded border border-zinc-800 bg-zinc-950 p-2.5 text-[10px] leading-relaxed text-zinc-500">
          认证链：candidate → reviewed → validated → certified → revoked。
          LLM 只生成候选，编译 / 策略 / 测试决定发布；失效由依赖 / 失败率 / 反例驱动，非 TTL。
        </div>
        {stats && (
          <div className="mt-3 flex gap-4 text-[10px] text-zinc-500">
            <span>去重次数：<span className="font-mono text-violet-300">{stats.duplicates_avoided}</span></span>
            <span>已确认事实：<span className="font-mono text-emerald-300">{stats.facts_confirmed}</span></span>
          </div>
        )}
      </section>
    </div>
  );
}

function CandidateCard({ title, body }: { title: string; body: string }) {
  return (
    <li className="rounded-lg border border-dashed border-amber-500/40 bg-amber-500/5 p-3">
      <div className="flex items-center gap-2">
        <span className="rounded bg-amber-500/20 px-1.5 py-0.5 text-[10px] font-semibold text-amber-300">
          候选 · 未认证
        </span>
      </div>
      <div className="mt-1.5 text-xs font-medium text-zinc-200">{title}</div>
      <p className="mt-1 text-[11px] leading-relaxed text-zinc-400">{body}</p>
    </li>
  );
}
