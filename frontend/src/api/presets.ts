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
