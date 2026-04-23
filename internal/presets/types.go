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

// ─── OpenAI-style preset bundle (M32) ────────────────────────────────
//
// OpenAIBundleData is the full typed view of a SillyTavern "OpenAI-style"
// completion preset. Unlike SamplerData (which covers the ~6 common
// knobs), this struct understands the entire preset file so imported ST
// presets like DarkNet V3 can be applied with their full behavior:
//
//   - 111 prompt blocks with per-block role + enabled + injection_position
//     (the "Prompt Manager" in ST)
//   - Regex scripts that munge user input and assistant output on the fly
//     (jailbreak-style invisible-char tricks etc.)
//   - Per-provider flags (Claude sysprompt handling, Gemini squash, …)
//   - Continue / impersonation / group-nudge prompts
//   - Multimodal gates (image/video inlining, function calling)
//
// Unknown fields in the preset data are preserved because our Preset.Data
// is JSONB — this struct just gives the server a typed read path.
type OpenAIBundleData struct {
	// Sampler knobs (ST uses `openai_max_tokens`, we also read `max_tokens`).
	Temperature        *float64 `json:"temperature,omitempty"`
	TopP               *float64 `json:"top_p,omitempty"`
	TopK               *int     `json:"top_k,omitempty"`
	TopA               *float64 `json:"top_a,omitempty"`
	MinP               *float64 `json:"min_p,omitempty"`
	FrequencyPenalty   *float64 `json:"frequency_penalty,omitempty"`
	PresencePenalty    *float64 `json:"presence_penalty,omitempty"`
	RepetitionPenalty  *float64 `json:"repetition_penalty,omitempty"`
	Seed               *int64   `json:"seed,omitempty"`
	MaxTokens          *int     `json:"max_tokens,omitempty"`
	OpenAIMaxTokens    *int     `json:"openai_max_tokens,omitempty"`
	OpenAIMaxContext   *int     `json:"openai_max_context,omitempty"`
	MaxContextUnlocked *bool    `json:"max_context_unlocked,omitempty"`
	N                  *int     `json:"n,omitempty"`
	StreamOpenAI       *bool    `json:"stream_openai,omitempty"`

	// Prompt Manager: the array of named prompt blocks + the ordered list
	// of which ones are enabled and in what order.
	Prompts     []PromptBlock       `json:"prompts,omitempty"`
	PromptOrder []PromptOrderGroup  `json:"prompt_order,omitempty"`

	// Per-provider behavior / prefills.
	AssistantPrefill       string `json:"assistant_prefill,omitempty"`
	AssistantImpersonation string `json:"assistant_impersonation,omitempty"`
	ClaudeUseSysprompt     *bool  `json:"claude_use_sysprompt,omitempty"`
	UseMakersuiteSysprompt *bool  `json:"use_makersuite_sysprompt,omitempty"`
	SquashSystemMessages   *bool  `json:"squash_system_messages,omitempty"`

	// Continue / impersonation / nudge prompts.
	NewChatPrompt        string `json:"new_chat_prompt,omitempty"`
	NewGroupChatPrompt   string `json:"new_group_chat_prompt,omitempty"`
	NewExampleChatPrompt string `json:"new_example_chat_prompt,omitempty"`
	ContinueNudgePrompt  string `json:"continue_nudge_prompt,omitempty"`
	ContinuePrefill      *bool  `json:"continue_prefill,omitempty"`
	ContinuePostfix      string `json:"continue_postfix,omitempty"`
	ImpersonationPrompt  string `json:"impersonation_prompt,omitempty"`
	GroupNudgePrompt     string `json:"group_nudge_prompt,omitempty"`

	// Format / output presentation.
	WIFormat          string `json:"wi_format,omitempty"`
	ScenarioFormat    string `json:"scenario_format,omitempty"`
	PersonalityFormat string `json:"personality_format,omitempty"`
	WrapInQuotes      *bool  `json:"wrap_in_quotes,omitempty"`
	NamesBehavior     *int   `json:"names_behavior,omitempty"`
	SendIfEmpty       string `json:"send_if_empty,omitempty"`

	// Multimodal / tool use.
	ImageInlining      *bool  `json:"image_inlining,omitempty"`
	InlineImageQuality string `json:"inline_image_quality,omitempty"`
	VideoInlining      *bool  `json:"video_inlining,omitempty"`
	RequestImages      *bool  `json:"request_images,omitempty"`
	FunctionCalling    *bool  `json:"function_calling,omitempty"`

	// Reasoning / thinking.
	ShowThoughts     *bool  `json:"show_thoughts,omitempty"`
	ReasoningEffort  string `json:"reasoning_effort,omitempty"`
	ReasoningEnabled *bool  `json:"reasoning_enabled,omitempty"`

	// Other.
	EnableWebSearch      *bool  `json:"enable_web_search,omitempty"`
	BiasPresetSelected   string `json:"bias_preset_selected,omitempty"`
	SystemPromptOverride string `json:"system_prompt,omitempty"`

	// Extensions (regex scripts + future plug-ins).
	Extensions ExtensionsBundle `json:"extensions,omitempty"`
}

// PromptBlock is one entry in the Prompt Manager list. Non-marker blocks
// carry their own Content; marker blocks (identifier = "main",
// "chatHistory", "charDescription", "worldInfoBefore", etc.) are
// positional placeholders resolved against the chat's runtime context
// (character / persona / lorebook) at prompt-assembly time.
type PromptBlock struct {
	Identifier        string `json:"identifier"`
	Name              string `json:"name,omitempty"`
	Role              string `json:"role,omitempty"` // "system" | "user" | "assistant"
	Content           string `json:"content,omitempty"`
	SystemPrompt      bool   `json:"system_prompt,omitempty"` // legacy "is this the main sysprompt" flag
	Marker            bool   `json:"marker,omitempty"`        // true for positional placeholders
	InjectionPosition *int   `json:"injection_position,omitempty"` // 0 = in system msg, 1 = relative chat depth
	InjectionDepth    *int   `json:"injection_depth,omitempty"`
	InjectionOrder    *int   `json:"injection_order,omitempty"`
	ForbidOverrides   bool   `json:"forbid_overrides,omitempty"`
}

// PromptOrderGroup holds a per-character prompt ordering. ST stores one
// entry per character_id, plus a "character_id: 100001" wildcard entry
// that applies to every chat (our primary use).
type PromptOrderGroup struct {
	CharacterID int                `json:"character_id"`
	Order       []PromptOrderEntry `json:"order,omitempty"`
}

type PromptOrderEntry struct {
	Identifier string `json:"identifier"`
	Enabled    bool   `json:"enabled"`
}

// ExtensionsBundle holds preset-level plug-in data. Currently: regex
// scripts. ST also stores misc per-extension config here; unknown keys
// are preserved via the top-level Data blob's round-trip.
type ExtensionsBundle struct {
	RegexScripts []RegexScript `json:"regex_scripts,omitempty"`
}

// RegexScript is a find/replace transform applied at one of the numbered
// `placement` stages. ST uses:
//
//	1 = user input (before send)
//	2 = assistant output (before display)
//	3 = slash commands (unused here)
//	4 = world info lookup (unused here)
//	5 = reasoning block (unused here)
//	6 = display text (post-render)
//
// Scripts with `disabled: true` are silently skipped. `markdownOnly` means
// apply only when the text has markdown (we apply unconditionally for
// now — matches ST's default when `promptOnly` is false).
type RegexScript struct {
	ID              string   `json:"id,omitempty"`
	ScriptName      string   `json:"scriptName,omitempty"`
	FindRegex       string   `json:"findRegex"`
	ReplaceString   string   `json:"replaceString"`
	TrimStrings     []string `json:"trimStrings,omitempty"`
	Placement       []int    `json:"placement,omitempty"`
	Disabled        bool     `json:"disabled,omitempty"`
	MarkdownOnly    bool     `json:"markdownOnly,omitempty"`
	PromptOnly      bool     `json:"promptOnly,omitempty"`
	RunOnEdit       bool     `json:"runOnEdit,omitempty"`
	SubstituteRegex int      `json:"substituteRegex,omitempty"`
	MinDepth        *int     `json:"minDepth,omitempty"`
	MaxDepth        *int     `json:"maxDepth,omitempty"`
}

// AsSampler parses Data as SamplerData. Returns a zero value if the
// payload doesn't decode cleanly (e.g. a non-sampler preset fed in by
// mistake) rather than erroring, so callers can fall through to defaults.
func (p Preset) AsSampler() SamplerData {
	var s SamplerData
	_ = json.Unmarshal(p.Data, &s)
	return s
}

// AsSysprompt parses Data as SyspromptData. Same zero-on-failure policy as
// AsSampler — callers fall through to defaults rather than propagating
// decode errors into prompt assembly.
func (p Preset) AsSysprompt() SyspromptData {
	var s SyspromptData
	_ = json.Unmarshal(p.Data, &s)
	return s
}

// AsOpenAIBundle parses Data as OpenAIBundleData — the full SillyTavern
// OpenAI-style preset including prompts[] + prompt_order[] (Prompt Manager),
// regex scripts, per-provider flags, and every sampler knob. Zero-value on
// decode failure, same policy as AsSampler.
//
// This is the "full" view — use it when the UI / prompt assembly needs to
// respect the preset's richer semantics (M32). Callers that only want the
// flat sampler numbers can keep calling AsSampler().
func (p Preset) AsOpenAIBundle() OpenAIBundleData {
	var b OpenAIBundleData
	_ = json.Unmarshal(p.Data, &b)
	return b
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
