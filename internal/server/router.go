// Package server wires middleware, routes, and handlers onto an http.Handler.
package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/shastitko1970-netizen/wunest/internal/auth"
	"github.com/shastitko1970-netizen/wunest/internal/characters"
	"github.com/shastitko1970-netizen/wunest/internal/chats"
	"github.com/shastitko1970-netizen/wunest/internal/config"
	"github.com/shastitko1970-netizen/wunest/internal/db"
	"github.com/shastitko1970-netizen/wunest/internal/library"
	"github.com/shastitko1970-netizen/wunest/internal/presets"
	"github.com/shastitko1970-netizen/wunest/internal/spa"
	"github.com/shastitko1970-netizen/wunest/internal/users"
	"github.com/shastitko1970-netizen/wunest/internal/worldinfo"
	"github.com/shastitko1970-netizen/wunest/internal/wuapi"
)

// Deps is the dependency container for HTTP handlers. Kept flat — easier
// to refactor than growing a constructor.
type Deps struct {
	Config   *config.Config
	Postgres *db.Postgres
	Redis    *db.Redis
	WuApi    *wuapi.Client
	Logger   *slog.Logger
}

type Server struct {
	deps       Deps
	users      *users.Resolver
	characters *characters.Handler
	chats      *chats.Handler
	presets    *presets.Handler
	library    *library.Handler
	worlds     *worldinfo.Handler
}

func New(deps Deps) *Server {
	resolver := users.NewResolver(deps.Postgres)
	charRepo := characters.NewRepository(deps.Postgres)
	presetRepo := presets.NewRepository(deps.Postgres)
	worldsRepo := worldinfo.NewRepository(deps.Postgres)
	return &Server{
		deps:       deps,
		users:      resolver,
		characters: &characters.Handler{Repo: charRepo, Users: resolver},
		chats: &chats.Handler{
			Repo:       chats.NewRepository(deps.Postgres),
			Users:      resolver,
			Characters: charRepo,
			Presets:    presetRepo,
			Worlds:     worldsRepo,
			WuApi:      deps.WuApi,
		},
		presets: &presets.Handler{
			Repo:  presetRepo,
			Users: resolver,
		},
		library: &library.Handler{
			Client:         library.NewClient(),
			Users:          resolver,
			CharactersRepo: charRepo,
		},
		worlds: &worldinfo.Handler{
			Repo:       worldsRepo,
			Users:      resolver,
			Characters: charRepo,
		},
	}
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
	mux.Handle("GET /api/me/stats", authRequired(http.HandlerFunc(s.handleMeStats)))
	mux.Handle("GET /api/me/gold/transactions", authRequired(http.HandlerFunc(s.handleGoldTransactions)))
	mux.Handle("GET /api/me/defaults", authRequired(http.HandlerFunc(s.handleGetDefaults)))
	mux.Handle("PUT /api/me/defaults", authRequired(http.HandlerFunc(s.handleSetDefault)))

	// Feature packages register their own routes.
	s.characters.Register(mux, authRequired)
	s.chats.Register(mux, authRequired)
	s.presets.Register(mux, authRequired)
	s.library.Register(mux, authRequired)
	s.worlds.Register(mux, authRequired)

	// Model catalog proxy — pulls from WuApi /v1/models with the user's key.
	mux.Handle("GET /api/models", authRequired(http.HandlerFunc(s.handleModels)))

	// TODO: /api/personas/*

	// Catch-all: SPA (embedded Vue bundle). Must be LAST so that specific
	// routes above take priority. Vue Router handles client-side history.
	mux.Handle("/", spa.Handler())

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

// handleAuthCheck reports whether the caller is logged in. Used by the SPA
// at page-load to decide between showing the app or redirecting to
// wusphere.ru/login.
func (s *Server) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	u := auth.FromContext(r.Context())
	type resp struct {
		Authenticated bool   `json:"authenticated"`
		LoginURL      string `json:"login_url,omitempty"`
	}
	if u == nil {
		writeJSON(w, http.StatusOK, resp{
			Authenticated: false,
			LoginURL:      "https://wusphere.ru/login?return_to=" + s.deps.Config.PublicBaseURL,
		})
		return
	}
	writeJSON(w, http.StatusOK, resp{Authenticated: true})
}

// handleMe returns the current user profile as WuNest understands it.
// Pass-through of WuApi's /api/me plus our local wuapi_user_id. Does NOT
// include the API key — that's private to the session cookie and server-
// side code, never travels to the browser.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u := auth.FromContext(r.Context())

	// Ensure the local nest_users row exists (upsert) and bump last_active.
	_, _ = s.users.Resolve(r.Context(), u.WuApi.ID)

	type resp struct {
		ID              int64      `json:"wuapi_user_id"`
		Username        string     `json:"username"`
		FirstName       string     `json:"first_name"`
		Tier            string     `json:"tier"`
		TierExpiresAt   *time.Time `json:"tier_expires_at,omitempty"`
		GoldBalanceNano int64      `json:"gold_balance_nano"`
		ReferralCount   int        `json:"referral_count"`
		DailyLimit      int        `json:"daily_limit"`
		UsedToday       int        `json:"used_today"`
		CreatedAt       time.Time  `json:"created_at"`
	}

	writeJSON(w, http.StatusOK, resp{
		ID:              u.WuApi.ID,
		Username:        u.WuApi.Username,
		FirstName:       u.WuApi.FirstName,
		Tier:            string(u.WuApi.Tier),
		TierExpiresAt:   u.WuApi.TierExpiresAt,
		GoldBalanceNano: u.WuApi.GoldBalanceNano,
		ReferralCount:   u.WuApi.ReferralCount,
		DailyLimit:      u.WuApi.DailyLimit,
		UsedToday:       u.WuApi.UsedToday,
		CreatedAt:       u.WuApi.CreatedAt,
	})
}

// handleMeStats proxies GET /api/me/stats from WuApi.
func (s *Server) handleMeStats(w http.ResponseWriter, r *http.Request) {
	u := auth.FromContext(r.Context())
	if u.WuApi.APIKey == "" {
		http.Error(w, "no api key", http.StatusPreconditionFailed)
		return
	}
	body, resp, err := s.deps.WuApi.Proxy(r.Context(), "/api/me/stats", u.WuApi.APIKey)
	if err != nil {
		slog.Error("wuapi stats proxy", "err", err)
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, body)
}

// handleGoldTransactions proxies GET /api/me/gold/transactions.
func (s *Server) handleGoldTransactions(w http.ResponseWriter, r *http.Request) {
	u := auth.FromContext(r.Context())
	if u.WuApi.APIKey == "" {
		http.Error(w, "no api key", http.StatusPreconditionFailed)
		return
	}
	upstream := "/api/me/gold/transactions"
	if q := r.URL.RawQuery; q != "" {
		upstream += "?" + q
	}
	body, resp, err := s.deps.WuApi.Proxy(r.Context(), upstream, u.WuApi.APIKey)
	if err != nil {
		slog.Error("wuapi gold transactions proxy", "err", err)
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, body)
}

// handleGetDefaults returns the user's map of preset-type → preset-id.
// Missing keys mean "no default is set for that type".
func (s *Server) handleGetDefaults(w http.ResponseWriter, r *http.Request) {
	u := auth.FromContext(r.Context())
	local, err := s.users.Resolve(r.Context(), u.WuApi.ID)
	if err != nil {
		slog.Error("resolve user", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	settings, err := s.users.LoadSettings(r.Context(), local.ID)
	if err != nil {
		slog.Error("load settings", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defaults := settings.DefaultPresets
	if defaults == nil {
		defaults = map[string]uuid.UUID{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"default_presets": defaults})
}

// handleSetDefault updates a single default-preset entry.
// Body: { "type": "sampler", "preset_id": "uuid" | null }.
func (s *Server) handleSetDefault(w http.ResponseWriter, r *http.Request) {
	u := auth.FromContext(r.Context())
	local, err := s.users.Resolve(r.Context(), u.WuApi.ID)
	if err != nil {
		slog.Error("resolve user", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	var req struct {
		Type     string     `json:"type"`
		PresetID *uuid.UUID `json:"preset_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Type == "" {
		http.Error(w, "type required", http.StatusBadRequest)
		return
	}
	id := uuid.Nil
	if req.PresetID != nil {
		id = *req.PresetID
	}
	if err := s.users.SetDefaultPreset(r.Context(), local.ID, req.Type, id); err != nil {
		slog.Error("set default preset", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleModels proxies GET /api/models → WuApi GET /v1/models using the
// caller's API key. The response is passed through verbatim so the SPA can
// render whatever WuApi decides to expose (tier-filtered list).
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	u := auth.FromContext(r.Context())
	apiKey := u.WuApi.APIKey
	if apiKey == "" {
		http.Error(w, "no api key", http.StatusPreconditionFailed)
		return
	}

	body, resp, err := s.deps.WuApi.GetModels(r.Context(), apiKey)
	if err != nil {
		slog.Error("wuapi models", "err", err)
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	defer body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, body)
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
