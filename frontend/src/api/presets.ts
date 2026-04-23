import { apiFetch } from '@/api/client'

// ─── Types ────────────────────────────────────────────────────────────

export type PresetType =
  | 'sampler'
  | 'openai'
  | 'instruct'
  | 'context'
  | 'sysprompt'
  | 'reasoning'

export const PRESET_TYPES: PresetType[] = [
  'sampler',
  'instruct',
  'context',
  'sysprompt',
  'reasoning',
  'openai',
]

/** Typed view of sampler preset data. Mirrors SillyTavern's OpenAI/textgen
 *  sampler fields so imports round-trip cleanly. */
export interface SamplerData {
  temperature?: number | null
  top_p?: number | null
  top_k?: number | null
  min_p?: number | null
  max_tokens?: number | null
  frequency_penalty?: number | null
  presence_penalty?: number | null
  repetition_penalty?: number | null
  seed?: number | null
  stop?: string[] | null
  reasoning_enabled?: boolean | null
  system_prompt?: string | null
}

/** Instruct template — wraps user/assistant/system turns for text-completion
 *  backends. Field names match ST's instruct-mode schema. */
export interface InstructData {
  input_sequence?: string          // e.g. "[INST] "
  output_sequence?: string         // e.g. "[/INST] "
  system_sequence?: string         // e.g. "<<SYS>>\n"
  first_input_sequence?: string
  last_input_sequence?: string
  first_output_sequence?: string
  last_output_sequence?: string
  stop_sequence?: string
  activation_regex?: string        // auto-activate by model name
  wrap?: boolean                   // wrap tokens at limit
  user_alignment_message?: string
}

/** Context template — how character/world/history get woven into the prompt.
 *  ST field names verbatim; story_string carries {{macros}}. */
export interface ContextData {
  story_string?: string
  example_separator?: string
  chat_start?: string
  use_stop_strings?: boolean
  names_as_stop_strings?: boolean
  single_line?: boolean
  trim_sentences?: boolean
  always_force_name2?: boolean
}

/** System prompt preset — just content. Tracked as its own type so users can
 *  swap "jailbreak prompts" without touching character system_prompt.
 *  `post_history` matches ST's sysprompt preset field (distinct from the V3
 *  character card's `post_history_instructions`). */
export interface SyspromptData {
  content?: string
  post_history?: string
}

/** Reasoning / thinking block config — prefix/suffix/separator for <think>
 *  tag rewriting. Used by o1, Claude thinking, DeepSeek-R1. */
export interface ReasoningData {
  prefix?: string
  suffix?: string
  separator?: string
}

// ─── Full OpenAI-style bundle (M32) ───────────────────────────────────
//
// Mirrors the Go-side presets.OpenAIBundleData. Represents the FULL
// SillyTavern OpenAI completion preset, including the Prompt Manager
// (prompts + prompt_order), extension regex scripts, per-provider flags,
// and every sampler knob in one payload. Stored in Preset.data as JSONB
// — this interface is the typed read path.

export interface PromptBlock {
  identifier: string
  name?: string
  role?: string                  // "system" | "user" | "assistant"
  content?: string
  system_prompt?: boolean         // ST legacy flag (is-this-the-main-sysprompt)
  marker?: boolean
  injection_position?: number     // 0 = into system msg, 1 = relative depth
  injection_depth?: number
  injection_order?: number
  forbid_overrides?: boolean
}

export interface PromptOrderEntry {
  identifier: string
  enabled: boolean
}

export interface PromptOrderGroup {
  character_id: number            // 100001 = default/wildcard in ST
  order: PromptOrderEntry[]
}

export interface RegexScript {
  id?: string
  scriptName?: string
  findRegex: string
  replaceString: string
  trimStrings?: string[]
  placement?: number[]            // 1=user input, 2=AI output, 3-6 unused
  disabled?: boolean
  markdownOnly?: boolean
  promptOnly?: boolean
  runOnEdit?: boolean
  substituteRegex?: number
  minDepth?: number | null
  maxDepth?: number | null
}

export interface ExtensionsBundle {
  regex_scripts?: RegexScript[]
}

/**
 * OpenAIBundleData — the full ST preset payload the user imported.
 *
 * Not all fields are surfaced in every UI tab; the Raw JSON tab always
 * lets power users edit anything directly. The server preserves unknown
 * fields on round-trip so custom ST extensions keep working even if we
 * don't surface their controls.
 */
export interface OpenAIBundleData extends SamplerData {
  // Extra ST sampler variants
  top_a?: number | null
  openai_max_tokens?: number | null
  openai_max_context?: number | null
  max_context_unlocked?: boolean | null
  n?: number | null
  stream_openai?: boolean | null

  // Prompt Manager
  prompts?: PromptBlock[]
  prompt_order?: PromptOrderGroup[]

  // Per-provider behavior / prefills
  assistant_prefill?: string
  assistant_impersonation?: string
  claude_use_sysprompt?: boolean | null
  use_makersuite_sysprompt?: boolean | null
  squash_system_messages?: boolean | null

  // Continue / impersonation / nudge
  new_chat_prompt?: string
  new_group_chat_prompt?: string
  new_example_chat_prompt?: string
  continue_nudge_prompt?: string
  continue_prefill?: boolean | null
  continue_postfix?: string
  impersonation_prompt?: string
  group_nudge_prompt?: string

  // Format
  wi_format?: string
  scenario_format?: string
  personality_format?: string
  wrap_in_quotes?: boolean | null
  names_behavior?: number | null
  send_if_empty?: string

  // Multimodal / tool use
  image_inlining?: boolean | null
  inline_image_quality?: string
  video_inlining?: boolean | null
  request_images?: boolean | null
  function_calling?: boolean | null

  // Reasoning / thinking
  show_thoughts?: boolean | null
  reasoning_effort?: string
  // reasoning_enabled already on SamplerData

  // Other
  enable_web_search?: boolean | null
  bias_preset_selected?: string
  // system_prompt already on SamplerData

  // Extensions
  extensions?: ExtensionsBundle
}

/**
 * Preset — generic wrapper. `data` is deliberately `unknown` because each
 * type (sampler, instruct, context, reasoning, …) carries its own schema.
 * Consumers narrow based on `type` then cast to the relevant Data shape.
 */
export interface Preset {
  id: string
  type: PresetType
  name: string
  data: unknown
  /** True if this preset is the user's current default for its type. */
  is_default?: boolean
  created_at: string
  updated_at: string
}

// ─── API methods ─────────────────────────────────────────────────────

export const presetsApi = {
  list: (type?: PresetType) => {
    const q = type ? `?type=${encodeURIComponent(type)}` : ''
    return apiFetch<{ items: Preset[] }>(`/api/presets${q}`)
  },

  get: (id: string) => apiFetch<Preset>(`/api/presets/${id}`),

  /**
   * Create a new preset. `data` is passed verbatim; the server stores it
   * as JSONB and does not validate the shape — types (e.g. Instruct) carry
   * provider-specific fields we don't want to forbid by mistake.
   */
  create: (input: { type: PresetType; name: string; data: unknown }) =>
    apiFetch<Preset>('/api/presets', {
      method: 'POST',
      body: JSON.stringify(input),
    }),

  update: (id: string, patch: { name?: string; data?: unknown }) =>
    apiFetch<Preset>(`/api/presets/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(patch),
    }),

  delete: (id: string) =>
    apiFetch<void>(`/api/presets/${id}`, { method: 'DELETE' }),
}

// ─── Defaults API (GET/PUT /api/me/defaults) ─────────────────────────

export interface DefaultsResponse {
  default_presets: Record<string, string>
}

export const defaultsApi = {
  list: () => apiFetch<DefaultsResponse>('/api/me/defaults'),

  /**
   * Set (or clear) the default preset for a given type.
   * Pass presetID = null to clear.
   */
  set: (type: PresetType, presetID: string | null) =>
    apiFetch<void>('/api/me/defaults', {
      method: 'PUT',
      body: JSON.stringify({ type, preset_id: presetID }),
    }),
}
