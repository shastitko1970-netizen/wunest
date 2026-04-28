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

// NestLevel is the WuNest-specific subscription axis (independent from
// the LLM-tier `Tier` above). A user can simultaneously have
// `Tier=prem` (LLM access) AND `NestLevel=pro` (WuNest unlimited slots).
//
// Free is the absence of an active subscription, so the enum has only
// the two paid levels — code that wants "current level or free" should
// use *NestLevel and treat nil as free.
type NestLevel string

const (
	NestLevelPlus NestLevel = "plus"
	NestLevelPro  NestLevel = "pro"
)

// NestSubscription is the active subscription view attached to the
// session. Nil = free tier. Source of truth is WuApi's nest_subscriptions
// table; we read it through /api/me on every authenticated request.
type NestSubscription struct {
	Level     NestLevel
	ExpiresAt time.Time
}

// Per-category slot caps. -1 = unlimited (Pro). These mirror WuApi's
// constants of the same names — they have to match because both sides
// independently enforce. WuApi pricing endpoint also surfaces these so
// the SPA can render usage hints without hardcoding.
const (
	NestFreeLimit int = 3
	NestPlusLimit int = 10
	NestProLimit  int = -1
)

// NestLimitFor returns the per-category slot limit for the given level.
// Nil (no active subscription) returns the free-tier limit.
func NestLimitFor(level *NestLevel) int {
	if level == nil {
		return NestFreeLimit
	}
	switch *level {
	case NestLevelPlus:
		return NestPlusLimit
	case NestLevelPro:
		return NestProLimit
	default:
		return NestFreeLimit
	}
}

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
//
// Field naming matches our own convention (snake_case on the wire for our
// API, PascalCase here in Go); the JSON→struct decode happens in the
// wuapi package where the camelCase JSON tags are attached.
type WuApiProfile struct {
	ID                int64
	Username          string
	FirstName         string
	APIKey            string
	Tier              Tier
	TierExpiresAt     *time.Time
	GoldBalanceNano   int64
	ReferralCount     int
	DailyLimit        int
	UsedToday         int
	CreatedAt         time.Time
	Blocked           bool
	NestAccessGranted bool
	// WuNest subscription. nil for free-tier (no active subscription).
	// Drives per-resource slot limits in the limits package.
	NestSubscription *NestSubscription
}

// CurrentNestLevel returns the user's active WuNest level, or nil if
// they are on the free tier (no active subscription).
func (p WuApiProfile) CurrentNestLevel() *NestLevel {
	if p.NestSubscription == nil {
		return nil
	}
	level := p.NestSubscription.Level
	return &level
}
