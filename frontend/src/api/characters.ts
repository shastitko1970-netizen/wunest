import { apiFetch } from '@/api/client'

// ─── Types (kept in sync with Go's internal/characters/types.go) ──────

export interface CharacterData {
  name: string
  description?: string
  personality?: string
  scenario?: string
  first_mes?: string
  mes_example?: string
  creator_notes?: string
  system_prompt?: string
  post_history_instructions?: string
  alternate_greetings?: string[]
  tags?: string[]
  creator?: string
  character_version?: string
  character_book?: CharacterBook
  extensions?: Record<string, unknown>
  // V3-specific
  nickname?: string
  creator_notes_multilingual?: Record<string, string>
  source?: string[]
  group_only_greetings?: string[]
}

export interface CharacterBook {
  name?: string
  description?: string
  scan_depth?: number | null
  token_budget?: number | null
  recursive_scanning?: boolean | null
  entries?: CharacterBookEntry[]
  extensions?: Record<string, unknown>
}

/**
 * CharacterBookEntry — mirrors SillyTavern V3 spec. Every field is
 * surfaced in our editor; unknown ST fields round-trip via `extensions`.
 *
 * `position` is ST's string enum: "before_char" = splice into system
 * prompt before character description, "after_char" = after. Some older
 * cards use numeric positions; we normalize on import.
 */
export interface CharacterBookEntry {
  keys: string[]
  content: string
  enabled: boolean
  insertion_order: number
  case_sensitive?: boolean | null
  priority?: number
  id?: number
  comment?: string
  selective?: boolean
  secondary_keys?: string[]
  constant?: boolean
  position?: string              // "before_char" | "after_char"
  name?: string
  extensions?: Record<string, unknown>
}

export interface Character {
  id: string
  name: string
  data: CharacterData
  /** Small thumbnail (≤400 px). Set by import for PNG cards, empty for JSON. */
  avatar_url?: string
  /** Full-size PNG from the uploaded card. Used by detail views. */
  avatar_original_url?: string
  tags: string[]
  favorite: boolean
  spec: string
  source_url?: string
  created_at: string
  updated_at: string
}

// ─── API methods ──────────────────────────────────────────────────────

export const charactersApi = {
  list: () => apiFetch<{ items: Character[] }>('/api/characters'),

  get: (id: string) => apiFetch<Character>(`/api/characters/${id}`),

  create: (input: Partial<Character>) =>
    apiFetch<Character>('/api/characters', {
      method: 'POST',
      body: JSON.stringify(input),
    }),

  update: (id: string, patch: Partial<Character>) =>
    apiFetch<Character>(`/api/characters/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(patch),
    }),

  delete: (id: string) =>
    apiFetch<void>(`/api/characters/${id}`, { method: 'DELETE' }),

  // Multipart upload — bypasses the default JSON content-type in apiFetch.
  // Accepts PNG (embedded V2/V3 metadata) or JSON (bare ST export); the
  // server sniffs by magic bytes and dispatches. Legacy name `importPNG`
  // kept for call-site compat; prefer `importCard` for new code.
  importCard: async (file: File, sourceURL?: string): Promise<Character> => {
    const fd = new FormData()
    fd.append('file', file)
    if (sourceURL) fd.append('source_url', sourceURL)

    const res = await fetch('/api/characters/import', {
      method: 'POST',
      credentials: 'include',
      body: fd,
    })
    if (!res.ok) {
      const body = await res.text().catch(() => '')
      throw new Error(body || `Import failed (${res.status})`)
    }
    return res.json() as Promise<Character>
  },

  /** @deprecated use `importCard` — backend now sniffs PNG vs JSON. */
  importPNG: async (file: File, sourceURL?: string): Promise<Character> => {
    return charactersApi.importCard(file, sourceURL)
  },
}
