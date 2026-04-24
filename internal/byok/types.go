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
	Masked    string    `json:"masked"`             // e.g. "sk-...6411"
	BaseURL   string    `json:"base_url,omitempty"` // provider API root, e.g. https://api.openai.com/v1
	CreatedAt time.Time `json:"created_at"`
}

// CreateInput is what the handler passes to the repo. Plaintext is
// encrypted inside the repo's Create method before ever touching the DB.
type CreateInput struct {
	UserID   uuid.UUID
	Provider string
	Label    string
	Key      string // plaintext; cleared from memory after Create returns
	BaseURL  string // provider API root; defaulted per-provider if empty
}

// Revealed carries a decrypted key plus the base URL it should be sent
// to, and the provider string so callers can apply provider-specific
// request shaping (e.g. stripping top_k for OpenAI, picking the right
// reasoning payload shape). Used by the chat stream to route directly to
// the real provider (bypassing WuApi's proxy) when the chat is pinned to
// a BYOK.
type Revealed struct {
	Key      string
	BaseURL  string
	Provider string
}

// SupportedProviders is the allow-list the handler enforces. Keeping it
// explicit means a typo doesn't silently create a "oppenai" row; it's also
// the source of truth for the client-side <v-select>.
//
// Order matters: most-popular providers first so the default-selected
// option in the UI is the one most users want.
var SupportedProviders = []string{
	"openai",
	"openrouter",
	"deepseek",
	"mistral",
	"anthropic",
	"google",
	"custom",
}

// providerBaseURL is the canonical OpenAI-compatible root for each
// provider. "Custom" has no default — the user must supply one.
//
// Anthropic and Google are OpenAI-compat endpoints that accept the same
// request shape as OpenAI (Anthropic's /v1 Messages endpoint is NOT
// OpenAI-compat; the URL below hits their OpenAI-compatibility layer).
// Users who need native Anthropic/Google formats can point "custom" at a
// proxy like OpenRouter instead.
var providerBaseURL = map[string]string{
	"openai":     "https://api.openai.com/v1",
	"openrouter": "https://openrouter.ai/api/v1",
	"deepseek":   "https://api.deepseek.com/v1",
	"mistral":    "https://api.mistral.ai/v1",
	"anthropic":  "https://api.anthropic.com/v1", // requires their OpenAI-compat beta header
	"google":     "https://generativelanguage.googleapis.com/v1beta/openai",
	"custom":     "",
}

// DefaultBaseURL returns the canonical API root for a provider, or empty
// if the provider is "custom" (URL must be user-supplied) or unknown.
func DefaultBaseURL(provider string) string {
	return providerBaseURL[provider]
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
