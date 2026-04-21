// Package server wires middleware, routes, and handlers onto an http.Handler.
package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/shastitko1970-netizen/wunest/internal/auth"
	"github.com/shastitko1970-netizen/wunest/internal/config"
	"github.com/shastitko1970-netizen/wunest/internal/db"
	"github.com/shastitko1970-netizen/wunest/internal/wuapi"
)

// Deps is the dependency container for HTTP handlers. Kept flat — easier to
// refactor than growing a constructor.
type Deps struct {
	Config   *config.Config
	Postgres *db.Postgres
	Redis    *db.Redis
	WuApi    *wuapi.Client
	Logger   *slog.Logger
}

type Server struct {
	deps Deps
}

func New(deps Deps) *Server {
	return &Server{deps: deps}
}

// Router builds the application http.Handler.
func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()

	// Public — no auth needed.
	mux.HandleFunc("GET /health", s.handleHealth)

	// Auth-gated: must have a valid wu_session cookie.
	authOptional := auth.Middleware(s.deps.Config, s.deps.Postgres, s.deps.WuApi, false)
	authRequired := auth.Middleware(s.deps.Config, s.deps.Postgres, s.deps.WuApi, true)

	mux.Handle("GET /api/auth/check", authOptional(http.HandlerFunc(s.handleAuthCheck)))
	mux.Handle("GET /api/me", authRequired(http.HandlerFunc(s.handleMe)))

	// TODO: /api/characters/*, /api/chats/*, /api/chats/:id/stream, /api/personas/*, ...
	// Will be registered as feature packages come online.

	return withRequestLogger(s.deps.Logger, mux)
}

// --- handlers ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	type status struct {
		Postgres string `json:"postgres"`
		Redis    string `json:"redis"`
		Status   string `json:"status"`
	}
	out := status{Postgres: "ok", Redis: "ok", Status: "ok"}

	if err := s.deps.Postgres.Ping(r.Context()); err != nil {
		out.Postgres = "down"
		out.Status = "degraded"
	}
	if err := s.deps.Redis.Ping(r.Context()).Err(); err != nil {
		out.Redis = "down"
		out.Status = "degraded"
	}

	w.Header().Set("Content-Type", "application/json")
	if out.Status != "ok" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(out)
}

// handleAuthCheck reports whether the caller is logged in. Used by the SPA at
// page-load to decide between showing the app or redirecting to wusphere.ru/login.
func (s *Server) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	u := auth.FromContext(r.Context())
	type resp struct {
		Authenticated bool   `json:"authenticated"`
		LoginURL      string `json:"login_url,omitempty"`
	}
	if u == nil {
		writeJSON(w, http.StatusOK, resp{Authenticated: false, LoginURL: "https://wusphere.ru/login?return_to=" + s.deps.Config.PublicBaseURL})
		return
	}
	writeJSON(w, http.StatusOK, resp{Authenticated: true})
}

// handleMe returns the current user profile as WuNest understands it.
// Passes through key fields from WuApi; does NOT include the api_key.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u := auth.FromContext(r.Context())

	type resp struct {
		ID              int64  `json:"wuapi_user_id"`
		Username        string `json:"username"`
		FirstName       string `json:"first_name"`
		Tier            string `json:"tier"`
		GoldBalanceNano int64  `json:"gold_balance_nano"`
		DailyLimit      int    `json:"daily_limit"`
		UsedToday       int    `json:"used_today"`
	}

	writeJSON(w, http.StatusOK, resp{
		ID:              u.WuApi.ID,
		Username:        u.WuApi.Username,
		FirstName:       u.WuApi.FirstName,
		Tier:            string(u.WuApi.Tier),
		GoldBalanceNano: u.WuApi.GoldBalanceNano,
		DailyLimit:      u.WuApi.DailyLimit,
		UsedToday:       u.WuApi.UsedToday,
	})
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// withRequestLogger logs every HTTP request at INFO with method, path, status, duration.
func withRequestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sr, r)
		logger.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sr.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote", r.RemoteAddr,
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Flush exposes http.Flusher if the wrapped writer supports it (needed for SSE).
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
