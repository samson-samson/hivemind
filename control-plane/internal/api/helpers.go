package api

import (
	"context"
	"time"

	"github.com/ops-hive/control-plane/internal/iam"
)

// timeNow 返回统一 UTC 时间。
func timeNow() time.Time { return time.Now().UTC() }

// defaultSource 缺省来源标记。
func defaultSource(s string) string {
	if s == "" {
		return "api"
	}
	return s
}

// addEdge 便捷写入一条边（忽略重复/无关错误）。
func (s *Service) addEdge(ctx context.Context, incidentID string, typ iam.EdgeType, from, to string) error {
	return s.store.AddEdge(ctx, &iam.Edge{
		ID:         iam.NewID("edge"),
		IncidentID: incidentID,
		Type:       typ,
		From:       from,
		To:         to,
		Timestamp:  timeNow(),
	})
}
