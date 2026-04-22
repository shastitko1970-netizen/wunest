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

/** Typed view of sampler preset data. Other types store arbitrary shapes. */
export interface SamplerData {
  temperature?: number | null
  top_p?: number | null
  max_tokens?: number | null
  frequency_penalty?: number | null
  presence_penalty?: number | null
  system_prompt?: string | null
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
