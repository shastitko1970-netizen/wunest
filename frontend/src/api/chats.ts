import { apiFetch } from '@/api/client'

// ─── Types (kept in sync with Go's internal/chats/types.go) ───────────

export type Role = 'user' | 'assistant' | 'system'

export interface Chat {
  id: string
  user_id: string
  /** Primary character. For single-char chats this is THE character;
   *  for group chats this mirrors `character_ids[0]` for legacy filters
   *  (sidebar avatar, "find chat by character" flows). */
  character_id?: string | null
  character_name?: string
  /** All participants. 1-element for single chats, 2+ for groups.
   *  Source of truth for who's IN the chat. */
  character_ids?: string[]
  /** Derived: true iff character_ids.length > 1. */
  is_group_chat?: boolean
  name: string
  /** Free-form user-authored tags for organising chats (M38.1). */
  tags?: string[]
  chat_metadata?: {
    sampler?: ChatSamplerMetadata
    persona_id?: string | null
    byok_id?: string | null
    authors_note?: AuthorsNote | null
    /** M44 per-chat auto-summary config. Absent → feature off (default).
     *  Present with enabled=false → user had it on, turned off, we keep
     *  the threshold/model for easy re-enable. */
    auto_summarise?: AutoSummariseConfig
    /** Monotonic counter of provider tokens this chat has consumed over
     *  its whole lifetime — survives swipes, regenerates and message
     *  deletions. Written server-side after every successful stream. */
    usage_total?: {
      tokens_in: number
      tokens_out: number
      api_calls: number
    }
    /** M51 Sprint 3 wave 2 — per-chat theme override. When set, the SPA
     *  applies that preset on chat-mount and reverts to user-default
     *  on unmount. Empty / unset = use the user-default appearance.themePreset. */
    theme_preset?: string
    /** M55 — WuEco virtual provider flag. When true the SPA picks the
     *  `:lite` variant of any selected model on send so the server
     *  applies eco-mode caps (input 30k, output 1500, reasoning off). */
    eco_mode?: boolean
    [key: string]: unknown
  }
  created_at: string
  updated_at: string
  last_message_at?: string
}

/** Aggregate stats for a single chat (M38.3). */
export interface ChatStats {
  chat_id: string
  messages_total: number
  messages_user: number
  messages_assistant: number
  messages_system: number
  messages_hidden: number
  tokens_in_total: number
  tokens_out_total: number
  swipes_total: number
  first_message_at?: string
  last_message_at?: string
  unique_models_used: number
}

/** One summary row (M38.4). Three roles:
 *    auto    — rolling narrative maintained by the memory engine
 *    manual  — user-authored notes (free-form)
 *    pinned  — always-on key facts
 */
export interface Summary {
  id: string
  chat_id: string
  content: string
  role: 'auto' | 'manual' | 'pinned'
  covered_through_message_id?: number | null
  token_count: number
  model?: string
  position: number
  created_at: string
  updated_at: string
}

/** One hit from GET /api/chats/search. Server returns snippets wrapped
 *  with `<<<...>>>` markers highlighting the match positions — UI
 *  swaps those for `<mark>` on render. */
export interface SearchHit {
  chat_id: string
  chat_name: string
  character_id?: string | null
  character_name?: string
  message_id: number
  role: Role
  snippet: string
  created_at: string
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

/** M44 per-chat auto-summary config — opt-in, OFF by default.
 *
 *  Lives under chat_metadata.auto_summarise. Read by backend after each
 *  assistant turn's `done`-SSE: if Enabled && assistant_tokens_in
 *  (prompt size) >= ThresholdTokens, backend fires a background
 *  SummariseChat with the user's selected Model (and optional BYOKID
 *  override). User pays own tokens per summarise call. */
export interface AutoSummariseConfig {
  enabled: boolean
  /** Range 0..2_000_000 (UI-enforced slider + numeric input). Zero/
   *  negative is accepted as "fire every turn" — edge case, respected. */
  threshold_tokens: number
  /** Empty string → server uses defaultSummariserModel (gemini-2.5-flash). */
  model?: string
  /** Null/undefined → use chat's upstream (BYOK pinned on chat, else
   *  WuApi pool). Non-null UUID → a different BYOK key — user can pin
   *  cheap Gemini Flash for summaries while chat runs on Claude Sonnet. */
  byok_id?: string | null
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
  /** Silent flag — when true, message is greyed-out in the UI but
   *  still feeds into the model prompt (M38.2). */
  hidden?: boolean
  /** Speaker attribution in a group chat. Nil for user/system and for
   *  single-character assistant messages (fall back to chat.character_id). */
  character_id?: string | null
  /** Parallel to `swipes`: index i is attributed to this character.
   *  Set when the message holds multi-speaker swipes (group-chat
   *  greetings pool). When absent, every swipe falls back to
   *  `character_id`. */
  swipe_character_ids?: (string | null)[]
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

  create: (input: { character_id?: string; character_ids?: string[]; name?: string }) =>
    apiFetch<Chat>('/api/chats', {
      method: 'POST',
      body: JSON.stringify(input),
    }),

  search: (q: string, opts: { characterId?: string; limit?: number } = {}) => {
    const params = new URLSearchParams()
    params.set('q', q)
    if (opts.characterId) params.set('character_id', opts.characterId)
    if (opts.limit) params.set('limit', String(opts.limit))
    return apiFetch<{ items: SearchHit[] }>(`/api/chats/search?${params.toString()}`)
  },

  rename: (id: string, name: string) =>
    apiFetch<void>(`/api/chats/${id}`, {
      method: 'PATCH',
      body: JSON.stringify({ name }),
    }),

  setTags: (id: string, tags: string[]) =>
    apiFetch<void>(`/api/chats/${id}`, {
      method: 'PATCH',
      body: JSON.stringify({ tags }),
    }),

  listTags: () =>
    apiFetch<{ items: string[] }>('/api/chats/tags'),

  stats: (id: string) =>
    apiFetch<ChatStats>(`/api/chats/${id}/stats`),

  // ── Memory / summaries (M38.4) ─────────────────────────────────
  listSummaries: (chatID: string) =>
    apiFetch<{ items: Summary[] }>(`/api/chats/${chatID}/summaries`),
  createSummary: (chatID: string, content: string, pinned: boolean) =>
    apiFetch<Summary>(`/api/chats/${chatID}/summaries`, {
      method: 'POST',
      body: JSON.stringify({ content, pinned }),
    }),
  updateSummary: (chatID: string, sid: string, body: { content?: string; role?: string }) =>
    apiFetch<void>(`/api/chats/${chatID}/summaries/${sid}`, {
      method: 'PATCH',
      body: JSON.stringify(body),
    }),
  deleteSummary: (chatID: string, sid: string) =>
    apiFetch<void>(`/api/chats/${chatID}/summaries/${sid}`, {
      method: 'DELETE',
    }),
  summarize: (chatID: string, model?: string) =>
    apiFetch<{ summary: Summary | null; folded: number; message?: string }>(
      `/api/chats/${chatID}/summarize`,
      { method: 'POST', body: JSON.stringify({ model: model ?? '' }) },
    ),

  // ── M44 auto-summarise per-chat config ─────────────────────────
  // Opt-in feature. When enabled, after each assistant turn on this
  // chat, backend checks tokens_in >= threshold_tokens and fires
  // SummariseChat in a background goroutine using the picked
  // provider (wuapi or BYOK) + model. User pays their own tokens.
  setAutoSummarise: (chatID: string, cfg: AutoSummariseConfig) =>
    apiFetch<void>(`/api/chats/${chatID}/auto-summarise`, {
      method: 'PUT',
      body: JSON.stringify(cfg),
    }),
  clearAutoSummarise: (chatID: string) =>
    apiFetch<void>(`/api/chats/${chatID}/auto-summarise`, { method: 'DELETE' }),

  // Silent message toggle (M38.2). Reuses the edit endpoint with just
  // `hidden` in the body.
  setMessageHidden: (chatID: string, mid: number, hidden: boolean) =>
    apiFetch<void>(`/api/chats/${chatID}/messages/${mid}`, {
      method: 'PATCH',
      body: JSON.stringify({ hidden }),
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

  /**
   * M51 Sprint 3 wave 2 — set or clear the chat's theme preset override.
   * When set, the SPA applies that preset on mount and reverts to the
   * user-default appearance.themePreset on unmount. Pass `null` to clear.
   *
   * Note: the preset id is not validated server-side — sending an unknown
   * id will surface a friendly fallback in the theme store at apply time
   * rather than corrupting state.
   */
  setThemePreset: (id: string, preset: string | null) =>
    apiFetch<void>(`/api/chats/${id}/theme-preset`, {
      method: 'PUT',
      body: JSON.stringify({ theme_preset: preset }),
    }),

  /** Toggle WuEco virtual provider for this chat (M55). When enabled
   *  the SPA will append `:lite` to model ids before sending so the
   *  server applies eco-mode caps. Persisted in chat_metadata so the
   *  preference survives chat reopens. */
  setEcoMode: (id: string, enabled: boolean) =>
    apiFetch<void>(`/api/chats/${id}/eco-mode`, {
      method: 'PUT',
      body: JSON.stringify({ enabled }),
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

  /** Bulk-delete target message + every message beneath it. Returns the
   *  number of rows removed so the SPA can tell the user how many they
   *  just nuked (useful after a 429-flood cleanup). */
  deleteMessagesAfter: (chatID: string, messageID: number) =>
    apiFetch<{ deleted: number }>(
      `/api/chats/${chatID}/messages/${messageID}/delete-after`,
      { method: 'POST' },
    ),

  /** Fork the chat up to `messageID` into a sibling chat — the new chat
   *  copies character wiring + metadata + every message through this
   *  point, and the caller can continue from there without disturbing
   *  the original timeline. Returns the fresh Chat object for nav. */
  fork: (chatID: string, messageID: number) =>
    apiFetch<Chat>(
      `/api/chats/${chatID}/messages/${messageID}/fork`,
      { method: 'POST' },
    ),

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
  | { event: 'assistant_start'; data: { id: number; model: string; character_id?: string | null } }
  | { event: 'swipe_start'; data: { id: number; swipe_id: number } }
  | { event: 'continue_start'; data: { id: number; existing_len: number } }
  | { event: 'token'; data: { content: string } }
  | { event: 'done'; data: {
        id: number
        content: string
        reasoning?: string
        tokens_in: number
        tokens_out: number
        latency_ms: number
        finish_reason?: string
        /** Model id of the run that just completed (M53). Lets the
         *  client refresh `extras.model` on swipes/regens — without it
         *  the bubble's model badge stayed at the previous run's value
         *  until the user reloaded. */
        model?: string
        /** Fresh post-bump chat-level spend counter. Patched onto the
         *  cached Chat so the SPA's spend chip reflects swipes/regens
         *  that the per-message extras summation can't see. */
        usage_total?: {
          tokens_in: number
          tokens_out: number
          api_calls: number
        } | null
    } }
  | { event: 'error'; data: { kind: string; message: string } }
  | { event: 'raw'; data: unknown }

export interface SendMessageInput {
  content: string
  model?: string
  temperature?: number
  max_tokens?: number
  /** Who speaks next in a group chat. Ignored for single-char chats.
   *  When omitted from a group chat the server picks the first
   *  participant as a safe default. */
  speaker_id?: string
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

/** Continue — extend the existing assistant message with more content.
 *  Streams via SSE; `continue_start` replaces `assistant_start` and each
 *  `token` event should be APPENDED to the existing message (not added
 *  to a new row). Final `done.content` carries the combined text. */
export async function* continueMessageStream(
  chatID: string,
  messageID: number,
  input: Partial<SendMessageInput> = {},
  signal?: AbortSignal,
): AsyncGenerator<StreamEvent, void, unknown> {
  yield* streamSSE(`/api/chats/${chatID}/messages/${messageID}/continue`, input, signal)
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
