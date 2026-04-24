import { ApiError, apiFetch } from '@/api/client'

// Converter API — posts a SillyTavern theme `.json` to the backend,
// which runs an LLM conversion (user's BYOK or WuApi pool → the LLM)
// and returns a WuNest-compatible theme JSON. Result is stored 24h
// server-side so the user can re-download within that window.

export interface ConvertJob {
  id: string
  user_id: string
  status: 'pending' | 'running' | 'done' | 'error'
  model: string
  byok_id?: string | null
  input_sha256: string
  input_size: number
  error_message?: string
  tokens_in: number
  tokens_out: number
  created_at: string
  expires_at: string
  finished_at?: string | null
}

// Payload returned by POST /api/convert/theme. `output` is the
// LLM-produced WuNest theme (same shape as ST theme JSON). Frontend
// can either download it as a file OR hand it straight to the
// appearance import flow.
export interface ConvertResponse {
  job: ConvertJob
  output: unknown // WuNest theme JSON — structurally a superset of STTheme
  output_url: string
  download_url: string
}

export interface ConvertParams {
  file: File
  model: string
  byokId?: string    // empty/undefined → use WuApi pool
  signal?: AbortSignal
}

/**
 * convertTheme — fire-and-wait: the POST returns when the LLM has
 * finished (or errored). No streaming to the UI because the output
 * is a single JSON blob, not incremental text.
 *
 * On 429 (rate limit) we throw an ApiError with `status === 429` and
 * a structured `resets_at` attached via the error message — view layer
 * surfaces a friendly "3 conversions/hour used, try again at …".
 */
export async function convertTheme(p: ConvertParams): Promise<ConvertResponse> {
  const fd = new FormData()
  fd.append('file', p.file, p.file.name)
  fd.append('model', p.model)
  if (p.byokId) fd.append('byok_id', p.byokId)

  const res = await fetch('/api/convert/theme', {
    method: 'POST',
    credentials: 'include',
    body: fd,
    signal: p.signal,
  })

  if (!res.ok) {
    // 429 carries a JSON body with resets_at hint — preserve the whole
    // blob in the message so the UI can render a countdown if desired.
    let message = res.statusText
    try {
      const body = await res.json()
      if (body?.error === 'rate_limited' && body?.resets_at) {
        message = `rate_limited|${body.resets_at}`
      } else if (typeof body === 'string') {
        message = body
      } else if (body?.message) {
        message = body.message
      }
    } catch {
      // Not JSON — grab plain text for context.
      try { message = (await res.text()) || res.statusText } catch { /* noop */ }
    }
    throw new ApiError(res.status, message)
  }

  return (await res.json()) as ConvertResponse
}

/** List user's recent conversion jobs (server filters to last 24h). */
export function listJobs(): Promise<{ items: ConvertJob[] }> {
  return apiFetch<{ items: ConvertJob[] }>('/api/convert/jobs')
}

/** Fetch one job's metadata + output. Owner-gated on the backend. */
export function getJob(id: string): Promise<{ job: ConvertJob; output: unknown }> {
  return apiFetch<{ job: ConvertJob; output: unknown }>(`/api/convert/${id}`)
}

/** Direct download URL — triggers a browser file download dialog. */
export function downloadUrl(id: string): string {
  return `/api/convert/${id}/download`
}
