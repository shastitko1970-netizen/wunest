// Package presets owns generation-setting templates stored in nest_presets.
// A preset is a named, per-user bundle of values that can be re-applied
// to a chat.
//
// V1 UI focus:
//   - sampler   — temperature / top_p / max_tokens / penalties / system prompt
//
// Other types are storable and importable, but only viewable (raw JSON) in
// the Presets manager until dedicated editors ship:
//   - instruct  — input/output/system sequences for text-completion backends
//   - context   — story_string template + separators
//   - sysprompt — plain text system prompt bundle
//   - reasoning — <think>/</think> tag pair for thinking models
//   - openai    — Prompt-Manager block order (future)
package presets

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// PresetType constrains what kinds of presets we persist. Keep in sync with
// the CHECK constraint on nest_presets.type.
type PresetType string

const (
	TypeSampler   PresetType = "sampler"
	TypeOpenAI    PresetType = "openai"
	TypeInstruct  PresetType = "instruct"
	TypeContext   PresetType = "context"
	TypeSysprompt PresetType = "sysprompt"
	TypeReasoning PresetType = "reasoning"
)

// Preset is the wire shape returned by /api/presets.
//
// Data is stored verbatim as JSONB — every preset type has its own schema
// (SamplerData, InstructData, ...). The server doesn't validate field-by-
// field at write time; the frontend / consumer validates when it applies.
// This keeps import from SillyTavern lossless.
type Preset struct {
	ID        uuid.UUID       `json:"id"`
	Type      PresetType      `json:"type"`
	Name      string          `json:"name"`
	Data      json.RawMessage `json:"data"`
	// IsDefault is derived at read-time from the user's settings.default_presets
	// map — it's not a column on nest_presets. Zero-value means "not default".
	IsDefault bool      `json:"is_default,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SamplerData is the typed view of `data` for type=sampler. Matches
// OpenAI-compat parameter names exactly so the chat handler can forward
// most fields unchanged.
//
// Pointer types distinguish "unset → fall back to server default" from
// "user explicitly set to 0".
type SamplerData struct {
	Temperature          *float64 `json:"temperature,omitempty"`
	TopP                 *float64 `json:"top_p,omitempty"`
	MaxTokens            *int     `json:"max_tokens,omitempty"`
	FrequencyPenalty     *float64 `json:"frequency_penalty,omitempty"`
	PresencePenalty      *float64 `json:"presence_penalty,omitempty"`
	SystemPromptOverride string   `json:"system_prompt,omitempty"`
}

// InstructData is the typed view for type=instruct. Field names follow
// SillyTavern's export format so `.json` files import round-trip.
type InstructData struct {
	Enabled           *bool  `json:"enabled,omitempty"`
	InputSequence     string `json:"input_sequence,omitempty"`
	OutputSequence    string `json:"output_sequence,omitempty"`
	SystemSequence    string `json:"system_sequence,omitempty"`
	StopSequence      string `json:"stop_sequence,omitempty"`
	Wrap              *bool  `json:"wrap,omitempty"`
	Macro             *bool  `json:"macro,omitempty"`
	Names             *bool  `json:"names,omitempty"`
	NamesForceGroups  *bool  `json:"names_force_groups,omitempty"`
	ActivationRegex   string `json:"activation_regex,omitempty"`
	// FirstOutputSequence / LastOutputSequence etc. can live in Extensions.
	Extensions map[string]any `json:"extensions,omitempty"`
}

// ContextData is the typed view for type=context.
type ContextData struct {
	StoryString          string `json:"story_string,omitempty"`
	ChatStart            string `json:"chat_start,omitempty"`
	ExampleSeparator     string `json:"example_separator,omitempty"`
	UseStopStrings       *bool  `json:"use_stop_strings,omitempty"`
	NamesAsStopStrings   *bool  `json:"names_as_stop_strings,omitempty"`
	ExamplesInputPrefix  string `json:"examples_input_prefix,omitempty"`
	ExamplesOutputPrefix string `json:"examples_output_prefix,omitempty"`
	Extensions           map[string]any `json:"extensions,omitempty"`
}

// SyspromptData is the typed view for type=sysprompt.
type SyspromptData struct {
	Content   string         `json:"content,omitempty"`
	PostHistory string       `json:"post_history_instructions,omitempty"`
	Extensions map[string]any `json:"extensions,omitempty"`
}

// ReasoningData is the typed view for type=reasoning — <think> tag control
// for thinking models (o1 / Claude thinking / DeepSeek-R1).
type ReasoningData struct {
	Prefix    string `json:"prefix,omitempty"`
	Suffix    string `json:"suffix,omitempty"`
	Separator string `json:"separator,omitempty"`
}

// AsSampler parses Data as SamplerData. Returns a zero value if the
// payload doesn't decode cleanly (e.g. a non-sampler preset fed in by
// mistake) rather than erroring, so callers can fall through to defaults.
func (p Preset) AsSampler() SamplerData {
	var s SamplerData
	_ = json.Unmarshal(p.Data, &s)
	return s
}

// CreateInput is the payload for POST /api/presets.
type CreateInput struct {
	UserID uuid.UUID
	Type   PresetType
	Name   string
	Data   json.RawMessage
}

// UpdatePatch is a sparse update for PATCH /api/presets/:id.
type UpdatePatch struct {
	Name *string
	Data *json.RawMessage
}
