import { apiFetch } from '@/api/client'

// ─── Types ────────────────────────────────────────────────────────────

export interface BYOKKey {
  id: string
  provider: string
  label?: string
  masked: string       // e.g. "sk-…6411"
  base_url: string     // where this key routes to (OpenAI-compat /v1 root)
  created_at: string
}

export interface BYOKCreateInput {
  provider: string
  label?: string
  key: string          // plaintext; server encrypts before storing
  base_url?: string    // required for "custom"; pre-fills from defaults for known providers
}

/** Metadata per provider — populates the BYOK form's base-URL default.
 *  Served by `/api/byok/providers` so the UI doesn't hardcode URLs. */
export interface BYOKProviderInfo {
  id: string
  default_url?: string   // empty for "custom" (user must provide)
}

// ─── API ──────────────────────────────────────────────────────────────

export const byokApi = {
  list: () => apiFetch<{ items: BYOKKey[] }>('/api/byok'),

  providers: () => apiFetch<{ items: BYOKProviderInfo[] }>('/api/byok/providers'),

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
