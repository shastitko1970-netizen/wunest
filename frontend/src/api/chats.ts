import { apiFetch } from '@/api/client'

// ─── Types (kept in sync with Go's internal/chats/types.go) ───────────

export type Role = 'user' | 'assistant' | 'system'

export interface Chat {
  id: string
  user_id: string
  character_id?: string | null
  character_name?: string
  name: string
  chat_metadata?: {
    sampler?: ChatSamplerMetadata
    persona_id?: string | null
    byok_id?: string | null
    authors_note?: AuthorsNote | null
    [key: string]: unknown
  }
  created_at: string
  updated_at: string
  last_message_at?: string
}

/** Report returned by POST /api/chats/import. */
export interface ImportReport {
  chat: Chat
  imported: number
  skipped: number
  skipped_details: { line: number; reason: string }[]
  skipped_overflow: number
  total_data_lines: number
}

/** Author's Note — prose block injected at `depth` from history's end.
 *  Mirrors SillyTavern's semantics; `role` defaults to "system". */
export interface AuthorsNote {
  content: string
  depth: number
  role?: 'system' | 'user' | 'assistant'
}

/** Mirror of Go's internal/chats/types.go ChatSamplerMetadata. */
export interface ChatSamplerMetadata {
  temperature?: number | null
  top_p?: number | null
  top_k?: number | null
  min_p?: number | null
  max_tokens?: number | null
  frequency_penalty?: number | null
  presence_penalty?: number | null
  repetition_penalty?: number | null
  seed?: number | null
  stop?: string[] | null
  reasoning_enabled?: boolean | null
  system_prompt?: string | null
  preset_id?: string | null
}

export interface Message {
  id: number
  chat_id: string
  role: Role
  content: string
  swipes?: string[]
  swipe_id: number
  extras?: MessageExtras
  hidden?: boolean
  created_at: string
}

export interface MessageExtras {
  model?: string
  reasoning?: string
  tokens_in?: number
  tokens_out?: number
  latency_ms?: number
  finish_reason?: string
  error?: string
}

// ─── HTTP API methods ─────────────────────────────────────────────────

export const chatsApi = {
  list: () => apiFetch<{ items: Chat[] }>('/api/chats'),

  get: (id: string) => apiFetch<Chat>(`/api/chats/${id}`),

  create: (input: { character_id?: string; name?: string }) =>
    apiFetch<Chat>('/api/chats', {
      method: 'POST',
      body: JSON.stringify(input),
    }),

  rename: (id: string, name: string) =>
    apiFetch<void>(`/api/chats/${id}`, {
      method: 'PATCH',
      body: JSON.stringify({ name }),
    }),

  setSampler: (id: string, sampler: ChatSamplerMetadata) =>
    apiFetch<void>(`/api/chats/${id}/sampler`, {
      method: 'PUT',
      body: JSON.stringify(sampler),
    }),

  /** Set or clear the chat's Author's Note. Pass null to clear. */
  setAuthorsNote: (id: string, note: AuthorsNote | null) =>
    apiFetch<void>(`/api/chats/${id}/authors-note`, {
      method: 'PUT',
      body: JSON.stringify(note),
    }),

  /** Download the chat as JSONL. Returns a Blob + suggested filename. */
  async exportJsonl(id: string): Promise<{ blob: Blob; filename: string }> {
    const res = await fetch(`/api/chats/${id}/export`, { credentials: 'include' })
    if (!res.ok) throw new Error(`Export failed (${res.status})`)
    // Content-Disposition: attachment; filename="Chat.jsonl"
    const disp = res.headers.get('Content-Disposition') ?? ''
    const match = /filename="?([^";]+)"?/i.exec(disp)
    const filename = match?.[1] ?? `chat-${id}.jsonl`
    const blob = await res.blob()
    return { blob, filename }
  },

  /** Upload a JSONL file, creating a new chat. Returns the full report —
   *  imported/skipped counts plus details for the first N skipped lines. */
  async importJsonl(file: File): Promise<ImportReport> {
    const fd = new FormData()
    fd.append('file', file)
    const res = await fetch('/api/chats/import', {
      method: 'POST',
      credentials: 'include',
      body: fd,
    })
    if (!res.ok) {
      const body = await res.text().catch(() => '')
      throw new Error(body || `Import failed (${res.status})`)
    }
    return res.json()
  },

  delete: (id: string) =>
    apiFetch<void>(`/api/chats/${id}`, { method: 'DELETE' }),

  listMessages: (chatID: string) =>
    apiFetch<{ items: Message[] }>(`/api/chats/${chatID}/messages`),

  editMessage: (chatID: string, messageID: number, content: string) =>
    apiFetch<void>(`/api/chats/${chatID}/messages/${messageID}`, {
      method: 'PATCH',
      body: JSON.stringify({ content }),
    }),

  deleteMessage: (chatID: string, messageID: number) =>
    apiFetch<void>(`/api/chats/${chatID}/messages/${messageID}`, {
      method: 'DELETE',
    }),

  /** Navigate between stored swipes. Returns the updated message. */
  selectSwipe: (chatID: string, messageID: number, swipeID: number) =>
    apiFetch<Message>(`/api/chats/${chatID}/messages/${messageID}/swipe`, {
      method: 'PATCH',
      body: JSON.stringify({ swipe_id: swipeID }),
    }),
}

// ─── Streaming send ───────────────────────────────────────────────────

/** One decoded SSE event emitted by POST /api/chats/:id/messages. */
export type StreamEvent =
  | { event: 'user_message'; data: Message }
  | { event: 'assistant_start'; data: { id: number; model: string } }
  | { event: 'swipe_start'; data: { id: number; swipe_id: number } }
  | { event: 'token'; data: { content: string } }
  | { event: 'done'; data: {
        id: number
        content: string
        reasoning?: string
        tokens_in: number
        tokens_out: number
        latency_ms: number
        finish_reason?: string
    } }
  | { event: 'error'; data: { kind: string; message: string } }
  | { event: 'raw'; data: unknown }

export interface SendMessageInput {
  content: string
  model?: string
  temperature?: number
  max_tokens?: number
}

/**
 * sendMessage sends a user message and streams the assistant response.
 *
 * Yields StreamEvent objects as they arrive. Throws on HTTP errors *before*
 * the stream starts; in-stream errors come through as `{event: 'error'}`.
 *
 * Cancellation: pass an AbortSignal; calling .abort() cuts the upstream too.
 */
export async function* sendMessageStream(
  chatID: string,
  input: SendMessageInput,
  signal?: AbortSignal,
): AsyncGenerator<StreamEvent, void, unknown> {
  yield* streamSSE(`/api/chats/${chatID}/messages`, input, signal)
}

/** Regenerate — drops the last assistant message, re-streams a reply. */
export async function* regenerateStream(
  chatID: string,
  input: Partial<SendMessageInput> = {},
  signal?: AbortSignal,
): AsyncGenerator<StreamEvent, void, unknown> {
  yield* streamSSE(`/api/chats/${chatID}/regenerate`, input, signal)
}

/** Swipe — keep the existing assistant message, append a new variant,
 *  navigate to it. Streams via the same SSE protocol as send/regenerate,
 *  with a `swipe_start` event replacing `assistant_start`. */
export async function* swipeMessageStream(
  chatID: string,
  messageID: number,
  input: Partial<SendMessageInput> = {},
  signal?: AbortSignal,
): AsyncGenerator<StreamEvent, void, unknown> {
  yield* streamSSE(`/api/chats/${chatID}/messages/${messageID}/swipe`, input, signal)
}

async function* streamSSE(
  url: string,
  body: unknown,
  signal?: AbortSignal,
): AsyncGenerator<StreamEvent, void, unknown> {
  const res = await fetch(url, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', 'Accept': 'text/event-stream' },
    body: JSON.stringify(body),
    signal,
  })

  if (!res.ok) {
    const body = await res.text().catch(() => '')
    throw new Error(body || `Send failed (${res.status})`)
  }
  if (!res.body) {
    throw new Error('Streaming not supported by this browser')
  }

  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  // Parser state for a multi-line SSE block.
  let currentEvent = ''
  let currentData = ''

  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })

      // Process all complete lines. Last (possibly partial) line stays in buffer.
      let nl: number
      while ((nl = buffer.indexOf('\n')) >= 0) {
        const line = buffer.slice(0, nl)
        buffer = buffer.slice(nl + 1)

        // Blank line = event terminator.
        if (line.trim() === '') {
          if (currentEvent && currentData) {
            yield decodeEvent(currentEvent, currentData)
          }
          currentEvent = ''
          currentData = ''
          continue
        }
        if (line.startsWith('event: ')) {
          currentEvent = line.slice(7).trim()
        } else if (line.startsWith('data: ')) {
          // A data: line; multi-line data is concatenated with '\n'.
          currentData = currentData ? currentData + '\n' + line.slice(6) : line.slice(6)
        }
        // Ignore other SSE fields (id:, retry:) for now.
      }
    }
    // Flush trailing event without terminating blank line.
    if (currentEvent && currentData) {
      yield decodeEvent(currentEvent, currentData)
    }
  } finally {
    try { await reader.cancel() } catch { /* already closed */ }
  }
}

function decodeEvent(event: string, data: string): StreamEvent {
  let parsed: unknown
  try {
    parsed = JSON.parse(data)
  } catch {
    return { event: 'raw', data }
  }
  // We trust the server about event types; treat parsed as any-typed payload.
  return { event: event as StreamEvent['event'], data: parsed } as StreamEvent
}
