// Package evidence 证据总线：append-only 证据账本 + 血缘 DAG + 独立性评分。
//
// 账本规则（P0 审计底线）：
//   - 只追加（append-only），不支持修改/删除；
//   - 每条记录哈希链式校验（prev_hash 串联），篡改可被 Verify 发现；
//   - 每条记录带单调递增序号 seq，作为 proof_trace 的可回放指针。
package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/samson-samson/hivemind/control-plane/internal/iam"
)

// LedgerEntry 账本中的一条记录。
type LedgerEntry struct {
	Seq        int64         `json:"seq"`         // 单调递增序号（proof_trace 指针）
	IncidentID string        `json:"incident_id"` // 归属事故
	Evidence   *iam.Evidence `json:"evidence"`    // 证据内容（自包含，便于回放）
	PrevHash   string        `json:"prev_hash"`   // 前一条哈希（链式校验）
	Hash       string        `json:"hash"`        // 本条哈希
	CapturedAt time.Time     `json:"captured_at"` // 入账时间
}

// Ledger 纯内存的 append-only 账本。
type Ledger struct {
	mu       sync.Mutex
	entries  []*LedgerEntry
	seq      int64
	lastHash string // 链尾哈希
}

// NewLedger 构造空账本。
func NewLedger() *Ledger {
	return &Ledger{}
}

// Append 向账本追加一条证据，返回带序号与哈希的账本记录。
// 一旦成功即不可回退（不提供删除/修改）。
func (l *Ledger) Append(_ context.Context, incidentID string, ev *iam.Evidence) (*LedgerEntry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.seq++
	// 入账前克隆：账本保存不可变快照，防止外部随后 mutate 破坏哈希链。
	snapshot := ev.Clone()
	entry := &LedgerEntry{
		Seq:        l.seq,
		IncidentID: incidentID,
		Evidence:   snapshot,
		PrevHash:   l.lastHash,
		CapturedAt: time.Now().UTC(),
	}
	h, err := entry.computeHash()
	if err != nil {
		return nil, err
	}
	entry.Hash = h
	l.entries = append(l.entries, entry)
	l.lastHash = h
	return entry, nil
}

// Get 按序号取回账本记录，供 proof_trace 回放。
func (l *Ledger) Get(_ context.Context, seq int64) (*LedgerEntry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.entries {
		if e.Seq == seq {
			return e, nil
		}
	}
	return nil, fmt.Errorf("ledger entry %d not found", seq)
}

// List 返回某事故的全部账本记录（按 seq 升序）。
func (l *Ledger) List(_ context.Context, incidentID string) ([]*LedgerEntry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]*LedgerEntry, 0)
	for _, e := range l.entries {
		if e.IncidentID == incidentID {
			out = append(out, e)
		}
	}
	return out, nil
}

// Verify 校验整条哈希链的完整性（探测篡改）。
func (l *Ledger) Verify(_ context.Context) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	prev := ""
	for _, e := range l.entries {
		if e.PrevHash != prev {
			return false, nil
		}
		h, err := e.computeHash()
		if err != nil {
			return false, err
		}
		if e.Hash != h {
			return false, nil
		}
		prev = e.Hash
	}
	return true, nil
}

// computeHash 计算本条哈希：sha256(seq | incident_id | evidence_json | prev_hash)。
// evidence_json 以稳定方式编码，字段顺序变化不影响校验（marshal 输出稳定）。
func (e *LedgerEntry) computeHash() (string, error) {
	b, err := json.Marshal(e.Evidence)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	fmt.Fprintf(h, "%d|%s|%x|%s", e.Seq, e.IncidentID, b, e.PrevHash)
	return hex.EncodeToString(h.Sum(nil)), nil
}
