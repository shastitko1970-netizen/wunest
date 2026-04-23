// Uploads API — multipart file POST to /api/uploads/{avatar,attachment,background}.
//
// Server returns a public URL (served by nginx `/images/*` → MinIO). The
// URL is the only thing the rest of the app needs; the caller decides
// what to do with it (save into a form, insert into a message, etc.).
//
// Separate from `apiFetch` because multipart uploads can't share the
// JSON-content-type default. Error shape matches the server's structured
// error envelope: `{ "error": { "type": "...", "message": "..." } }`.

import { ApiError } from './client'

export interface AvatarUploadResult {
  avatar_url: string           // thumbnail — use as card avatar
  avatar_original_url: string  // full-size — detail views
}

export interface AttachmentUploadResult {
  url: string
  content_type: string
  size: number
}

export type BackgroundUploadResult = AttachmentUploadResult

/** Upload an avatar (PNG / JPEG). Returns thumbnail + original URLs. */
export function uploadAvatar(file: File, signal?: AbortSignal): Promise<AvatarUploadResult> {
  return postFile<AvatarUploadResult>('/api/uploads/avatar', file, signal)
}

/** Upload an arbitrary image attachment (chat messages). */
export function uploadAttachment(file: File, signal?: AbortSignal): Promise<AttachmentUploadResult> {
  return postFile<AttachmentUploadResult>('/api/uploads/attachment', file, signal)
}

/** Upload a chat background image. */
export function uploadBackground(file: File, signal?: AbortSignal): Promise<BackgroundUploadResult> {
  return postFile<BackgroundUploadResult>('/api/uploads/background', file, signal)
}

// ─── internals ────────────────────────────────────────────────────────

async function postFile<T>(path: string, file: File, signal?: AbortSignal): Promise<T> {
  const fd = new FormData()
  fd.append('file', file, file.name)

  const res = await fetch(path, {
    method: 'POST',
    credentials: 'include',
    body: fd,
    signal,
  })

  if (!res.ok) {
    // Parse structured error shape when available; fall back to raw body.
    let message = res.statusText
    try {
      const body = await res.json()
      if (body?.error?.message) message = body.error.message
      else if (body?.error?.type) message = body.error.type
    } catch { /* leave fallback */ }
    throw new ApiError(res.status, message)
  }
  return (await res.json()) as T
}
