package iam

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

// IncidentFingerprint 计算事故指纹：对症状签名（告警名 + 受影响实体 +
// 指标变化模式 + 日志关键词等）做规范化后哈希，用于**跨事故相似候选召回**。
// 注意：指纹命中只用于候选召回，不直接决定根因（铁律 2）。
func IncidentFingerprint(symptomSet []string) string {
	norm := normalizeStrings(symptomSet)
	canonical, _ := json.Marshal(norm) // 规范化后的有序数组，marshal 不会失败
	sum := sha256.Sum256(canonical)
	return "v1:" + hex.EncodeToString(sum[:])
}

// normalizeStrings 返回去除空白、去重、排序后的字符串集合。
func normalizeStrings(in []string) []string {
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
