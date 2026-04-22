import { apiFetch } from '@/api/client'

// ─── Types ────────────────────────────────────────────────────────────

export interface BYOKKey {
  id: string
  provider: string
  label?: string
  masked: string       // e.g. "sk-…6411"
  created_at: string
}

export interface BYOKCreateInput {
  provider: string
  label?: string
  key: string         // plaintext; server encrypts before storing
}

// ─── API ──────────────────────────────────────────────────────────────

export const byokApi = {
  list: () => apiFetch<{ items: BYOKKey[] }>('/api/byok'),

  providers: () => apiFetch<{ items: string[] }>('/api/byok/providers'),

  create: (input: BYOKCreateInput) =>
    apiFetch<BYOKKey>('/api/byok', {
      method: 'POST',
      body: JSON.stringify(input),
    }),

  delete: (id: string) =>
    apiFetch<void>(`/api/byok/${id}`, { method: 'DELETE' }),

  /** Pin a chat to a BYOK key (null clears) so the stream path uses
   *  that key for upstream auth instead of the user's WuApi key. */
  setForChat: (chatID: string, byokID: string | null) =>
    apiFetch<void>(`/api/chats/${chatID}/byok`, {
      method: 'PUT',
      body: JSON.stringify({ byok_id: byokID }),
    }),
}
