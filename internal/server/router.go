// Package server wires middleware, routes, and handlers onto an http.Handler.
package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/shastitko1970-netizen/wunest/internal/auth"
	"github.com/shastitko1970-netizen/wunest/internal/characters"
	"github.com/shastitko1970-netizen/wunest/internal/chats"
	"github.com/shastitko1970-netizen/wunest/internal/config"
	"github.com/shastitko1970-netizen/wunest/internal/db"
	"github.com/shastitko1970-netizen/wunest/internal/byok"
	"github.com/shastitko1970-netizen/wunest/internal/library"
	"github.com/shastitko1970-netizen/wunest/internal/personas"
	"github.com/shastitko1970-netizen/wunest/internal/presets"
	"github.com/shastitko1970-netizen/wunest/internal/quickreplies"
	"github.com/shastitko1970-netizen/wunest/internal/spa"
	"github.com/shastitko1970-netizen/wunest/internal/storage"
	"github.com/shastitko1970-netizen/wunest/internal/uploads"
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
	byok       *byok.Handler
	byokRepo   *byok.Repository // stream hot path calls Reveal directly
	uploads    *uploads.Handler
	quickReplies *quickreplies.Handler
}

func New(deps Deps) *Server {
	resolver := users.NewResolver(deps.Postgres)
	charRepo := characters.NewRepository(deps.Postgres)
	presetRepo := presets.NewRepository(deps.Postgres)
	worldsRepo := worldinfo.NewRepository(deps.Postgres)
	personasRepo := personas.NewRepository(deps.Postgres)
	// BYOK repo refuses to init with a wrong-sized key — makes a
	// mis-configured deploy die at startup rather than silently writing
	// unreadable rows.
	byokRepo, err := byok.NewRepository(deps.Postgres, deps.Config.SecretsKey)
	if err != nil {
		deps.Logger.Error("byok: failed to init repo; keys will be unavailable", "err", err)
		byokRepo = nil
	}
	// Object storage (MinIO). nil storage is valid — Put* returns
	// ErrDisabled on dev laptops without MinIO running. Character import
	// and avatar flows fall back to "no thumbnail" in that case.
	storageClient, err := storage.New(storage.Config{
		Endpoint:      deps.Config.MinIOEndpoint,
		AccessKey:     deps.Config.MinIOAccessKey,
		SecretKey:     deps.Config.MinIOSecretKey,
		UseSSL:        deps.Config.MinIOUseSSL,
		PublicBaseURL: deps.Config.MinIOPublicBaseURL,
	})
	if err != nil {
		deps.Logger.Error("storage: init failed; uploads disabled", "err", err)
		storageClient = nil
	} else if storageClient == nil {
		deps.Logger.Info("storage: MINIO_ENDPOINT not set — image uploads disabled")
	} else {
		deps.Logger.Info("storage: enabled",
			"endpoint", deps.Config.MinIOEndpoint,
			"public_base", deps.Config.MinIOPublicBaseURL,
		)
	}
	// Adapter fulfils characters.BookExtractor by creating a Lorebook via
	// worldinfo.Repository and attaching it to the new character. Lives
	// here instead of inside characters/ to keep that package free of a
	// dependency on worldinfo (which imports characters).
	bookExtractor := &worldsBookExtractor{repo: worldsRepo}
	return &Server{
		deps:       deps,
		users:      resolver,
		characters: &characters.Handler{Repo: charRepo, Users: resolver, Books: bookExtractor, Storage: storageClient},
		chats: &chats.Handler{
			Repo:       chats.NewRepository(deps.Postgres),
			Users:      resolver,
			Characters: charRepo,
			Presets:    presetRepo,
			Worlds:     worldsRepo,
			Personas:   personasRepo,
			BYOK:       byokRepo, // may be nil if init failed; resolveAPIKey handles that
			WuApi:      deps.WuApi,
			Storage:    storageClient, // M39.4 image-gen needs this for base64 rehost
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
		byok: &byok.Handler{
			Repo:  byokRepo,
			Users: resolver,
		},
		byokRepo:     byokRepo,
		uploads:      &uploads.Handler{Storage: storageClient},
		quickReplies: &quickreplies.Handler{Repo: quickreplies.NewRepository(deps.Postgres), Users: resolver},
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
	// /auth/start — logs the sign-in initiation (UA, IP, return_to) and
	// 302-redirects to WuApi. Lets us see server-side EXACTLY when each user
	// attempts to log in, which is most of the battle when debugging a
	// "can't sign in on mobile" report. Called by the SPA's Sign-In button
	// instead of pointing directly at api.wusphere.ru.
	mux.HandleFunc("GET /auth/start", s.handleAuthStart)
	mux.Handle("GET /api/me", authRequired(http.HandlerFunc(s.handleMe)))
	mux.Handle("GET /api/me/stats", authRequired(http.HandlerFunc(s.handleMeStats)))
	mux.Handle("GET /api/me/gold/transactions", authRequired(http.HandlerFunc(s.handleGoldTransactions)))
	mux.Handle("POST /api/me/nest-access/redeem", authRequired(http.HandlerFunc(s.handleNestRedeem)))
	mux.Handle("GET /api/me/defaults", authRequired(http.HandlerFunc(s.handleGetDefaults)))
	mux.Handle("PUT /api/me/defaults", authRequired(http.HandlerFunc(s.handleSetDefault)))
	mux.Handle("GET /api/me/appearance", authRequired(http.HandlerFunc(s.handleGetAppearance)))
	mux.Handle("PUT /api/me/appearance", authRequired(http.HandlerFunc(s.handleSetAppearance)))
	mux.Handle("GET /api/me/default-model", authRequired(http.HandlerFunc(s.handleGetDefaultModel)))
	mux.Handle("PUT /api/me/default-model", authRequired(http.HandlerFunc(s.handleSetDefaultModel)))

	// Feature packages register their own routes.
	s.characters.Register(mux, authRequired)
	s.chats.Register(mux, authRequired)
	s.presets.Register(mux, authRequired)
	s.library.Register(mux, authRequired)
	s.worlds.Register(mux, authRequired)
	s.personas.Register(mux, authRequired)
	if s.byok != nil && s.byok.Repo != nil {
		s.byok.Register(mux, authRequired)
	}
	// Uploads endpoints register unconditionally; when MinIO isn't
	// configured the handler returns 503 with a machine-readable error.
	s.uploads.Register(mux, authRequired)
	s.quickReplies.Register(mux, authRequired)

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

// handleAuthStart is the server-side entry point for "Sign In" clicks. The
// SPA sends the user here with a `return_to` query param; we log the
// attempt (UA, IP, return_to, whether the user already had a session
// cookie) and 302 them onward to WuApi's /auth/refresh with the same
// return_to preserved. This costs one extra redirect but gives us a
// durable server-side record of login initiations — so if a tester says
// "I tried to sign in from my phone and it bounced me", we can find the
// exact line in the log.
func (s *Server) handleAuthStart(w http.ResponseWriter, r *http.Request) {
	returnTo := r.URL.Query().Get("return_to")
	if returnTo == "" {
		returnTo = s.deps.Config.PublicBaseURL
	}

	// Capture context for the log event.
	ua := r.UserAgent()
	if len(ua) > 140 {
		ua = ua[:140]
	}
	hasExisting := false
	if c, err := r.Cookie(s.deps.Config.SessionCookieName); err == nil && c.Value != "" {
		hasExisting = true
	}
	slog.Info("auth_start",
		"outcome", "redirect_to_wuapi",
		"return_to", returnTo,
		"had_existing_session", hasExisting,
		"ua", ua,
		"remote", r.RemoteAddr,
		"xff", r.Header.Get("X-Forwarded-For"),
	)

	// Build the WuApi URL and redirect. We pass the return_to through
	// unchanged — WuApi handles URL-encoding its own way.
	wuapiLogin := "https://api.wusphere.ru/auth/refresh?return_to=" + url.QueryEscape(returnTo)
	http.Redirect(w, r, wuapiLogin, http.StatusFound)
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
		// Use our own /auth/start so any client that follows this URL
		// also goes through the server-side login log.
		writeJSON(w, http.StatusOK, resp{
			Authenticated: false,
			LoginURL:      "/auth/start?return_to=" + url.QueryEscape(s.deps.Config.PublicBaseURL),
		})
		return
	}
	writeJSON(w, http.StatusOK, resp{Authenticated: true})
}

// handleMe returns the current user profile as WuNest understands it.
// Pass-through of WuApi's /api/me plus our local wuapi_user_id. Does NOT
// include the API key — that's private to the session cookie and server-
// side code, never travels to the browser.
//
// active_presets is also inlined (was a separate /api/me/defaults call) so
// the SPA's first paint already knows which presets are active and can
// render chip labels without a second round-trip.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u := auth.FromContext(r.Context())

	// Ensure the local nest_users row exists (upsert) and bump last_active.
	nu, _ := s.users.Resolve(r.Context(), u.WuApi.ID)

	// Best-effort load of active presets. On failure we return an empty
	// map rather than a 500 — the UI will just show "no active preset"
	// which is correct for a fresh user anyway.
	active := map[string]string{}
	if nu != nil {
		if settings, err := s.users.LoadSettings(r.Context(), nu.ID); err == nil && settings != nil {
			for t, id := range settings.DefaultPresets {
				active[t] = id.String()
			}
		}
	}

	type resp struct {
		ID                int64             `json:"wuapi_user_id"`
		Username          string            `json:"username"`
		FirstName         string            `json:"first_name"`
		Tier              string            `json:"tier"`
		TierExpiresAt     *time.Time        `json:"tier_expires_at,omitempty"`
		GoldBalanceNano   int64             `json:"gold_balance_nano"`
		ReferralCount     int               `json:"referral_count"`
		DailyLimit        int               `json:"daily_limit"`
		UsedToday         int               `json:"used_today"`
		CreatedAt         time.Time         `json:"created_at"`
		NestAccessGranted bool              `json:"nest_access_granted"`
		ActivePresets     map[string]string `json:"active_presets"`
	}

	writeJSON(w, http.StatusOK, resp{
		ID:                u.WuApi.ID,
		Username:          u.WuApi.Username,
		FirstName:         u.WuApi.FirstName,
		Tier:              string(u.WuApi.Tier),
		TierExpiresAt:     u.WuApi.TierExpiresAt,
		GoldBalanceNano:   u.WuApi.GoldBalanceNano,
		ReferralCount:     u.WuApi.ReferralCount,
		DailyLimit:        u.WuApi.DailyLimit,
		UsedToday:         u.WuApi.UsedToday,
		CreatedAt:         u.WuApi.CreatedAt,
		NestAccessGranted: u.WuApi.NestAccessGranted,
		ActivePresets:     active,
	})
}

// handleNestRedeem proxies POST /api/me/nest-access/redeem to WuApi so
// the session cookie and CORS stay on one origin. WuApi validates the
// code atomically and flips the users.nest_access_granted flag; on the
// next /api/me the SPA sees the new value and drops the lock screen.
//
// Body / response pass through verbatim — WuApi already returns the
// updated user JSON on success and a {error: ...} with the right status
// on failure.
func (s *Server) handleNestRedeem(w http.ResponseWriter, r *http.Request) {
	u := auth.FromContext(r.Context())
	if u.WuApi.APIKey == "" {
		http.Error(w, "no api key", http.StatusPreconditionFailed)
		return
	}
	body, resp, err := s.deps.WuApi.ProxyPOST(r.Context(), "/api/me/nest-access/redeem", u.WuApi.APIKey, r.Body)
	if err != nil {
		slog.Error("wuapi nest-access redeem", "err", err)
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, body)
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

// handleGetDefaultModel returns the user's stored preferred model id, or
// empty when not set. The client uses this to hydrate a picker in Settings.
func (s *Server) handleGetDefaultModel(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, map[string]any{"default_model": settings.DefaultModel})
}

// handleSetDefaultModel updates settings.default_model. Body:
// { "default_model": "wu-claude" } or { "default_model": "" } to clear.
func (s *Server) handleSetDefaultModel(w http.ResponseWriter, r *http.Request) {
	u := auth.FromContext(r.Context())
	local, err := s.users.Resolve(r.Context(), u.WuApi.ID)
	if err != nil {
		slog.Error("resolve user", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	var req struct {
		DefaultModel string `json:"default_model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := s.users.SetDefaultModel(r.Context(), local.ID, req.DefaultModel); err != nil {
		slog.Error("set default model", "err", err)
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

// withRequestLogger logs every HTTP request at INFO with method, path, status,
// duration, and extra context that helps diagnose auth issues post-facto.
//
// We emit:
//   - ua           : User-Agent header (helps identify mobile/desktop/browser)
//   - referer      : where the request was triggered from (detects cross-sub
//                    redirects from WuApi's /auth/refresh flow)
//   - xff          : X-Forwarded-For chain (nginx populates this; useful for
//                    correlating a single user's requests across pages)
//   - cookie_sess  : whether the wu_session cookie was present (never the
//                    value itself — that's a bearer token)
//
// The cookie-presence flag makes it trivial to grep logs like:
//   grep 'path=/api/auth/check' | grep 'cookie_sess=false'
// to find users whose browser isn't sending the session back after a login.
func withRequestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sr, r)

		// Cookie presence only — never log the value.
		hasSession := false
		if c, err := r.Cookie("wu_session"); err == nil && c.Value != "" {
			hasSession = true
		}

		// Truncate UA to keep log lines scannable; full UA is rarely needed
		// and very noisy with Chromium's brand-list soup.
		ua := r.UserAgent()
		if len(ua) > 140 {
			ua = ua[:140]
		}

		logger.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sr.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote", r.RemoteAddr,
			"xff", r.Header.Get("X-Forwarded-For"),
			"ua", ua,
			"referer", r.Header.Get("Referer"),
			"cookie_sess", hasSession,
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
