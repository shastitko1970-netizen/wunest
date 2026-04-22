import { apiFetch } from '@/api/client'

// ─── Types ────────────────────────────────────────────────────────────

export interface Persona {
  id: string
  name: string
  description: string
  avatar_url?: string
  is_default: boolean
  created_at: string
}

export interface PersonaCreateInput {
  name: string
  description?: string
  avatar_url?: string
  is_default?: boolean
}

export interface PersonaUpdatePatch {
  name?: string
  description?: string
  avatar_url?: string
}

// ─── API ──────────────────────────────────────────────────────────────

export const personasApi = {
  list: () => apiFetch<{ items: Persona[] }>('/api/personas'),

  get: (id: string) => apiFetch<Persona>(`/api/personas/${id}`),

  create: (input: PersonaCreateInput) =>
    apiFetch<Persona>('/api/personas', {
      method: 'POST',
      body: JSON.stringify(input),
    }),

  update: (id: string, patch: PersonaUpdatePatch) =>
    apiFetch<Persona>(`/api/personas/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(patch),
    }),

  /** Make this persona the user's default. Demotes any prior default. */
  setDefault: (id: string) =>
    apiFetch<void>(`/api/personas/${id}/default`, { method: 'PUT' }),

  /** Clear the account-wide default entirely. */
  clearDefault: () =>
    apiFetch<void>(`/api/personas/_/default`, { method: 'DELETE' }),

  delete: (id: string) =>
    apiFetch<void>(`/api/personas/${id}`, { method: 'DELETE' }),

  /**
   * Per-chat persona override. Pass personaID = null to clear the override
   * (falls back to the user's default or session name).
   */
  setForChat: (chatID: string, personaID: string | null) =>
    apiFetch<void>(`/api/chats/${chatID}/persona`, {
      method: 'PUT',
      body: JSON.stringify({ persona_id: personaID }),
    }),
}
