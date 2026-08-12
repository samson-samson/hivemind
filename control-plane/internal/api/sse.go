package api

import (
	"encoding/json"
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

	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-sub.C:
			b, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, b)
			flusher.Flush()
		}
	}
}
