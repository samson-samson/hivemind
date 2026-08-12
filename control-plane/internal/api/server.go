// Package api REST/HTTP 层：实现 P0-brief §5 的 API 契约。
//
// 前端（kimi-k3）依赖此契约；REST/JSON + SSE，gRPC 仅内部（P0 未启用）。
// 所有写路径均只写"账本/工作图"（只读协同），不产生任何修复/执行副作用。
package api

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/ops-hive/control-plane/internal/eventbus"
	"github.com/ops-hive/control-plane/internal/evidence"
	"github.com/ops-hive/control-plane/internal/iam"
	"github.com/ops-hive/control-plane/internal/lease"
	"github.com/ops-hive/control-plane/internal/querycoord"
	"github.com/ops-hive/control-plane/internal/stats"
)

// Service 控制平面服务：编排存储 / 查询协调 / 证据账本 / 租约 / 事件总线。
type Service struct {
	store  iam.Store
	coord  *querycoord.Coordinator
	ledger *evidence.Ledger
	leases *lease.Manager
	bus    *eventbus.Bus
	stats  *stats.Collector

	// context@vN 版本计数（每次变更 +1）
	verMu    sync.Mutex
	versions map[string]int

	freshWindow time.Duration
	leaseTTL    time.Duration
}

// NewService 组装各组件并返回服务。freshWindow 为查询结果新鲜度窗口，
// leaseTTL 为租约心跳超时；传 0 使用默认值。
func NewService(store iam.Store, freshWindow, leaseTTL time.Duration) *Service {
	if freshWindow == 0 {
		freshWindow = 5 * time.Minute
	}
	if leaseTTL == 0 {
		leaseTTL = time.Minute
	}
	ledger := evidence.NewLedger()
	return &Service{
		store:       store,
		coord:       querycoord.NewCoordinator(store, freshWindow),
		ledger:      ledger,
		leases:      lease.NewManager(leaseTTL),
		bus:         eventbus.NewBus(),
		stats:       stats.NewCollector(store, ledger),
		versions:    make(map[string]int),
		freshWindow: freshWindow,
		leaseTTL:    leaseTTL,
	}
}

// Store 暴露底层存储（测试/扩展用）。
func (s *Service) Store() iam.Store { return s.store }

// Bus 暴露事件总线（测试/扩展用）。
func (s *Service) Bus() *eventbus.Bus { return s.bus }

// bumpVersion 递增事故的内容版本号，返回新版本。
func (s *Service) bumpVersion(incidentID string) int {
	s.verMu.Lock()
	defer s.verMu.Unlock()
	s.versions[incidentID]++
	return s.versions[incidentID]
}

// contextVersion 返回当前版本号（0 表示尚无变更）。
func (s *Service) contextVersion(incidentID string) int {
	s.verMu.Lock()
	defer s.verMu.Unlock()
	return s.versions[incidentID]
}

// Handler 构建 HTTP 路由（Go 1.22+ 方法+路径模式）。
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()

	// Incident
	mux.HandleFunc("POST /api/v1/incidents", s.handleCreateIncident)
	mux.HandleFunc("GET /api/v1/incidents", s.handleListIncidents)
	mux.HandleFunc("GET /api/v1/incidents/{id}", s.handleGetIncident)
	mux.HandleFunc("GET /api/v1/incidents/{id}/context", s.handleGetContext)
	// context@vN 路径形式（Go mux 不支持段内通配，用回退路由处理）。
	mux.HandleFunc("GET /api/v1/incidents/{id}/{rest...}", s.handleContextPathAlias)

	// Work graph
	mux.HandleFunc("POST /api/v1/incidents/{id}/work-nodes", s.handleCreateWorkNode)
	mux.HandleFunc("GET /api/v1/incidents/{id}/work-nodes", s.handleListWorkNodes)

	// Advisory lease
	mux.HandleFunc("POST /api/v1/incidents/{id}/leases", s.handleCreateLease)
	mux.HandleFunc("POST /api/v1/incidents/{id}/leases/{lid}/heartbeat", s.handleLeaseHeartbeat)
	mux.HandleFunc("DELETE /api/v1/incidents/{id}/leases/{lid}", s.handleReleaseLease)

	// Operations (single-flight)
	mux.HandleFunc("POST /api/v1/incidents/{id}/operations", s.handleRegisterOperation)

	// Evidence
	mux.HandleFunc("POST /api/v1/incidents/{id}/evidence", s.handlePushEvidence)
	mux.HandleFunc("GET /api/v1/incidents/{id}/evidence", s.handleListEvidence)

	// Facts / Hypotheses（补充入口，喂 stats）
	mux.HandleFunc("POST /api/v1/incidents/{id}/facts", s.handlePostFact)
	mux.HandleFunc("POST /api/v1/incidents/{id}/hypotheses", s.handlePostHypothesis)

	// Guidance（IC 发言）
	mux.HandleFunc("POST /api/v1/incidents/{id}/guidance", s.handlePostGuidance)

	// Stats / Events
	mux.HandleFunc("GET /api/v1/incidents/{id}/stats", s.handleGetStats)
	mux.HandleFunc("GET /api/v1/incidents/{id}/events", s.handleSSE)

	// Health
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	return logMiddleware(mux)
}

// logMiddleware 简单访问日志。
func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

// ---- 序列化辅助 ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return err
	}
	return nil
}
