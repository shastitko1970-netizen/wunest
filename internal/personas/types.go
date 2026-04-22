// Package personas owns user-side personas: the "you" a character talks to.
//
// A Persona is how the user represents themselves in a chat. When a message
// goes upstream, `{{user}}` resolves to the persona's name, and the persona's
// description is threaded into the system prompt as "About the user: …".
//
// Resolution at send time (highest wins):
//  1. The chat's `chat_metadata.persona_id` (per-chat override)
//  2. The user's `is_default=true` persona (account-wide default)
//  3. WuApi profile's first_name (fallback — same as before M7)
package personas

import (
	"time"

	"github.com/google/uuid"
)

// Persona is the wire shape returned to the SPA.
type Persona struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreateInput bundles create-time fields. user_id is provided by the handler.
type CreateInput struct {
	UserID      uuid.UUID
	Name        string
	Description string
	AvatarURL   string
	IsDefault   bool
}

// UpdatePatch — nil pointer means "leave alone".
type UpdatePatch struct {
	Name        *string
	Description *string
	AvatarURL   *string
}
