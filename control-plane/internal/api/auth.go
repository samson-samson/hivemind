package api

// 认证与身份（安全基线，§9.1-9.2）。
//
// 机制：API Key 认证（请求头 X-API-Key）→ 解析出用户身份（人负责制）。
// 关键约束：from_ic / confirmed_by 等身份字段一律从认证主体解析，
// 客户端自报的字段被忽略（禁止伪造 IC）。
//
// 配置（环境变量）：
//   HIVEMIND_API_KEYS="key1:zhangqian,key2:lixia"   用户=key 冒号后
//   缺省时使用内置开发 key "hivemind-dev-key:zhangqian"（并打警告）
//   HIVEMIND_AUTH=off   显式关闭（仅本地开发，生产必须开）

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

type ctxKey int

const userKey ctxKey = 1

const authHeader = "X-API-Key"

// authConfig 内存中的 key→user 映射（启动时从 env 加载）。
type authConfig struct {
	mu    sync.RWMutex
	keys  map[string]string // key → user
	enabled bool
}

var globalAuth = &authConfig{keys: map[string]string{}, enabled: true}

// initAuth 从环境变量加载 key 映射（幂等，可被测试重置）。
func initAuth() {
	globalAuth.mu.Lock()
	defer globalAuth.mu.Unlock()
	globalAuth.keys = map[string]string{}
	if strings.EqualFold(os.Getenv("HIVEMIND_AUTH"), "off") {
		globalAuth.enabled = false
		log.Println("[auth] disabled by HIVEMIND_AUTH=off (dev only)")
		return
	}
	globalAuth.enabled = true
	raw := os.Getenv("HIVEMIND_API_KEYS")
	if strings.TrimSpace(raw) == "" {
		globalAuth.keys["hivemind-dev-key"] = "zhangqian"
		log.Println("[auth] WARNING: using built-in dev key 'hivemind-dev-key' (user=zhangqian); set HIVEMIND_API_KEYS in production")
		return
	}
	for _, pair := range strings.Split(raw, ",") {
		kv := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(kv) == 2 && kv[0] != "" && kv[1] != "" {
			globalAuth.keys[kv[0]] = kv[1]
		}
	}
	log.Printf("[auth] loaded %d API keys", len(globalAuth.keys))
}

// AuthMiddleware 校验 X-API-Key 并把用户注入 context。
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		globalAuth.mu.RLock()
		enabled := globalAuth.enabled
		keys := globalAuth.keys
		globalAuth.mu.RUnlock()
		if !enabled {
			next.ServeHTTP(w, r)
			return
		}
		key := r.Header.Get(authHeader)
		if key == "" {
			key = r.URL.Query().Get("api_key") // 兼容 curl 冒烟
		}
		if user, ok := keys[key]; ok && user != "" {
			ctx := context.WithValue(r.Context(), userKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		writeError(w, http.StatusUnauthorized, "invalid or missing API key (X-API-Key); set HIVEMIND_API_KEYS")
	})
}

// authUser 从 context 取认证用户（未认证返回空串）。
func authUser(r *http.Request) string {
	if u, ok := r.Context().Value(userKey).(string); ok {
		return u
	}
	return ""
}
