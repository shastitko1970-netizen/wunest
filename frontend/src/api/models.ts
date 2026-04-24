import { apiFetch } from '@/api/client'

// Shape mirrors OpenAI's /v1/models: a list of whatever the active provider
// decides to expose. For WuApi that's wu-tier aliases + any gold models the
// user's balance permits; for BYOK it's literally whatever the provider's
// `/models` endpoint returns (OpenRouter has hundreds, Anthropic has ~6).
export interface Model {
  id: string
  object?: string
  owned_by?: string
  created?: number
}

export interface ModelListResponse {
  object: 'list'
  data: Model[]
}

export const modelsApi = {
  // WuApi pool — proxied through our backend.
  list: () => apiFetch<ModelListResponse>('/api/models'),

  // Live-fetch the catalogue of models available through a stored BYOK key.
  // Backend caches the result in Redis for 10 min; pass `refresh=true` to
  // bypass the cache (user clicked "refresh" in the picker).
  listForBYOK: (byokID: string, refresh = false) =>
    apiFetch<ModelListResponse>(
      `/api/byok/${byokID}/models${refresh ? '?refresh=1' : ''}`,
    ),
}
