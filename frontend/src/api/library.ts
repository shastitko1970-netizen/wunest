import { apiFetch } from '@/api/client'
import type { Character } from '@/api/characters'

// ─── Types (mirror internal/library/chub.go) ──────────────────────────

export interface LibraryResult {
  full_path: string
  name: string
  tagline?: string
  description?: string
  avatar_url?: string
  max_res_url?: string
  tags?: string[]
  star_count: number
  rating_count: number
  rating: number
  nsfw: boolean
  creator?: string
  created_at?: string
  last_activity?: string
}

export interface LibrarySearchResponse {
  items: LibraryResult[]
  count: number
}

export type SortOption =
  | 'trending_downloads'
  | 'last_activity_at'
  | 'created_at'
  | 'star_count'

export interface SearchParams {
  q?: string
  page?: number
  per_page?: number
  sort?: SortOption
  nsfw?: boolean
  tags?: string[]
  exclude_tags?: string[]
}

// ─── API methods ─────────────────────────────────────────────────────

export const libraryApi = {
  searchChub: (params: SearchParams = {}) => {
    const qs = new URLSearchParams()
    if (params.q) qs.set('q', params.q)
    if (params.page != null) qs.set('page', String(params.page))
    if (params.per_page != null) qs.set('per_page', String(params.per_page))
    if (params.sort) qs.set('sort', params.sort)
    qs.set('nsfw', params.nsfw ? 'true' : 'false')
    if (params.tags?.length) qs.set('tags', params.tags.join(','))
    if (params.exclude_tags?.length) qs.set('exclude_tags', params.exclude_tags.join(','))

    return apiFetch<LibrarySearchResponse>(`/api/library/chub/search?${qs.toString()}`)
  },

  /** Import one CHUB character. Server fetches the PNG and persists a new
   *  nest_characters row, returning the full Character for UI insertion. */
  importChub: (fullPath: string) =>
    apiFetch<Character>('/api/library/chub/import', {
      method: 'POST',
      body: JSON.stringify({ full_path: fullPath }),
    }),
}
