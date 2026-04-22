// Package presets owns sampler / system-prompt templates stored in
// nest_presets. A preset is a named bundle of generation-settings that the
// user can re-apply across chats.
//
// V1 scope: `sampler` type only (temperature, top_p, max_tokens, system
// prompt override). Other types reserved by the migration (instruct,
// context, sysprompt, reasoning) ship in later milestones.
package presets

import (
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
type Preset struct {
	ID        uuid.UUID  `json:"id"`
	Type      PresetType `json:"type"`
	Name      string     `json:"name"`
	Data      SamplerData `json:"data"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// SamplerData is the payload for `type = 'sampler'`. Kept small for v1 —
// each field matches the equivalent OpenAI-compat parameter name exactly
// so the chat handler can forward most fields unchanged.
//
// Pointer types allow "unset" semantics: a preset that only sets
// temperature leaves every other slider at the chat's / server's default.
type SamplerData struct {
	Temperature      *float64 `json:"temperature,omitempty"`
	TopP             *float64 `json:"top_p,omitempty"`
	MaxTokens        *int     `json:"max_tokens,omitempty"`
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64 `json:"presence_penalty,omitempty"`
	// SystemPromptOverride, if non-empty, replaces (not appends to) the
	// character-derived system message in the built prompt. Useful for
	// "pure NSFW / safety-off / style-strict" presets that want to wipe
	// the default persona altogether.
	SystemPromptOverride string `json:"system_prompt,omitempty"`
}

// DefaultSampler is what a freshly created chat ships with before the
// user changes anything. Matches WuApi's hands-off defaults so users who
// never open the Settings drawer still get reasonable output.
func DefaultSampler() SamplerData {
	t := 1.0
	return SamplerData{Temperature: &t}
}

// CreateInput is the payload for POST /api/presets.
type CreateInput struct {
	UserID uuid.UUID
	Type   PresetType
	Name   string
	Data   SamplerData
}

// UpdatePatch is a sparse update for PATCH /api/presets/:id.
type UpdatePatch struct {
	Name *string
	Data *SamplerData
}
