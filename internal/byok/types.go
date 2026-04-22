package byok

import (
	"time"

	"github.com/google/uuid"
)

// Key is the wire shape returned to the SPA. Plaintext is NEVER included
// — the client sends a key once and from then on we only return a masked
// preview so it can be identified in the UI.
type Key struct {
	ID        uuid.UUID `json:"id"`
	Provider  string    `json:"provider"`
	Label     string    `json:"label,omitempty"`
	Masked    string    `json:"masked"` // e.g. "sk-...6411"
	CreatedAt time.Time `json:"created_at"`
}

// CreateInput is what the handler passes to the repo. Plaintext is
// encrypted inside the repo's Create method before ever touching the DB.
type CreateInput struct {
	UserID   uuid.UUID
	Provider string
	Label    string
	Key      string // plaintext; cleared from memory after Create returns
}

// SupportedProviders is the allow-list the handler enforces. Keeping it
// explicit means a typo doesn't silently create a "oppenai" row; it's also
// the source of truth for the client-side <v-select>.
var SupportedProviders = []string{
	"openai",
	"anthropic",
	"google",
	"openrouter",
	"mistral",
	"deepseek",
	"custom",
}

// IsSupportedProvider reports whether the given string is in SupportedProviders.
func IsSupportedProvider(p string) bool {
	for _, s := range SupportedProviders {
		if s == p {
			return true
		}
	}
	return false
}

// Mask hides all but the last N characters of a key, for UI display.
// Keys shorter than 8 chars get fully masked (rare, but e.g. test keys).
func Mask(key string) string {
	if len(key) < 8 {
		return "••••"
	}
	return key[:3] + "…" + key[len(key)-4:]
}
