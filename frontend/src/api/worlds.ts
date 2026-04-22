import { apiFetch } from '@/api/client'

// ─── Types ────────────────────────────────────────────────────────────

export type Position = 'before_char' | 'after_char' | ''

export interface WorldEntry {
  id?: number
  name?: string
  comment?: string
  keys: string[]
  secondary_keys?: string[]
  content: string
  enabled: boolean
  selective?: boolean
  constant?: boolean
  insertion_order?: number
  priority?: number
  position?: Position
  case_sensitive?: boolean | null
  depth?: number
}

export interface World {
  id: string
  name: string
  description: string
  entries: WorldEntry[]
  created_at: string
  updated_at: string
}

export interface WorldSummary {
  id: string
  name: string
  description: string
  entry_count: number
  updated_at: string
}

// ─── API ──────────────────────────────────────────────────────────────

export const worldsApi = {
  list: () => apiFetch<{ items: WorldSummary[] }>('/api/worlds'),

  get: (id: string) => apiFetch<World>(`/api/worlds/${id}`),

  create: (input: { name: string; description?: string; entries?: WorldEntry[] }) =>
    apiFetch<World>('/api/worlds', {
      method: 'POST',
      body: JSON.stringify(input),
    }),

  update: (id: string, patch: { name?: string; description?: string; entries?: WorldEntry[] }) =>
    apiFetch<World>(`/api/worlds/${id}`, {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),

  delete: (id: string) =>
    apiFetch<void>(`/api/worlds/${id}`, { method: 'DELETE' }),

  /**
   * Import a SillyTavern lorebook .json. The server normalises both the
   * newer array-shaped entries and the legacy `{ "0": {...} }` object shape.
   */
  importST: (payload: { name: string; description?: string; entries: unknown }) =>
    apiFetch<World>('/api/worlds/import', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),

  // Per-character attachment.
  listForCharacter: (characterID: string) =>
    apiFetch<{ world_ids: string[] }>(`/api/characters/${characterID}/worlds`),

  attach: (characterID: string, worldID: string) =>
    apiFetch<void>(`/api/characters/${characterID}/worlds/${worldID}`, {
      method: 'PUT',
    }),

  detach: (characterID: string, worldID: string) =>
    apiFetch<void>(`/api/characters/${characterID}/worlds/${worldID}`, {
      method: 'DELETE',
    }),
}
