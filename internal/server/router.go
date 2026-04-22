// Package server wires middleware, routes, and handlers onto an http.Handler.
package server

import (
	"context"
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
	"github.com/shastitko1970-netizen/wunest/internal/personas"
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
	personas   *personas.Handler
}

func New(deps Deps) *Server {
	resolver := users.NewResolver(deps.Postgres)
	charRepo := characters.NewRepository(deps.Postgres)
	presetRepo := presets.NewRepository(deps.Postgres)
	worldsRepo := worldinfo.NewRepository(deps.Postgres)
	personasRepo := personas.NewRepository(deps.Postgres)
	// Adapter fulfils characters.BookExtractor by creating a Lorebook via
	// worldinfo.Repository and attaching it to the new character. Lives
	// here instead of inside characters/ to keep that package free of a
	// dependency on worldinfo (which imports characters).
	bookExtractor := &worldsBookExtractor{repo: worldsRepo}
	return &Server{
		deps:       deps,
		users:      resolver,
		characters: &characters.Handler{Repo: charRepo, Users: resolver, Books: bookExtractor},
		chats: &chats.Handler{
			Repo:       chats.NewRepository(deps.Postgres),
			Users:      resolver,
			Characters: charRepo,
			Presets:    presetRepo,
			Worlds:     worldsRepo,
			Personas:   personasRepo,
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
			Books:          bookExtractor,
		},
		worlds: &worldinfo.Handler{
			Repo:       worldsRepo,
			Users:      resolver,
			Characters: charRepo,
		},
		personas: &personas.Handler{
			Repo:  personasRepo,
			Users: resolver,
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
	mux.Handle("GET /api/me/appearance", authRequired(http.HandlerFunc(s.handleGetAppearance)))
	mux.Handle("PUT /api/me/appearance", authRequired(http.HandlerFunc(s.handleSetAppearance)))

	// Feature packages register their own routes.
	s.characters.Register(mux, authRequired)
	s.chats.Register(mux, authRequired)
	s.presets.Register(mux, authRequired)
	s.library.Register(mux, authRequired)
	s.worlds.Register(mux, authRequired)
	s.personas.Register(mux, authRequired)

	// Model catalog proxy — pulls from WuApi /v1/models with the user's key.
	mux.Handle("GET /api/models", authRequired(http.HandlerFunc(s.handleModels)))

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

// handleGetAppearance returns the user's saved appearance blob. Empty JSON
// object when the user hasn't customised anything yet — the client treats
// `{}` as "use whatever the selected theme's defaults are".
func (s *Server) handleGetAppearance(w http.ResponseWriter, r *http.Request) {
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
	out := json.RawMessage("{}")
	if len(settings.Appearance) > 0 {
		out = settings.Appearance
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}

// handleSetAppearance replaces settings.appearance with the request body,
// which must be a JSON object. Size-capped at 256 KiB so a runaway custom
// CSS field can't bloat the row.
func (s *Server) handleSetAppearance(w http.ResponseWriter, r *http.Request) {
	u := auth.FromContext(r.Context())
	local, err := s.users.Resolve(r.Context(), u.WuApi.ID)
	if err != nil {
		slog.Error("resolve user", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256*1024)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "body too large or unreadable", http.StatusRequestEntityTooLarge)
		return
	}
	// Validate it's at least parseable JSON — even `{}` qualifies. Prevents
	// us storing random bytes that would blow up future LoadSettings reads.
	var probe any
	if err := json.Unmarshal(body, &probe); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := s.users.SetAppearance(r.Context(), local.ID, json.RawMessage(body)); err != nil {
		slog.Error("set appearance", "err", err)
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

// worldsBookExtractor implements characters.BookExtractor by creating a
// Lorebook via worldinfo.Repository and attaching it to the character.
// Lives here so the `characters` package stays unaware of worldinfo —
// worldinfo already imports characters (for ErrNotFound etc.) and going
// the other way would create a cycle.
type worldsBookExtractor struct {
	repo *worldinfo.Repository
}

func (a *worldsBookExtractor) CreateAndAttach(
	ctx context.Context,
	userID, characterID uuid.UUID,
	name, description string,
	entriesJSON []byte,
) error {
	var entries []worldinfo.Entry
	if err := json.Unmarshal(entriesJSON, &entries); err != nil {
		return err
	}
	// Drop disabled/empty entries on the way in — ST cards often include
	// placeholder rows that would clutter the Lorebook UI.
	cleaned := make([]worldinfo.Entry, 0, len(entries))
	for _, e := range entries {
		if e.Content == "" && len(e.Keys) == 0 {
			continue
		}
		// Default enabled if the field was missing (some ST exports omit it).
		cleaned = append(cleaned, e)
	}
	w, err := a.repo.Create(ctx, userID, name, description, cleaned)
	if err != nil {
		return err
	}
	return a.repo.Attach(ctx, characterID, w.ID)
}
