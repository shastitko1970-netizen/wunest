// Package characters owns the character-card domain: types, PNG parsing,
// storage, and HTTP handlers.
//
// Character cards follow the community-standard V2/V3 spec (originally from
// SillyTavern). We intentionally do NOT inherit code from SillyTavern, but
// we DO maintain on-the-wire compatibility with the V3 format so users can
// import/export between WuNest, CHUB, and other clients.
//
// On disk we always store the V3 shape: `spec = "chara_card_v3"`, `data`
// contains the payload. V2 imports are upgraded to V3 shape on write.
package characters

import (
	"time"

	"github.com/google/uuid"
)

// Character is our public representation, returned by the HTTP API.
type Character struct {
	ID        uuid.UUID     `json:"id"`
	Name      string        `json:"name"`
	Data      CharacterData `json:"data"`
	// AvatarURL is the small (400-px max) thumbnail that card previews use.
	AvatarURL string `json:"avatar_url,omitempty"`
	// AvatarOriginalURL points to the unresized uploaded PNG/JPEG in MinIO.
	// Optional — pre-M33 characters and manually-created (no PNG) characters
	// may have an empty original.
	AvatarOriginalURL string   `json:"avatar_original_url,omitempty"`
	Tags              []string `json:"tags"`
	Favorite          bool     `json:"favorite"`
	Spec              string   `json:"spec"` // "chara_card_v3"
	SourceURL         string   `json:"source_url,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// CharacterData mirrors the V3 `data` object — the payload of a character
// card. Stored as JSONB in Postgres.
//
// Field order follows the V3 spec. Unknown fields are preserved via
// Extensions.
type CharacterData struct {
	Name                    string         `json:"name"`
	Description             string         `json:"description,omitempty"`
	Personality             string         `json:"personality,omitempty"`
	Scenario                string         `json:"scenario,omitempty"`
	FirstMes                string         `json:"first_mes,omitempty"`
	MesExample              string         `json:"mes_example,omitempty"`
	CreatorNotes            string         `json:"creator_notes,omitempty"`
	SystemPrompt            string         `json:"system_prompt,omitempty"`
	PostHistoryInstructions string         `json:"post_history_instructions,omitempty"`
	AlternateGreetings      []string       `json:"alternate_greetings,omitempty"`
	Tags                    []string       `json:"tags,omitempty"`
	Creator                 string         `json:"creator,omitempty"`
	CharacterVersion        string         `json:"character_version,omitempty"`
	CharacterBook           *CharacterBook `json:"character_book,omitempty"`
	Extensions              map[string]any `json:"extensions,omitempty"`

	// V3-only fields
	Assets []CardAsset `json:"assets,omitempty"`
	Nickname string     `json:"nickname,omitempty"`
	CreatorNotesMultilingual map[string]string `json:"creator_notes_multilingual,omitempty"`
	Source []string `json:"source,omitempty"`
	GroupOnlyGreetings []string `json:"group_only_greetings,omitempty"`
}

// CharacterBook is an embedded lorebook shipped with the character.
type CharacterBook struct {
	Name            string                `json:"name,omitempty"`
	Description     string                `json:"description,omitempty"`
	ScanDepth       *int                  `json:"scan_depth,omitempty"`
	TokenBudget     *int                  `json:"token_budget,omitempty"`
	RecursiveScanning *bool               `json:"recursive_scanning,omitempty"`
	Extensions      map[string]any        `json:"extensions,omitempty"`
	Entries         []CharacterBookEntry  `json:"entries,omitempty"`
}

// CharacterBookEntry is one entry in a character's embedded lorebook.
// Fields follow the V3 spec; unused optional fields are elided via omitempty.
type CharacterBookEntry struct {
	Keys           []string       `json:"keys"`
	Content        string         `json:"content"`
	Enabled        bool           `json:"enabled"`
	InsertionOrder int            `json:"insertion_order"`
	CaseSensitive  *bool          `json:"case_sensitive,omitempty"`
	Priority       int            `json:"priority,omitempty"`
	ID             int            `json:"id,omitempty"`
	Comment        string         `json:"comment,omitempty"`
	Selective      bool           `json:"selective,omitempty"`
	SecondaryKeys  []string       `json:"secondary_keys,omitempty"`
	Constant       bool           `json:"constant,omitempty"`
	Position       string         `json:"position,omitempty"` // "before_char" | "after_char"
	Name           string         `json:"name,omitempty"`
	Extensions     map[string]any `json:"extensions,omitempty"`
}

// CardAsset is a V3 asset reference (sprite, background, etc.).
type CardAsset struct {
	Type string `json:"type"`
	URI  string `json:"uri"`
	Name string `json:"name,omitempty"`
	Ext  string `json:"ext,omitempty"`
}
