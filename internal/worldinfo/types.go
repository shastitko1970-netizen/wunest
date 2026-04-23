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
// before_char / after_char are wired through to the prompt builder today.
// at_depth / before_an / after_an are stored verbatim for ST JSON round-trip
// fidelity but currently fall back to before_char during activation — the
// extra splice points will be wired once author's-note gets the same
// dispatcher as character splicing. Data written here survives re-exports
// without loss.
const (
	PositionBeforeChar = "before_char" // prepended to system prompt
	PositionAfterChar  = "after_char"  // appended to system prompt
	PositionAtDepth    = "at_depth"    // stored-only: inject at N turns from history end
	PositionBeforeAN   = "before_an"   // stored-only: before Author's Note
	PositionAfterAN    = "after_an"    // stored-only: after Author's Note
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
	// ExcludeRecursion: if true, this entry's content is NOT added to the
	// next recursion pass's scan window. Matches the ST flag by the same
	// name. Useful for "flavor text" entries that happen to contain words
	// that could otherwise trigger cascades.
	ExcludeRecursion bool `json:"exclude_recursion,omitempty"`
	// PreventRecursion: if true, this entry cannot be activated BY recursion
	// (only by the initial history scan). Shipped together with
	// ExcludeRecursion because ST presets use the pair; semantically the
	// former blocks outbound triggers, the latter blocks inbound.
	PreventRecursion bool `json:"prevent_recursion,omitempty"`
	// ── ST v1.12+ match / group / probability fields ──
	// Probability: 0 = unset (treated as 100%). 1..99 = random gate
	// probability; 100 = always. Enforced in the activator.
	Probability int `json:"probability,omitempty"`
	// MatchWholeWords: when true, keys must hit on word boundaries instead
	// of plain substrings — so "cat" won't fire on "concatenate". Stored as
	// a pointer so we can distinguish unset (default) from explicit false.
	MatchWholeWords *bool `json:"match_whole_words,omitempty"`
	// Group: mutually-exclusive activation group. If multiple entries in
	// the same non-empty group match in one pass, only the one with lowest
	// InsertionOrder fires (ties → book then entry index).
	Group string `json:"group,omitempty"`
	// GroupOverride: when true, this entry bypasses the group cap — useful
	// for "always include" group members. Stored but not currently enforced
	// in the activator beyond data fidelity.
	GroupOverride bool `json:"group_override,omitempty"`
	// Role: for PositionAtDepth entries, which role the injected message
	// takes. "system" (default), "user", or "assistant". Stored only today.
	Role string `json:"role,omitempty"`
	// ── Stateful activation rules (stored only, not enforced yet) ──
	// Sticky: once this entry fires, it stays active for this many additional
	// turns. Requires per-chat activation state to enforce.
	Sticky int `json:"sticky,omitempty"`
	// Cooldown: minimum turns between consecutive activations.
	Cooldown int `json:"cooldown,omitempty"`
	// Delay: minimum turns from the start of the chat before this entry
	// is eligible to activate.
	Delay int `json:"delay,omitempty"`
	// AutomationID: tool-calling / slash-command hook. Stored only.
	AutomationID string `json:"automation_id,omitempty"`
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
