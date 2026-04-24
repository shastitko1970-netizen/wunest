// Quick replies — short template snippets shown as chips above the
// composer. User-scoped (one flat list per account).

import { apiFetch } from '@/api/client'

export interface QuickReply {
  id: string
  user_id: string
  label: string
  text: string
  position: number
  send_now: boolean
  created_at: string
  updated_at: string
}

export const quickRepliesApi = {
  list: () => apiFetch<{ items: QuickReply[] }>('/api/quick-replies'),

  create: (input: { label?: string; text: string; send_now?: boolean }) =>
    apiFetch<QuickReply>('/api/quick-replies', {
      method: 'POST',
      body: JSON.stringify(input),
    }),

  update: (id: string, patch: Partial<Pick<QuickReply, 'label' | 'text' | 'position' | 'send_now'>>) =>
    apiFetch<QuickReply>(`/api/quick-replies/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(patch),
    }),

  delete: (id: string) =>
    apiFetch<void>(`/api/quick-replies/${id}`, { method: 'DELETE' }),
}
