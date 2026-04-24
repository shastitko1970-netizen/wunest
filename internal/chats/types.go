// Package chats owns the chat/message domain: CRUD of chats, prompt
// assembly, and streaming proxy to WuApi's /v1/chat/completions.
//
// A "chat" is a conversation between one NestUser and one character (group
// chats are a future extension). "Messages" are append-only within a chat;
// swipes (alternative assistant outputs for the same turn) live in the
// message's JSONB `swipes` array.
package chats

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/shastitko1970-netizen/wunest/internal/presets"
)

// Chat represents a single conversation thread.
//
// For single-character chats (pre-M35 shape) CharacterID + CharacterName
// are populated and CharacterIDs holds a 1-element array. For group
// chats CharacterIDs is the source of truth (2+ characters); CharacterID
// mirrors CharacterIDs[0] for legacy query paths (e.g. "filter chats by
// character" in Library) and the displayed avatar on the chat list row.
type Chat struct {
	ID            uuid.UUID       `json:"id"`
	UserID        uuid.UUID       `json:"user_id"`
	CharacterID   *uuid.UUID      `json:"character_id,omitempty"`
	CharacterName string          `json:"character_name,omitempty"` // denormalised for list view
	CharacterIDs  []uuid.UUID     `json:"character_ids,omitempty"`  // all participants (group chats)
	IsGroupChat   bool            `json:"is_group_chat,omitempty"`  // derived: len(CharacterIDs) > 1
	Name          string          `json:"name"`
	Tags          []string        `json:"tags"` // free-form user-authored; dedup on read
	Metadata      json.RawMessage `json:"chat_metadata,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	LastMessageAt time.Time       `json:"last_message_at,omitempty"` // derived for sorting
}

// Role is the speaker role in a chat message. Mirrors OpenAI's role taxonomy.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
)

// Message is one turn in a chat.
//
// `swipes` holds alternative assistant outputs (only populated for assistant
// messages that have been regenerated). `swipe_id` is the index of the
// currently displayed alternative; 0 means "Content field is the current".
type Message struct {
	ID        int64           `json:"id"`
	ChatID    uuid.UUID       `json:"chat_id"`
	Role      Role            `json:"role"`
	Content   string          `json:"content"`
	Swipes    json.RawMessage `json:"swipes,omitempty"`
	SwipeID   int             `json:"swipe_id"`
	Extras    json.RawMessage `json:"extras,omitempty"`
	Hidden    bool            `json:"hidden,omitempty"`
	// CharacterID attributes an assistant message to a specific character
	// in a group chat. Nil for user/system messages and for single-
	// character assistant messages where chat.character_id already says
	// who spoke.
	CharacterID *uuid.UUID `json:"character_id,omitempty"`
	// SwipeCharacterIDs is parallel to Swipes: index i here owns index i
	// there. Populated when the message ships with multi-speaker swipes
	// (group greetings at chat-create time). Empty slice / nil means
	// every swipe falls back to CharacterID. Never used for non-group
	// assistant messages.
	SwipeCharacterIDs []uuid.UUID `json:"swipe_character_ids,omitempty"`
	CreatedAt         time.Time   `json:"created_at"`
}

// Summary is one row from nest_chat_summaries. Three kinds live here:
//
//   - role="auto"   — the rolling chat summary maintained by the memory
//                      engine. There's at most one per chat; regen
//                      replaces in place.
//   - role="manual" — free-form user-authored notes ("Act 1 recap",
//                      "things Alice knows but Bob doesn't"). Persist
//                      until the user deletes them. Never auto-replaced.
//   - role="pinned" — critical facts the user wants to always inject
//                      regardless of context budget (e.g. world rules).
//                      Same lifecycle as manual but different UI slot.
//
// CoveredThroughMessageID is the message_id of the LATEST message the
// summary covers. Messages with id > this in history are still played
// fresh to the model. For manual/pinned summaries it's typically nil.
type Summary struct {
	ID                      uuid.UUID  `json:"id"`
	ChatID                  uuid.UUID  `json:"chat_id"`
	Content                 string     `json:"content"`
	Role                    string     `json:"role"` // auto | manual | pinned
	CoveredThroughMessageID *int64     `json:"covered_through_message_id,omitempty"`
	TokenCount              int        `json:"token_count"`
	Model                   string     `json:"model,omitempty"`
	Position                int        `json:"position"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

// ChatGroupMetadata captures per-chat group-chat preferences, stored
// under chat_metadata.group. Everything is optional; missing values
// fall back to the defaults documented on each field.
type ChatGroupMetadata struct {
	// MutedCharacterIDs are excluded from the speaker picker and from
	// round-robin rotation, but still appear in the scene manifest
	// (they're "in the room" — they just don't speak). Nil / empty
	// means nobody is muted.
	MutedCharacterIDs []uuid.UUID `json:"muted_character_ids,omitempty"`
	// AutoSpeaker controls auto-next behaviour:
	//   "" / "manual"       — no auto-advance (client only)
	//   "round_robin"       — after each assistant turn, rotate to the
	//                         next non-muted participant
	AutoSpeaker string `json:"auto_speaker,omitempty"`
	// LastSpeakerID is updated after every assistant turn so round-robin
	// knows where the rotation pointer is. Server-managed, client reads
	// but shouldn't write.
	LastSpeakerID *uuid.UUID `json:"last_speaker_id,omitempty"`
}

// MessageExtras is the typed view of the `extras` JSONB column. Only a
// handful of fields are known today; unknowns round-trip via `Extra`.
type MessageExtras struct {
	Model       string         `json:"model,omitempty"`       // wu-kitsune, anthropic/claude-opus-4.6, etc
	Reasoning   string         `json:"reasoning,omitempty"`   // extracted <think>...</think> block
	TokensIn    int            `json:"tokens_in,omitempty"`
	TokensOut   int            `json:"tokens_out,omitempty"`
	LatencyMs   int            `json:"latency_ms,omitempty"`
	FinishReason string        `json:"finish_reason,omitempty"`
	Error       string         `json:"error,omitempty"`       // populated when generation failed
	// M48 — flag for system messages injected by auto-summarise. Frontend
	// renders regular system messages as a generic pill; we set this so
	// future UX (e.g. «tap to view summary», dismiss, custom icon) can
	// branch on autoSummariseEvent without keyword-matching content.
	AutoSummariseEvent bool `json:"auto_summarise_event,omitempty"`
	Extra       map[string]any `json:"extra,omitempty"`
}

// CreateChatInput captures what a caller needs to start a new chat.
//
// CharacterIDs is the source of truth for participants. When non-empty
// CharacterID will be derived as CharacterIDs[0] by the repo (legacy
// column stays in sync for backward-compat queries). When both are
// empty the chat is character-less (valid — rare, used for sandbox /
// system-prompt-only flows).
type CreateChatInput struct {
	UserID       uuid.UUID
	CharacterID  *uuid.UUID
	CharacterIDs []uuid.UUID
	Name         string
	Metadata     json.RawMessage
}

// SendMessageInput is the body accepted by POST /api/chats/:id/messages.
//
// When Model is empty the handler picks a sensible default (see handler.go).
// Sampler fields are optional per-request overrides that take precedence
// over chat_metadata.sampler. `SystemPromptOverride` is NOT accepted from
// the client body (it's computed server-side from chat_metadata.sampler)
// — the `-` json tag keeps it out of the wire format.
type SendMessageInput struct {
	Content              string         `json:"content"`
	Model                string         `json:"model,omitempty"`
	Temperature          *float64       `json:"temperature,omitempty"`
	TopP                 *float64       `json:"top_p,omitempty"`
	TopK                 *int           `json:"top_k,omitempty"`
	MinP                 *float64       `json:"min_p,omitempty"`
	MaxTokens            *int           `json:"max_tokens,omitempty"`
	FrequencyPenalty     *float64       `json:"frequency_penalty,omitempty"`
	PresencePenalty      *float64       `json:"presence_penalty,omitempty"`
	RepetitionPenalty    *float64       `json:"repetition_penalty,omitempty"`
	Seed                 *int           `json:"seed,omitempty"`
	Stop                 []string       `json:"stop,omitempty"`
	ReasoningEnabled     *bool          `json:"reasoning_enabled,omitempty"`
	PersonaID            *uuid.UUID     `json:"persona_id,omitempty"`
	Overrides            map[string]any `json:"overrides,omitempty"`
	SystemPromptOverride string         `json:"-"`
	// SpeakerID names the character that should respond in a group chat.
	// Must be ∈ chat.character_ids; validated server-side. Single-char
	// chats ignore this field (speaker is always the sole participant).
	SpeakerID *uuid.UUID `json:"speaker_id,omitempty"`
	// Server-populated from chat_metadata.authors_note right before the
	// prompt is built; not accepted from the wire body.
	AuthorsNote *AuthorsNote `json:"-"`
	// Bundle is the full ST-style preset payload (prompts + prompt_order +
	// regex scripts + per-provider flags) extracted from the user's active
	// sampler preset. Server-populated in applyActivePresets so prompt
	// assembly can walk the Prompt Manager and regex scripts can mangle
	// the outgoing content. Never accepted from the wire.
	Bundle *presets.OpenAIBundleData `json:"-"`
}

// ChatSamplerMetadata is the shape of chat_metadata.sampler. Stored as
// JSONB; the chat handler hydrates it before each generation and uses
// its values as defaults (overridable by per-request SendMessageInput).
//
// Field set mirrors SendMessageInput except Content/Model (per-message
// concerns) and SystemPromptOverride (renamed here to match ST's wire form).
type ChatSamplerMetadata struct {
	Temperature          *float64 `json:"temperature,omitempty"`
	TopP                 *float64 `json:"top_p,omitempty"`
	TopK                 *int     `json:"top_k,omitempty"`
	MinP                 *float64 `json:"min_p,omitempty"`
	MaxTokens            *int     `json:"max_tokens,omitempty"`
	FrequencyPenalty     *float64 `json:"frequency_penalty,omitempty"`
	PresencePenalty      *float64 `json:"presence_penalty,omitempty"`
	RepetitionPenalty    *float64 `json:"repetition_penalty,omitempty"`
	Seed                 *int     `json:"seed,omitempty"`
	Stop                 []string `json:"stop,omitempty"`
	ReasoningEnabled     *bool    `json:"reasoning_enabled,omitempty"`
	SystemPromptOverride string   `json:"system_prompt,omitempty"`
	// PresetID is informational — identifies the template that was most
	// recently applied. Not a foreign key (the preset may be deleted or
	// edited after; we copy values at apply-time).
	PresetID *uuid.UUID `json:"preset_id,omitempty"`
}

// AutoSummariseConfig — M44 per-chat auto-summary configuration.
//
// Stored under `chat_metadata.auto_summarise`. Opt-in: a chat without
// this key behaves exactly as before (manual Summariser button only).
// When Enabled is true, after each successful assistant turn we
// compare the turn's prompt-size (tokens_in of the assistant message)
// to ThresholdTokens; if the prompt is ≥ threshold, we fire a
// background SummariseChat call. It uses Model (and optionally BYOKID
// for routing) and bills the user's tokens like a regular generation.
//
// ByokID is a *uuid.UUID so "no pinned BYOK → WuApi pool" is
// expressible as nil, separate from "zero value". Model likewise may
// be "" meaning "fall back to defaultSummariserModel at call time".
//
// ThresholdTokens range UI-enforced 0..2_000_000. Zero or negative is
// treated as "any turn triggers" — arguably silly, but we don't want
// server-side logic to silently normalise and surprise the user.
type AutoSummariseConfig struct {
	Enabled         bool       `json:"enabled"`
	ThresholdTokens int        `json:"threshold_tokens"`
	Model           string     `json:"model,omitempty"`
	BYOKID          *uuid.UUID `json:"byok_id,omitempty"`
}
