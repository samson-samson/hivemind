package api

import (
	"encoding/json"
	"time"
	"fmt"
	"net/http"
)

// handleSSE 流式输出事故的实时事件（text/event-stream）。
// 事件格式：event: <type>\ndata: <json>\n\n
func (s *Service) handleSSE(w http.ResponseWriter, r *http.Request) {
	incidentID := r.PathValue("id")
	if _, err := s.store.GetIncident(r.Context(), incidentID); err != nil {
		mapError(w, err)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	sub := s.bus.Subscribe(incidentID)
	defer s.bus.Unsubscribe(incidentID, sub)

	// 连接建立提示。
	fmt.Fprintf(w, "event: connected\ndata: {\"incident_id\":%q}\n\n", incidentID)
	flusher.Flush()

	// 事件 ID（SSE 标准语义）：重连时前端带 Last-Event-ID 可续传。
	// P0 无事件存储，重连后由前端重新拉快照 + 增量；ID 仅用于去重与乱序检测。
	var seq int64
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n") // 注释行：防代理断连
			flusher.Flush()
		case ev := <-sub.C:
			b, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			seq++
			fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", seq, ev.Type, b)
			flusher.Flush()
		}
	}
}
