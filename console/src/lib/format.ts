import type {
  CostLevel,
  DedupStatus,
  GuidancePriority,
  HypothesisStatus,
  IncidentStatus,
  Severity,
  WorkNodeStatus,
} from '../lib/api';

export const STATUS_LABEL: Record<IncidentStatus, string> = {
  detected: '已检测',
  triaging: '分诊中',
  investigating: '调查中',
  candidate: '候选方案',
  mitigated: '已止血',
  resolved: '已解决',
};

export const STATUS_COLOR: Record<IncidentStatus, string> = {
  detected: 'bg-rose-500',
  triaging: 'bg-amber-500',
  investigating: 'bg-sky-500',
  candidate: 'bg-violet-500',
  mitigated: 'bg-emerald-500',
  resolved: 'bg-zinc-500',
};

export const SEVERITY_STYLE: Record<Severity, string> = {
  P1: 'text-rose-400 border-rose-500/40 bg-rose-500/10',
  P2: 'text-amber-400 border-amber-500/40 bg-amber-500/10',
  P3: 'text-zinc-400 border-zinc-500/40 bg-zinc-500/10',
};

export const WN_STATUS: Record<WorkNodeStatus, { label: string; cls: string }> = {
  pending: { label: '待认领', cls: 'border-zinc-600 text-zinc-400' },
  claimed: { label: '已认领', cls: 'border-sky-500 text-sky-400' },
  in_progress: { label: '进行中', cls: 'border-sky-500 text-sky-300' },
  done: { label: '已完成', cls: 'border-emerald-500 text-emerald-400' },
  stale: { label: '已过期', cls: 'border-amber-500 text-amber-400' },
  cancelled: { label: '已取消', cls: 'border-zinc-700 text-zinc-500' },
};

export const COST_LABEL: Record<CostLevel, string> = { low: '低成本', medium: '中成本', high: '高成本' };

export const DEDUP_STYLE: Record<DedupStatus, { label: string; cls: string }> = {
  fresh: { label: '实查', cls: 'bg-sky-500/15 text-sky-300 border-sky-500/40' },
  single_flight: { label: '已去重', cls: 'bg-amber-500/15 text-amber-300 border-amber-500/40' },
  reused: { label: '已复用', cls: 'bg-violet-500/15 text-violet-300 border-violet-500/40' },
};

export const HYP_STATUS: Record<HypothesisStatus, { label: string; cls: string }> = {
  open: { label: '待定', cls: 'text-zinc-300' },
  strengthening: { label: '增强中', cls: 'text-emerald-300' },
  weakening: { label: '减弱中', cls: 'text-amber-300' },
  refuted: { label: '已证伪', cls: 'text-rose-400 line-through' },
  confirmed: { label: '已确认', cls: 'text-emerald-400' },
};

export const GUIDANCE_STYLE: Record<GuidancePriority, string> = {
  info: 'border-zinc-600 text-zinc-300',
  directive: 'border-sky-500 text-sky-300',
  urgent: 'border-rose-500 text-rose-300',
};

export const fmtTime = (iso: string) => iso.slice(11, 16);

export const fmtPct = (v: number) => `${(v * 100).toFixed(0)}%`;
