import { apiFetch } from '@/api/client'

// ─── Types ────────────────────────────────────────────────────────────

export type Position =
  | 'before_char'
  | 'after_char'
  | 'at_depth'
  | 'before_an'
  | 'after_an'
  | ''

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

  // Recursion controls (see internal/worldinfo/types.go).
  exclude_recursion?: boolean
  prevent_recursion?: boolean

  // ─── ST v1.12+ flexibility fields ───
  /** 0 (unset) = 100%; 1..99 = random roll; 100 = always. */
  probability?: number
  /** match_whole_words uses word-boundary matching instead of substring. */
  match_whole_words?: boolean | null
  /** Mutually-exclusive activation group. Empty = no group. */
  group?: string
  /** Bypass the group cap — useful for "always include" group members. */
  group_override?: boolean
  /** For at_depth entries: role of the injected message. */
  role?: 'system' | 'user' | 'assistant' | ''

  // Stateful activation — stored for ST round-trip fidelity, not yet
  // enforced in the activator.
  sticky?: number
  cooldown?: number
  delay?: number
  automation_id?: string
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
