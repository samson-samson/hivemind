// Command server 是控制平面入口：HTTP(REST) + SSE。
//
// P0 定位：只读协同账本——工作图 + 查询去重 + 证据血缘 + 人工 IC + 指挥室 v1。
// 存储默认纯内存（任何机器可跑）；接入 PostgreSQL 时替换 iam.Store 实现即可。
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/samson-samson/hivemind/control-plane/internal/api"
	"github.com/samson-samson/hivemind/control-plane/internal/iam"
)

func main() {
	addr := envOr("OPSHIVE_ADDR", ":8080")
	freshWindow := envDurationOr("OPSHIVE_FRESH_WINDOW", 5*time.Minute)
	leaseTTL := envDurationOr("OPSHIVE_LEASE_TTL", time.Minute)

	// P0 使用纯内存存储；未来切换到 PostgreSQL：
	//   store, err := pgstore.New(ctx, os.Getenv("DATABASE_URL"))
	store := iam.NewMemoryStore()

	svc := api.NewService(store, freshWindow, leaseTTL)
	srv := &http.Server{
		Addr:         addr,
		Handler:      svc.Handler(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // SSE 长连接，不设写超时
	}

	go func() {
		log.Printf("Hivemind control-plane listening on %s (fresh_window=%s lease_ttl=%s)", addr, freshWindow, leaseTTL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	// 优雅退出。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDurationOr(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
