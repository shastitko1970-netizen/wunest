import { apiFetch } from '@/api/client'

// ─── Types (mirror internal/presets/types.go) ─────────────────────────

export type PresetType =
  | 'sampler'
  | 'openai'
  | 'instruct'
  | 'context'
  | 'sysprompt'
  | 'reasoning'

/**
 * SamplerData uses null for "unset" instead of the Go side's pointer-nil.
 * Omitting a field from the payload makes JSON.stringify drop it entirely,
 * so the server sees `{"data":{"temperature":0.9}}` with no other fields —
 * correctly interpreted as "leave the other knobs at defaults".
 */
export interface SamplerData {
  temperature?: number | null
  top_p?: number | null
  max_tokens?: number | null
  frequency_penalty?: number | null
  presence_penalty?: number | null
  /** Replaces the character-derived system message when non-empty. */
  system_prompt?: string | null
}

export interface Preset {
  id: string
  type: PresetType
  name: string
  data: SamplerData
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

  create: (input: { type?: PresetType; name: string; data: SamplerData }) =>
    apiFetch<Preset>('/api/presets', {
      method: 'POST',
      body: JSON.stringify({ type: input.type ?? 'sampler', ...input }),
    }),

  update: (id: string, patch: { name?: string; data?: SamplerData }) =>
    apiFetch<Preset>(`/api/presets/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(patch),
    }),

  delete: (id: string) =>
    apiFetch<void>(`/api/presets/${id}`, { method: 'DELETE' }),
}
