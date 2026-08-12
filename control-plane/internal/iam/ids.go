package iam

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// NewID 生成带类型前缀的唯一 ID，如 "inc_3fa9c1..."。
// 使用 crypto/rand 保证全局唯一；若随机源异常则回退到纳秒时间戳。
func NewID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(b)
}
