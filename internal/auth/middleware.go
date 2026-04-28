// Package auth handles session resolution via the shared wu_session cookie.
//
// Flow for every authenticated request:
//  1. Read cookie `wu_session` from the request.
//  2. Ask WuApi /api/me what user it belongs to.
//  3. Upsert a local NestUser row (first login creates it).
//  4. Attach a SessionUser to the request context.
//
// Handlers read the user via FromContext(ctx).
package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"

	"github.com/shastitko1970-netizen/wunest/internal/config"
	"github.com/shastitko1970-netizen/wunest/internal/db"
	"github.com/shastitko1970-netizen/wunest/internal/models"
	"github.com/shastitko1970-netizen/wunest/internal/wuapi"
)

// sessionFingerprint returns a short stable hex string derived from the
// session token. Used in logs to correlate requests from one user without
// ever persisting the token itself. SHA-256 truncated to 8 hex chars
// (~32 bits of entropy) — collision-resistant enough to follow one
// session's trail, not enough to brute-force back to the token.
func sessionFingerprint(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:4])
}

type ctxKey int

const userKey ctxKey = iota

// Middleware returns an http middleware that resolves the session cookie and
// attaches a SessionUser to the request context on success.
//
// Behaviour:
//   - Missing or invalid cookie → 401 if `require` is true, else pass-through.
//   - WuApi down or 5xx        → 503.
//   - User blocked              → 403.
//
// Logging: every outcome emits one structured `auth` log event at INFO (happy
// path) or WARN (failures) so post-mortem on user-reported login loops
// becomes a matter of `grep auth wunest.log | grep <username-or-ua>` rather
// than reverse-engineering timestamps. The session cookie VALUE is never
// logged; only presence + length range + a stable short fingerprint so two
// requests from the same session can be correlated without exposing it.
func Middleware(cfg *config.Config, pg *db.Postgres, wu *wuapi.Client, require bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ua := r.UserAgent()
			if len(ua) > 140 {
				ua = ua[:140]
			}

			cookie, err := r.Cookie(cfg.SessionCookieName)
			if err != nil || cookie.Value == "" {
				// No session cookie. Only log when the caller actually
				// required auth — bare /api/auth/check polls and other
				// optional-auth paths would spam the log otherwise.
				if require {
					slog.Warn("auth",
						"outcome", "no_cookie",
						"path", r.URL.Path,
						"ua", ua,
						"referer", r.Header.Get("Referer"),
					)
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			sessFp := sessionFingerprint(cookie.Value)

			profile, err := wu.Me(r.Context(), cookie.Value)
			if err != nil {
				if errors.Is(err, wuapi.ErrUnauthorized) {
					slog.Warn("auth",
						"outcome", "wuapi_rejected",
						"reason", "cookie present, /api/me returned 401 — session expired or forged",
						"sess_fp", sessFp,
						"path", r.URL.Path,
						"ua", ua,
					)
					if require {
						http.Error(w, "unauthorized", http.StatusUnauthorized)
						return
					}
					next.ServeHTTP(w, r)
					return
				}
				slog.Error("auth",
					"outcome", "wuapi_error",
					"err", err.Error(),
					"sess_fp", sessFp,
					"path", r.URL.Path,
					"ua", ua,
				)
				http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
				return
			}

			if profile.Blocked {
				slog.Warn("auth",
					"outcome", "blocked",
					"user_id", profile.ID,
					"username", profile.Username,
					"path", r.URL.Path,
				)
				http.Error(w, "account blocked", http.StatusForbidden)
				return
			}

			// Happy path — log only for the entry endpoints where it matters
			// (auth/check, /api/me). Other routes would spam.
			if r.URL.Path == "/api/auth/check" || r.URL.Path == "/api/me" {
				slog.Info("auth",
					"outcome", "ok",
					"user_id", profile.ID,
					"username", profile.Username,
					"nest_access_granted", profile.NestAccessGranted,
					"sess_fp", sessFp,
					"path", r.URL.Path,
					"ua", ua,
				)
			}

			// TODO: upsert nest_users row and load Local fields.
			// Stubbed for the skeleton — populated when migrations run.
			sessionUser := &models.SessionUser{
				Local: models.NestUser{WuApiUserID: profile.ID},
				WuApi: models.WuApiProfile{
					ID:                profile.ID,
					Username:          profile.Username,
					FirstName:         profile.FirstName,
					APIKey:            profile.APIKey,
					Tier:              models.Tier(profile.Tier),
					TierExpiresAt:     profile.TierExpiresAt,
					GoldBalanceNano:   profile.GoldBalanceNano,
					ReferralCount:     profile.ReferralCount,
					DailyLimit:        profile.DailyLimit,
					UsedToday:         int(profile.UsedToday),
					CreatedAt:         profile.CreatedAt,
					Blocked:           profile.Blocked,
					NestAccessGranted: profile.NestAccessGranted,
				},
			}

			// M54.2 — WuNest subscription. Nil for free-tier users (no
			// active subscription on WuApi side). Limits package consumes
			// CurrentNestLevel() to gate per-resource creates.
			if sub := profile.NestSubscription; sub != nil {
				sessionUser.WuApi.NestSubscription = &models.NestSubscription{
					Level:     models.NestLevel(sub.Level),
					ExpiresAt: sub.ExpiresAt,
				}
			}

			ctx := context.WithValue(r.Context(), userKey, sessionUser)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// FromContext retrieves the authenticated user from the request context.
// Returns nil if the request was not authenticated.
func FromContext(ctx context.Context) *models.SessionUser {
	u, _ := ctx.Value(userKey).(*models.SessionUser)
	return u
}

// RequireNestAccess used to enforce the WuNest beta gate at the HTTP
// layer (M29). Disabled 2026-04-25 — we opened public access, so every
// authenticated user is allowed through. The middleware itself is
// kept (and still wired in chats handler etc.) so the gate can be
// re-enabled by flipping `gateEnabled` to true if we ever want a
// closed beta again.
//
// History:
//
//	mux.Handle("POST /api/chats/{id}/messages",
//	    authRequired(auth.RequireNestAccess(http.HandlerFunc(h.sendMessage))))
//
// During the closed beta this checked `su.WuApi.NestAccessGranted`
// and returned 403 + `{error:"nest_access_required"}` for un-redeemed
// users. With public access the middleware is a transparent
// pass-through that still requires authentication (the auth check
// guards against `su == nil`).
const gateEnabled = false

func RequireNestAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		su := FromContext(r.Context())
		if gateEnabled && (su == nil || !su.WuApi.NestAccessGranted) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusForbidden)
			// Tagged error body — SPA can key off `error` to show a
			// "need access code" UI instead of a generic failure.
			_, _ = w.Write([]byte(`{"error":"nest_access_required","message":"WuNest is in closed beta — redeem an access code to use generation endpoints."}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
