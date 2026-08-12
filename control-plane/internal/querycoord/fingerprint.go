// Package querycoord 查询协调器：操作指纹生成 + single-flight 去重。
//
// 目标（铁律 1）：让"完全相同查询"被 single-flight 压掉、兼容查询复用已有结果、
// 仅在需要新鲜度时允许重查——协同目标是"无意重复最小化 + 受控冗余交叉验证"，
// 不是"重复归零"。
package querycoord

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/ops-hive/control-plane/internal/iam"
)

// FingerprintVersion 指纹算法版本。算法演进时递增，避免跨版本误合并。
const FingerprintVersion = "v1"

// Fingerprint 计算操作指纹：对 QuerySpec 做规范化后哈希。
// 规范化保证"完全相同查询"（字段顺序、空白差异）产生相同指纹。
func Fingerprint(spec iam.QuerySpec) (string, error) {
	canonical, err := canonicalJSON(spec)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return FingerprintVersion + ":" + hex.EncodeToString(sum[:]), nil
}

// canonicalJSON 将 QuerySpec 规范化为稳定 JSON 字节。
// 规范化规则：
//   - 过滤条件：去空白、排序（消除顺序差异）
//   - 查询 AST / 数据快照：折叠连续空白
//   - 时间窗：截断到秒，统一 RFC3339 格式
//   - 枚举字段：小写 + 去空白
func canonicalJSON(spec iam.QuerySpec) ([]byte, error) {
	norm := iam.QuerySpec{
		Target:       strings.TrimSpace(spec.Target),
		DataSource:   strings.ToLower(strings.TrimSpace(spec.DataSource)),
		TimeWindow:   canonicalTimeRange(spec.TimeWindow),
		QueryAST:     collapseSpace(spec.QueryAST),
		Filters:      canonicalStrings(spec.Filters),
		Tenant:       strings.ToLower(strings.TrimSpace(spec.Tenant)),
		ToolVersion:  strings.TrimSpace(spec.ToolVersion),
		DataSnapshot: collapseSpace(spec.DataSnapshot),
	}
	return json.Marshal(norm)
}

// canonicalTimeRange 统一时间窗表示（秒级精度，RFC3339）。
func canonicalTimeRange(tr iam.TimeRange) iam.TimeRange {
	out := iam.TimeRange{Start: tr.Start.Truncate(time.Second)}
	if tr.End != nil {
		e := tr.End.Truncate(time.Second)
		out.End = &e
	}
	return out
}

// canonicalStrings 去空白、去重、排序。
func canonicalStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		t := strings.TrimSpace(s)
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// collapseSpace 将连续空白折叠为单个空格，并去除首尾空白。
func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
