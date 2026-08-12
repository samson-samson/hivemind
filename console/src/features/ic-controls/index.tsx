import { useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate, useParams } from 'react-router-dom';
import {
  api,
  type AgentRole,
  type CostLevel,
  type GuidancePriority,
  type Severity,
} from '../../lib/api';

/**
 * IC 操作：建事故 / 调工作图（新增工作单元）/ 发 Guidance。
 * P0 仅此三项写操作；无任何审批执行 UI。
 */
export function IcControls() {
  return (
    <div className="grid max-w-6xl grid-cols-1 gap-4 p-5 lg:grid-cols-3">
      <CreateIncidentCard />
      <AddWorkNodeCard />
      <GuidanceCard />
    </div>
  );
}

const inputCls =
  'w-full rounded border border-zinc-700 bg-zinc-950 px-2.5 py-1.5 text-xs text-zinc-200 placeholder-zinc-600 focus:border-sky-500 focus:outline-none';
const labelCls = 'mb-1 block text-[10px] font-medium uppercase tracking-wider text-zinc-500';
const btnCls =
  'rounded bg-sky-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-sky-500 disabled:opacity-40';

function CreateIncidentCard() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [title, setTitle] = useState('');
  const [severity, setSeverity] = useState<Severity>('P2');
  const [icName, setIcName] = useState('张倩');
  const [symptoms, setSymptoms] = useState('');

  const mutation = useMutation({
    mutationFn: () =>
      api.createIncident({
        title,
        severity,
        ic_name: icName,
        symptom_set: symptoms.split('\n').map((s) => s.trim()).filter(Boolean),
      }),
    onSuccess: (inc) => {
      queryClient.invalidateQueries({ queryKey: ['incidents'] });
      navigate(`/incidents/${inc.id}/overview`);
    },
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (title.trim()) mutation.mutate();
  };

  return (
    <form onSubmit={submit} className="rounded-lg border border-zinc-800 bg-zinc-900/50 p-4">
      <h2 className="text-xs font-semibold uppercase tracking-wider text-zinc-500">建事故</h2>
      <div className="mt-3 space-y-3">
        <div>
          <label className={labelCls}>标题</label>
          <input className={inputCls} value={title} onChange={(e) => setTitle(e.target.value)} placeholder="如：order-service 延迟飙升" />
        </div>
        <div className="flex gap-3">
          <div className="flex-1">
            <label className={labelCls}>严重度</label>
            <select className={inputCls} value={severity} onChange={(e) => setSeverity(e.target.value as Severity)}>
              <option value="P1">P1</option>
              <option value="P2">P2</option>
              <option value="P3">P3</option>
            </select>
          </div>
          <div className="flex-1">
            <label className={labelCls}>IC</label>
            <input className={inputCls} value={icName} onChange={(e) => setIcName(e.target.value)} />
          </div>
        </div>
        <div>
          <label className={labelCls}>症状集（每行一条）</label>
          <textarea className={`${inputCls} h-20 resize-none`} value={symptoms} onChange={(e) => setSymptoms(e.target.value)} />
        </div>
        <button className={btnCls} disabled={mutation.isPending || !title.trim()}>
          {mutation.isPending ? '创建中…' : '创建事故'}
        </button>
      </div>
    </form>
  );
}

function AddWorkNodeCard() {
  const { incidentId = '' } = useParams();
  const queryClient = useQueryClient();
  const { data: nodes } = useQuery({
    queryKey: ['work-nodes', incidentId],
    queryFn: () => api.listWorkNodes(incidentId),
  });

  const [question, setQuestion] = useState('');
  const [scope, setScope] = useState('');
  const [cost, setCost] = useState<CostLevel>('low');
  const [role, setRole] = useState<AgentRole>('explorer');
  const [dependsOn, setDependsOn] = useState('');

  const mutation = useMutation({
    mutationFn: () =>
      api.createWorkNode(incidentId, {
        question,
        scope,
        expected_evidence: [],
        cost,
        role,
        depends_on: dependsOn ? [dependsOn] : [],
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['work-nodes', incidentId] });
      setQuestion('');
      setScope('');
    },
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (question.trim() && scope.trim()) mutation.mutate();
  };

  return (
    <form onSubmit={submit} className="rounded-lg border border-zinc-800 bg-zinc-900/50 p-4">
      <h2 className="text-xs font-semibold uppercase tracking-wider text-zinc-500">调工作图（新增工作单元）</h2>
      <div className="mt-3 space-y-3">
        <div>
          <label className={labelCls}>调查问题</label>
          <input className={inputCls} value={question} onChange={(e) => setQuestion(e.target.value)} placeholder="如：回滚 v2.4.1 的影响面评估" />
        </div>
        <div>
          <label className={labelCls}>范围（数据源 + 目标）</label>
          <input className={`${inputCls} font-mono`} value={scope} onChange={(e) => setScope(e.target.value)} placeholder="argocd: payment-gateway 回滚预案" />
        </div>
        <div className="flex gap-3">
          <div className="flex-1">
            <label className={labelCls}>成本</label>
            <select className={inputCls} value={cost} onChange={(e) => setCost(e.target.value as CostLevel)}>
              <option value="low">低</option>
              <option value="medium">中</option>
              <option value="high">高</option>
            </select>
          </div>
          <div className="flex-1">
            <label className={labelCls}>角色</label>
            <select className={inputCls} value={role} onChange={(e) => setRole(e.target.value as AgentRole)}>
              <option value="explorer">探索</option>
              <option value="verifier">验证</option>
              <option value="skeptic">怀疑</option>
              <option value="executor">执行</option>
            </select>
          </div>
        </div>
        <div>
          <label className={labelCls}>依赖（可选）</label>
          <select className={inputCls} value={dependsOn} onChange={(e) => setDependsOn(e.target.value)}>
            <option value="">无</option>
            {(nodes ?? []).map((n) => (
              <option key={n.id} value={n.id}>
                {n.id} · {n.question.slice(0, 24)}…
              </option>
            ))}
          </select>
        </div>
        <button className={btnCls} disabled={mutation.isPending || !question.trim() || !scope.trim()}>
          {mutation.isPending ? '提交中…' : '加入工作图'}
        </button>
      </div>
    </form>
  );
}

function GuidanceCard() {
  const { incidentId = '' } = useParams();
  const queryClient = useQueryClient();
  const [text, setText] = useState('');
  const [priority, setPriority] = useState<GuidancePriority>('directive');

  const mutation = useMutation({
    mutationFn: () => api.createGuidance(incidentId, { from_ic: '张倩', text, priority }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['guidance', incidentId] });
      queryClient.invalidateQueries({ queryKey: ['events', incidentId] });
      setText('');
    },
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (text.trim()) mutation.mutate();
  };

  return (
    <form onSubmit={submit} className="rounded-lg border border-zinc-800 bg-zinc-900/50 p-4">
      <h2 className="text-xs font-semibold uppercase tracking-wider text-zinc-500">发 Guidance</h2>
      <div className="mt-3 space-y-3">
        <div>
          <label className={labelCls}>优先级</label>
          <select className={inputCls} value={priority} onChange={(e) => setPriority(e.target.value as GuidancePriority)}>
            <option value="info">知会</option>
            <option value="directive">指令</option>
            <option value="urgent">紧急</option>
          </select>
        </div>
        <div>
          <label className={labelCls}>内容</label>
          <textarea
            className={`${inputCls} h-28 resize-none`}
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder="对全部 agent 可见的协调指令。只有结构化证据与决策改变事故状态。"
          />
        </div>
        <button className={btnCls} disabled={mutation.isPending || !text.trim()}>
          {mutation.isPending ? '发送中…' : '发布 Guidance'}
        </button>
        {mutation.isSuccess && <div className="text-[10px] text-emerald-400">已发布，事件流已推送。</div>}
      </div>
    </form>
  );
}
