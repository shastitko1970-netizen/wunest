// Package server wires middleware, routes, and handlers onto an http.Handler.
package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/shastitko1970-netizen/wunest/internal/auth"
	"github.com/shastitko1970-netizen/wunest/internal/byok"
	"github.com/shastitko1970-netizen/wunest/internal/characters"
	"github.com/shastitko1970-netizen/wunest/internal/chats"
	"github.com/shastitko1970-netizen/wunest/internal/config"
	"github.com/shastitko1970-netizen/wunest/internal/converter"
	"github.com/shastitko1970-netizen/wunest/internal/db"
	"github.com/shastitko1970-netizen/wunest/internal/library"
	"github.com/shastitko1970-netizen/wunest/internal/outboundproxy"
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
	converter  *converter.Handler // M43 — ST theme → WuNest theme via LLM
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
	// Outbound proxy pool for BYOK direct-provider calls. Our server IP is
	// geo-blocked by OpenAI / Anthropic; without a proxy every BYOK request
	// to those providers 403s. A nil pool is legal — calls go direct (good
	// enough for OpenRouter, DeepSeek, Mistral, Google from this server).
	proxyPool, err := outboundproxy.Parse(deps.Config.OutboundProxies)
	if err != nil {
		deps.Logger.Error("outbound proxy: parse failed; BYOK calls will go direct", "err", err)
		proxyPool = nil
	} else if proxyPool != nil {
		deps.Logger.Info("outbound proxy: enabled", "count", proxyPool.Size())
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
			ProxyPool:  proxyPool,     // routes BYOK-direct calls around the geo-block
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
			Repo:       byokRepo,
			Users:      resolver,
			Redis:      deps.Redis.Client,
			ProxyPool:  proxyPool,
		},
		byokRepo:     byokRepo,
		uploads:      &uploads.Handler{Storage: storageClient},
		quickReplies: &quickreplies.Handler{Repo: quickreplies.NewRepository(deps.Postgres), Users: resolver},
		// Converter needs same LLM-call primitives as the chat stream
		// (BYOK + WuApi + proxy pool). Reusing components instead of
		// forking keeps the "where does the LLM call go" logic in one
		// place (chats.PrepareRequestForProvider / DirectCallHeaders).
		converter: &converter.Handler{
			Repo:      converter.NewRepository(deps.Postgres),
			Users:     resolver,
			BYOK:      byokRepo,
			WuApi:     deps.WuApi,
			ProxyPool: proxyPool,
			Logger:    deps.Logger,
		},
	}
}

// StartBackground launches long-running goroutines the server needs
// (periodic reapers, cache warmers). Returns a stop function that
// cancels everything and waits for in-flight work to finish; main.go
// calls it from graceful shutdown.
//
// Currently: converter reaper (deletes expired nest_converter_jobs rows
// every 10 minutes). Add more here as needs arise — keeping one central
// lifecycle hook avoids scattered goroutine wiring.
func (s *Server) StartBackground(ctx context.Context) func() {
	stops := []func(){}
	if s.converter != nil && s.converter.Repo != nil {
		stops = append(stops, converter.StartReaper(ctx, s.converter.Repo, s.deps.Logger))
	}
	return func() {
		for _, stop := range stops {
			stop()
		}
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
	// /auth/logout — clears the wu_session cookie (Domain=.wusphere.ru so
	// the same Set-Cookie header wipes it for nest, api and the marketing
	// site at once) and best-effort POSTs WuApi's /auth/logout so any
	// upstream session bookkeeping is also reset. Public — even a stale
	// or forged cookie should be removable, and we don't want a logout
	// click to 401-loop the user.
	mux.HandleFunc("POST /auth/logout", s.handleAuthLogout)
	mux.Handle("GET /api/me", authRequired(http.HandlerFunc(s.handleMe)))
	mux.Handle("GET /api/me/stats", authRequired(http.HandlerFunc(s.handleMeStats)))
	mux.Handle("GET /api/me/gold/transactions", authRequired(http.HandlerFunc(s.handleGoldTransactions)))
	// M54.2 — proxy WuApi's subscription detail endpoint so the SPA can
	// fetch from same-origin without dealing with cross-domain cookies.
	mux.Handle("GET /api/me/subscription", authRequired(http.HandlerFunc(s.handleMeSubscription)))
	// M54.4 — proxy WuApi's payment-create to start a Yookassa checkout
	// for a WuNest subscription. Body shape mirrors WuApi's
	// createPaymentRequest (passed through verbatim) so the SPA only
	// needs to hit one origin and follow the returned `payment_url`.
	mux.Handle("POST /api/pay/create", authRequired(http.HandlerFunc(s.handlePayCreate)))
	// M54.5 hotfix — proxy WuApi's public pricing catalog so the SPA
	// hits a same-origin endpoint and avoids CORS preflight against
	// api.wusphere.ru. Same payload shape; auth not required (catalog
	// is public on the WuApi side too).
	mux.HandleFunc("GET /api/pricing", s.handlePricing)
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
	s.converter.Register(mux, authRequired)

	// Model catalog proxy — pulls from WuApi /v1/models with the user's key.
	mux.Handle("GET /api/models", authRequired(http.HandlerFunc(s.handleModels)))
	// M55.2 — wu-gold catalog with eco-mode variants. Authed; SPA pulls
	// here to render the picker's separate "Эко-режим" section.
	mux.Handle("GET /api/models/catalog", authRequired(http.HandlerFunc(s.handleModelsCatalog)))

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

// handleAuthLogout clears the wu_session cookie locally and best-effort
// notifies WuApi so its `/auth/logout` (which also clears the same cookie
// with Domain=.wusphere.ru) runs upstream. Both clears are belt-and-
// suspenders: WuApi's response carries a Set-Cookie that wipes the cookie
// across .wusphere.ru subdomains, but if WuApi is unreachable we still
// drop the local copy so the user isn't stuck.
//
// Always returns 204 — logout must be idempotent. A missing cookie, a
// 4xx from WuApi, even a network error: all map to "you are now signed
// out". The SPA redirects to / after this and AppShell's auth-check
// re-runs on the next paint.
func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	ua := r.UserAgent()
	if len(ua) > 140 {
		ua = ua[:140]
	}

	// Forward the user's session key to WuApi so it can invalidate any
	// server-side state and emit its own clearing Set-Cookie. We use a
	// short detached context so a slow/down WuApi can't stall the user's
	// click — 3s ceiling, then we just drop the local cookie ourselves.
	cookie, _ := r.Cookie(s.deps.Config.SessionCookieName)
	sessFp := ""
	if cookie != nil && cookie.Value != "" {
		sessFp = sessionFingerprintShort(cookie.Value)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		body, resp, err := s.deps.WuApi.ProxyPOST(ctx, "/auth/logout", cookie.Value, http.NoBody)
		if err != nil {
			slog.Warn("auth_logout",
				"outcome", "wuapi_unreachable",
				"sess_fp", sessFp,
				"err", err.Error(),
			)
		} else {
			// Forward WuApi's Set-Cookie headers — they carry the
			// canonical Domain=.wusphere.ru clearing instruction. If WuApi
			// happens to also clear other auxiliary cookies (oauth_state
			// etc.), we propagate those too.
			for _, sc := range resp.Header.Values("Set-Cookie") {
				w.Header().Add("Set-Cookie", sc)
			}
			body.Close()
			slog.Info("auth_logout",
				"outcome", "wuapi_ok",
				"sess_fp", sessFp,
				"upstream_status", resp.StatusCode,
			)
		}
	}

	// Local clear — defensive in case WuApi was down or didn't include a
	// Set-Cookie. Same Domain/Path/SameSite as setSessionCookie on WuApi
	// so the browser overwrites the existing cookie with this expired one.
	http.SetCookie(w, &http.Cookie{
		Name:     s.deps.Config.SessionCookieName,
		Value:    "",
		Path:     "/",
		Domain:   s.deps.Config.SessionCookieDomain,
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	slog.Info("auth_logout",
		"outcome", "ok",
		"sess_fp", sessFp,
		"ua", ua,
		"remote", r.RemoteAddr,
	)
	w.WriteHeader(http.StatusNoContent)
}

// sessionFingerprintShort returns the same 8-hex-char sha256 prefix used by
// the auth middleware so logout events line up with login events in the log
// stream. Inlined here rather than exported from auth/ because router.go is
// the only other place that needs it; if a third caller appears, promote.
func sessionFingerprintShort(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:4])
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

// handlePricing proxies GET /api/pay/prices from WuApi as a same-origin
// endpoint (M54.5 hotfix).
//
// The WuApi catalog itself is public, but cross-origin browser fetches
// to api.wusphere.ru fail without CORS headers — and we don't want to
// open up CORS on WuApi just for this one path. Proxying through
// WuNest keeps the SPA's network requests same-origin and the WuApi
// CORS surface unchanged.
//
// No auth here: pricing is the same regardless of who's looking.
func (s *Server) handlePricing(w http.ResponseWriter, r *http.Request) {
	body, resp, err := s.deps.WuApi.Proxy(r.Context(), "/api/pay/prices", "")
	if err != nil {
		slog.Error("wuapi pricing proxy", "err", err)
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, body)
}

// handlePayCreate proxies POST /api/pay/create to WuApi (M54.4).
//
// The SPA POSTs the same body it would send directly to WuApi
// ({type, nest_level, ...}); we just attach the user's WuApi API key
// as Bearer and forward. WuApi creates the YooKassa payment and
// returns {payment_url} which the SPA uses to redirect.
//
// Why proxy at all: WuNest cookies are scoped to nest.wusphere.ru,
// not api.wusphere.ru directly. POSTing to api.wusphere.ru from the
// SPA would either rely on cross-origin cookies (Domain=.wusphere.ru,
// works but couples WuNest's auth to WuApi's session model) or
// require the SPA to ferry the API key in headers (leaks it into the
// SPA bundle). Proxying keeps both authentication and origin tidy.
func (s *Server) handlePayCreate(w http.ResponseWriter, r *http.Request) {
	u := auth.FromContext(r.Context())
	if u.WuApi.APIKey == "" {
		http.Error(w, "no api key", http.StatusPreconditionFailed)
		return
	}
	body, resp, err := s.deps.WuApi.ProxyPOST(r.Context(), "/api/pay/create", u.WuApi.APIKey, r.Body)
	if err != nil {
		slog.Error("wuapi pay/create proxy", "err", err)
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, body)
}

// handleMeSubscription proxies GET /api/me/subscription from WuApi.
// Surfaces the user's WuNest subscription state (level, expiry, monthly
// gold-discount cap usage). The SPA calls this from the subscription
// store on demand; the compact summary in /api/me already covers the
// common case (does the user have plus/pro?), so this path is only hit
// when a screen needs the discount-usage detail.
func (s *Server) handleMeSubscription(w http.ResponseWriter, r *http.Request) {
	u := auth.FromContext(r.Context())
	if u.WuApi.APIKey == "" {
		http.Error(w, "no api key", http.StatusPreconditionFailed)
		return
	}
	body, resp, err := s.deps.WuApi.Proxy(r.Context(), "/api/me/subscription", u.WuApi.APIKey)
	if err != nil {
		slog.Error("wuapi subscription proxy", "err", err)
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

// handleModelsCatalog proxies GET /api/models/catalog →
// WuApi GET /api/catalog/gold?app=wunest&secret=$WUNEST_API_SECRET (M55.2).
//
// Catalog itself is public, but only WuNest-authenticated calls
// receive the `:lite` eco-mode variants — the secret is held
// server-side and never sent to the SPA. The SPA cannot construct
// the WuApi URL on its own, so this is the only surface where lite
// models are fetchable from the browser.
func (s *Server) handleModelsCatalog(w http.ResponseWriter, r *http.Request) {
	body, resp, err := s.deps.WuApi.GetGoldCatalog(r.Context(), s.deps.Config.WuNestAPISecret)
	if err != nil {
		slog.Error("wuapi gold catalog", "err", err)
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", "application/json")
	// Cache shorter than the public side (60s) — eco metadata can
	// change between deploys and we don't want stale caps lingering
	// in the SPA's memory.
	w.Header().Set("Cache-Control", "private, max-age=60")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, body)
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
