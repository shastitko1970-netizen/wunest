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
)

// Chat represents a single conversation thread.
type Chat struct {
	ID            uuid.UUID       `json:"id"`
	UserID        uuid.UUID       `json:"user_id"`
	CharacterID   *uuid.UUID      `json:"character_id,omitempty"`
	CharacterName string          `json:"character_name,omitempty"` // denormalised for list view
	Name          string          `json:"name"`
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
	CreatedAt time.Time       `json:"created_at"`
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
	Extra       map[string]any `json:"extra,omitempty"`
}

// CreateChatInput captures what a caller needs to start a new chat.
type CreateChatInput struct {
	UserID      uuid.UUID
	CharacterID *uuid.UUID
	Name        string
	Metadata    json.RawMessage
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
	// Server-populated from chat_metadata.authors_note right before the
	// prompt is built; not accepted from the wire body.
	AuthorsNote *AuthorsNote `json:"-"`
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
