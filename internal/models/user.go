// Package models holds domain types shared across handlers and repositories.
package models

import (
	"time"

	"github.com/google/uuid"
)

// Tier mirrors WuApi's subscription tiers.
type Tier string

const (
	TierFree    Tier = "free"
	TierPlus    Tier = "plus"
	TierPrem    Tier = "prem"
	TierProPlus Tier = "pro_plus"
	TierMax     Tier = "max"
)

// NestUser is a WuNest-local shadow of a WuApi user. One row per user who has
// ever logged into WuNest. Links to the WuApi user via WuApiUserID (no FK —
// the WuApi DB is a separate schema; source of truth is WuApi's /api/me).
type NestUser struct {
	ID           uuid.UUID `json:"id"`
	WuApiUserID  int64     `json:"wuapi_user_id"`
	Settings     []byte    `json:"-"` // raw JSONB: per-user UI prefs, theme, etc.
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
}

// DisplayName returns a human-readable name for substitution in prompts.
// Currently a placeholder — future work wires in persona selection and the
// WuApi first_name fallback. For now it returns "You" which is safer than
// an empty string inside macro expansion.
func (u NestUser) DisplayName() string { return "You" }

// SessionUser is the request-scoped view of the authenticated user —
// combines WuApi's live profile with our local NestUser row.
//
// Resolved by auth middleware, attached to request context for handlers.
type SessionUser struct {
	Local NestUser
	WuApi WuApiProfile
}

// WuApiProfile is the subset of WuApi's /api/me response we care about.
// Mirrors wuapi.MeResponse but lives here to keep the domain pure.
type WuApiProfile struct {
	ID              int64
	Username        string
	FirstName       string
	APIKey          string
	Tier            Tier
	GoldBalanceNano int64
	DailyLimit      int
	UsedToday       int
	Blocked         bool
}
