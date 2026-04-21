import { apiFetch } from '@/api/client'

// Shape mirrors WuApi's /v1/models passthrough: an OpenAI-compat list of
// whatever the current user's tier can reach (wu-tier aliases + any gold
// models their balance permits).
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
  list: () => apiFetch<ModelListResponse>('/api/models'),
}
