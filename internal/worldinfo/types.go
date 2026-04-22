// Package worldinfo owns the Lorebook / World Info domain: storage,
// activation, prompt injection, and import from SillyTavern JSON.
//
// A World is a collection of Entries. Each Entry has activation conditions
// (keys, constant flag) and a payload (content + position). At prompt
// assembly time, the Activator inspects the last N chat messages, picks
// the entries whose keys matched (plus any constant entries), and groups
// the injected text by position (before_char / after_char) so the prompt
// builder can splice them into the system message.
//
// Shape compatibility: Entry mirrors CharacterBookEntry from the V3 card
// spec so we can round-trip through character PNG imports and ST .json
// lorebook files without mapping code.
package worldinfo

import (
	"time"

	"github.com/google/uuid"
)

// Position controls where in the system prompt an activated entry is spliced.
// Only a small set is supported in v1 — ST defines more (before_examples,
// after_examples, at_depth) which we'll add alongside instruct/examples later.
const (
	PositionBeforeChar = "before_char" // prepended to system prompt
	PositionAfterChar  = "after_char"  // appended to system prompt
)

// World is a lorebook row. Entries live as an ordered JSONB array.
type World struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Entries     []Entry   `json:"entries"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Summary is a light projection returned by list endpoints — skips the
// entries array to keep the payload small when rendering book cards.
type Summary struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	EntryCount  int       `json:"entry_count"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Entry is one lorebook entry. Field order and names follow the V3
// `CharacterBookEntry` spec; optional fields are omitempty so we round-trip
// through ST .json without spurious diffs.
type Entry struct {
	ID            int      `json:"id,omitempty"`
	Name          string   `json:"name,omitempty"`    // UI label
	Comment       string   `json:"comment,omitempty"` // alias for Name in ST imports
	Keys          []string `json:"keys"`              // OR-match list
	SecondaryKeys []string `json:"secondary_keys,omitempty"`
	Content       string   `json:"content"`
	Enabled       bool     `json:"enabled"`
	Selective     bool     `json:"selective,omitempty"` // if true, AND with secondary
	Constant      bool     `json:"constant,omitempty"`  // always active
	// Insertion order: lower first (more foundational context first). Ties
	// break on entry index.
	InsertionOrder int `json:"insertion_order,omitempty"`
	Priority       int `json:"priority,omitempty"` // ignored in v1
	// "before_char" or "after_char"; empty = "before_char".
	Position      string `json:"position,omitempty"`
	CaseSensitive *bool  `json:"case_sensitive,omitempty"` // default false
	// Depth: how far back into history to scan for keys. 0 = book-level default.
	Depth int `json:"depth,omitempty"`
}

// ActivationInput is what the Activator needs to decide which entries fire.
type ActivationInput struct {
	// Books ordered by attach priority (first wins on ties). Disabled books
	// should be filtered by the caller.
	Books []*World
	// Recent is the tail of chat history, newest-last. Both user and assistant
	// turns are acceptable — ST scans both by default.
	Recent []string
	// DefaultDepth is applied when an entry has Depth == 0.
	DefaultDepth int
}

// Activated is the result of running the activator against some history.
type Activated struct {
	BeforeChar []string // concatenate these before the character system block
	AfterChar  []string // concatenate these after the character system block
	// Traced entries — which keys matched, for future debugging UI.
	Trace []Trace
}

// Trace is a diagnostic breadcrumb per activated entry.
type Trace struct {
	WorldID uuid.UUID `json:"world_id"`
	EntryID int       `json:"entry_id"`
	Reason  string    `json:"reason"` // "constant" | "key: foo" | "key: foo+secondary: bar"
}
