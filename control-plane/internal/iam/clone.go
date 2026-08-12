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
