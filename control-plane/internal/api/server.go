// Package api REST/HTTP 层：实现 P0-brief §5 的 API 契约。
//
// 前端（kimi-k3）依赖此契约；REST/JSON + SSE，gRPC 仅内部（P0 未启用）。
// 所有写路径均只写"账本/工作图"（只读协同），不产生任何修复/执行副作用。
package api

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/samson-samson/hivemind/control-plane/internal/eventbus"
	"github.com/samson-samson/hivemind/control-plane/internal/evidence"
	"github.com/samson-samson/hivemind/control-plane/internal/iam"
	"github.com/samson-samson/hivemind/control-plane/internal/lease"
	"github.com/samson-samson/hivemind/control-plane/internal/querycoord"
	"github.com/samson-samson/hivemind/control-plane/internal/stats"
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
	initAuth() // 加载 API key 映射（安全基线）
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
	// context@vN 别名回退：无方法限定，避免 405 短路具体 POST 路由（如 runbooks）。
	mux.HandleFunc("/api/v1/incidents/{id}/{rest...}", s.handleContextPathAlias)

	// Work graph
	mux.HandleFunc("POST /api/v1/incidents/{id}/work-nodes", s.handleCreateWorkNode)
	mux.HandleFunc("GET /api/v1/incidents/{id}/work-nodes", s.handleListWorkNodes)
	mux.HandleFunc("PATCH /api/v1/incidents/{id}/work-nodes/{nid}", s.handleUpdateWorkNode)

	// Advisory lease
	mux.HandleFunc("POST /api/v1/incidents/{id}/leases", s.handleCreateLease)
	mux.HandleFunc("GET /api/v1/incidents/{id}/leases", s.handleListLeases)
	mux.HandleFunc("POST /api/v1/incidents/{id}/leases/{lid}/heartbeat", s.handleLeaseHeartbeat)
	mux.HandleFunc("DELETE /api/v1/incidents/{id}/leases/{lid}", s.handleReleaseLease)

	// Operations (single-flight)
	mux.HandleFunc("POST /api/v1/incidents/{id}/operations", s.handleRegisterOperation)
	mux.HandleFunc("GET /api/v1/incidents/{id}/operations", s.handleListOperations)

	// Evidence
	mux.HandleFunc("POST /api/v1/incidents/{id}/evidence", s.handlePushEvidence)
	mux.HandleFunc("GET /api/v1/incidents/{id}/evidence", s.handleListEvidence)

	// Facts / Hypotheses（补充入口，喂 stats）
	mux.HandleFunc("POST /api/v1/incidents/{id}/facts", s.handlePostFact)
	mux.HandleFunc("GET /api/v1/incidents/{id}/facts", s.handleListFacts)
	mux.HandleFunc("POST /api/v1/incidents/{id}/hypotheses", s.handlePostHypothesis)
	mux.HandleFunc("GET /api/v1/incidents/{id}/hypotheses", s.handleListHypotheses)

	// Guidance（IC 发言）
	mux.HandleFunc("POST /api/v1/incidents/{id}/guidance", s.handlePostGuidance)
	mux.HandleFunc("GET /api/v1/incidents/{id}/guidance", s.handleListGuidance)

	// Stats / Events
	mux.HandleFunc("GET /api/v1/incidents/{id}/stats", s.handleGetStats)
	mux.HandleFunc("GET /api/v1/incidents/{id}/events", s.handleSSE)

	// 知识蒸馏（runbook 记忆 + 经验召回）
	mux.HandleFunc("POST /api/v1/incidents/{id}/runbooks", s.handleCreateRunbook)
	mux.HandleFunc("GET /api/v1/incidents/{id}/runbooks", s.handleListRunbooks)
	mux.HandleFunc("PATCH /api/v1/runbooks/{rid}", s.handleUpdateRunbook)
	mux.HandleFunc("GET /api/v1/incidents/{id}/knowledge", s.handleListKnowledge)

	// AI 诊断（触发 headless-diagnoser 进场）
	mux.HandleFunc("POST /api/v1/incidents/{id}/diagnose", s.handleDiagnose)

	// 告警接入（自动开独立会议室，同指纹合并）
	mux.HandleFunc("POST /api/v1/ingest/alert", s.handleIngestAlert)

	// Health
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	return corsMiddleware(AuthMiddleware(logMiddleware(mux)))
}

// corsMiddleware 开发期跨域放行。生产由网关/同域部署处理，白名单可用
// OPSHIVE_CORS_ORIGINS 逗号分隔覆盖（默认 localhost:5173 即 Vite dev）。
func corsMiddleware(next http.Handler) http.Handler {
	allowed := map[string]bool{"http://localhost:5173": true}
	for _, o := range strings.Split(os.Getenv("OPSHIVE_CORS_ORIGINS"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			allowed[o] = true
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
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
