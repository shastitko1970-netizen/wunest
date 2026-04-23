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
	"errors"
	"log/slog"
	"net/http"

	"github.com/shastitko1970-netizen/wunest/internal/config"
	"github.com/shastitko1970-netizen/wunest/internal/db"
	"github.com/shastitko1970-netizen/wunest/internal/models"
	"github.com/shastitko1970-netizen/wunest/internal/wuapi"
)

type ctxKey int

const userKey ctxKey = iota

// Middleware returns an http middleware that resolves the session cookie and
// attaches a SessionUser to the request context on success.
//
// Behaviour:
//   - Missing or invalid cookie → 401 if `require` is true, else pass-through.
//   - WuApi down or 5xx        → 503.
//   - User blocked              → 403.
func Middleware(cfg *config.Config, pg *db.Postgres, wu *wuapi.Client, require bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(cfg.SessionCookieName)
			if err != nil || cookie.Value == "" {
				if require {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			profile, err := wu.Me(r.Context(), cookie.Value)
			if err != nil {
				if errors.Is(err, wuapi.ErrUnauthorized) {
					if require {
						http.Error(w, "unauthorized", http.StatusUnauthorized)
						return
					}
					next.ServeHTTP(w, r)
					return
				}
				slog.Error("wuapi me lookup failed", "err", err)
				http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
				return
			}

			if profile.Blocked {
				http.Error(w, "account blocked", http.StatusForbidden)
				return
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
