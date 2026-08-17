package iam

// Clone 深拷贝 Evidence，返回独立快照。
// 证据账本（Ledger）入账时必须先克隆，避免入账后被外部 mutate 导致哈希链失配
// （append-only 账本要求内容不可变）。
func (e *Evidence) Clone() *Evidence {
	if e == nil {
		return nil
	}
	out := *e // 拷贝标量字段与底层指针（slice/map 需另行拷贝）
	out.LineageDAG = append([]string(nil), e.LineageDAG...)
	out.ProofTrace = append([]ProofTraceEntry(nil), e.ProofTrace...)
	return &out
}

// Clone 深拷贝 WorkNode（防锁外修改竞态）。
func (w *WorkNode) Clone() *WorkNode {
	if w == nil {
		return nil
	}
	out := *w
	out.ExpectedEvidence = append([]string(nil), w.ExpectedEvidence...)
	out.DependsOn = append([]string(nil), w.DependsOn...)
	out.ProofTrace = append([]ProofTraceEntry(nil), w.ProofTrace...)
	return &out
}

// Clone 深拷贝 Operation。
func (o *Operation) Clone() *Operation {
	if o == nil {
		return nil
	}
	out := *o
	out.ProofTrace = append([]ProofTraceEntry(nil), o.ProofTrace...)
	return &out
}

// Clone 深拷贝 Runbook。
func (r *Runbook) Clone() *Runbook {
	if r == nil {
		return nil
	}
	out := *r
	out.Symptoms = append([]string(nil), r.Symptoms...)
	out.DiagnosticSteps = append([]string(nil), r.DiagnosticSteps...)
	out.VerificationActions = append([]string(nil), r.VerificationActions...)
	out.ProofTrace = append([]ProofTraceEntry(nil), r.ProofTrace...)
	return &out
}
