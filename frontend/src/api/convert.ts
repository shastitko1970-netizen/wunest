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
  // M51 Sprint 2 wave 2 — instead of `file: File` we accept a Blob +
  // filename pair. This lets paste-text and CSS-wrap callers synthesise
  // a Blob without faking a File object. A real File extends Blob, so
  // `file` upload still works: pass `{ blob: file, filename: file.name }`.
  blob: Blob
  filename: string
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
  fd.append('file', p.blob, p.filename)
  fd.append('model', p.model)
  if (p.byokId) fd.append('byok_id', p.byokId)

  const res = await fetch('/api/convert/theme', {
    method: 'POST',
    credentials: 'include',
    body: fd,
    signal: p.signal,
  })

  return parseConvertResponse(res)
}

/**
 * retryJob — re-runs an existing job's input through a different
 * model/source (M51 Sprint 2 wave 2). The server reads the original
 * bytes from `nest_converter_jobs.input_data` so the user doesn't
 * need to re-upload.
 *
 * On 410 the source job either expired or predates the retry feature
 * (input bytes weren't persisted). Caller should fall back to "upload
 * the file again" UX.
 */
export async function retryJob(
  id: string,
  model: string,
  byokId?: string,
): Promise<ConvertResponse> {
  const res = await fetch(`/api/convert/${id}/retry`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ model, byok_id: byokId || null }),
  })
  return parseConvertResponse(res)
}

// Shared response parser — used by both convertTheme and retryJob.
// Handles the 429 rate-limit body shape uniformly so the SPA's error
// surface gets `rate_limited|<ISO-8601>` for either entry point.
async function parseConvertResponse(res: Response): Promise<ConvertResponse> {
  if (!res.ok) {
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
