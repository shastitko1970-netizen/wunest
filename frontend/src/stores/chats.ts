import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  chatsApi,
  continueMessageStream,
  regenerateStream,
  sendMessageStream,
  swipeMessageStream,
  type AuthorsNote,
  type Chat,
  type ChatSamplerMetadata,
  type Message,
  type SendMessageInput,
  type StreamEvent,
} from '@/api/chats'
import { usePreferencesStore } from '@/stores/preferences'

// The chats store is intentionally simple:
//   - A flat list of chats (sidebar).
//   - One currently-open chat with its messages loaded eagerly.
// More sophisticated caching (per-chat memoization, scroll positions) can
// come later when we actually feel the pain.

export const useChatsStore = defineStore('chats', () => {
  const list = ref<Chat[]>([])
  const listLoading = ref(false)
  const listError = ref<string | null>(null)

  const currentId = ref<string | null>(null)
  const currentChat = ref<Chat | null>(null)
  const messages = ref<Message[]>([])
  const messagesLoading = ref(false)

  // True while a stream is in flight. UI uses this to disable the Send button
  // and show a "thinking…" indicator.
  const streaming = ref(false)
  const streamError = ref<string | null>(null)
  let streamAbort: AbortController | null = null

  const currentCharacterId = computed(() => currentChat.value?.character_id ?? null)

  async function fetchList() {
    listLoading.value = true
    listError.value = null
    try {
      const { items } = await chatsApi.list()
      list.value = items
    } catch (e) {
      listError.value = (e as Error).message
    } finally {
      listLoading.value = false
    }
  }

  async function open(id: string) {
    if (currentId.value === id) return
    currentId.value = id
    currentChat.value = null
    messages.value = []
    messagesLoading.value = true
    try {
      const [chat, msgs] = await Promise.all([
        chatsApi.get(id),
        chatsApi.listMessages(id),
      ])
      currentChat.value = chat
      // Empty message list comes back as null from Go's nil-slice
      // marshaling; coerce so any `.length` reads downstream stay safe.
      messages.value = Array.isArray(msgs?.items) ? msgs.items : []
    } finally {
      messagesLoading.value = false
    }
  }

  async function createForCharacter(characterID: string): Promise<Chat> {
    const chat = await chatsApi.create({ character_id: characterID })
    list.value = [chat, ...list.value]
    return chat
  }

  // createGroupChat starts a chat with 2+ character participants.
  // Server picks the name (first character + " + N"); caller can
  // override via `name`. Single-member arrays fall through to the
  // existing single-char path for cleaner sidebar UX.
  async function createGroupChat(characterIDs: string[], name?: string): Promise<Chat> {
    if (characterIDs.length === 1) {
      return createForCharacter(characterIDs[0])
    }
    const chat = await chatsApi.create({ character_ids: characterIDs, name })
    list.value = [chat, ...list.value]
    return chat
  }

  // existingForCharacter returns the newest chat owned by the given
  // character (or null if none). Used by Library's "Chat" button so
  // clicking a card reopens the last conversation instead of piling up
  // duplicate chats — users reported 10+ clones per character.
  //
  // `list` is sorted newest-first by fetchList, so the first hit is
  // the most-recently-touched chat.
  function existingForCharacter(characterID: string): Chat | null {
    return list.value.find(c => c.character_id === characterID) ?? null
  }

  async function remove(id: string) {
    await chatsApi.delete(id)
    list.value = list.value.filter(c => c.id !== id)
    if (currentId.value === id) {
      currentId.value = null
      currentChat.value = null
      messages.value = []
    }
  }

  async function rename(id: string, name: string) {
    await chatsApi.rename(id, name)
    const found = list.value.find(c => c.id === id)
    if (found) found.name = name
    if (currentChat.value?.id === id) currentChat.value.name = name
  }

  /** Send a user message and stream the assistant reply. When `content` is
   *  empty string we skip the optimistic user row — the server treats that
   *  as "no new user turn, reply from existing history" (used by the
   *  "ask for a reply" button when the chat tails on a user message). */
  async function send(input: SendMessageInput) {
    if (!currentId.value || streaming.value) return

    let optimistic: Message | null = null
    if (input.content && input.content.trim().length > 0) {
      // Optimistic: append a temporary user row that'll be swapped for the
      // persisted one once the `user_message` event arrives.
      optimistic = {
        id: -Date.now(),
        chat_id: currentId.value,
        role: 'user',
        content: input.content,
        swipe_id: 0,
        created_at: new Date().toISOString(),
      }
      messages.value = [...messages.value, optimistic]
    }

    await runStream(
      () => sendMessageStream(currentId.value!, input, streamAbort!.signal),
      optimistic,
    )
  }

  /** Generate a new variant of the last assistant reply, keeping the prior
   *  ones addressable via swipe navigation. Implemented on top of swipe so
   *  history is never destroyed — the "♻ new variant" button in the UI and
   *  the regenerate hotkey both call into this.
   *
   *  Fall-through to the destructive regenerateStream only when there is no
   *  existing assistant message to swipe from (rare — usually means the user
   *  deleted the tail; in that case the UI should be routing through
   *  `requestReplyFromLastUser` instead, but we keep the fallback to avoid
   *  hard-breaking old call sites). */
  async function regenerate(input: Partial<SendMessageInput> = {}) {
    if (!currentId.value || streaming.value) return

    // Find the last assistant message. If one exists, route through swipe —
    // additive, non-destructive, prior variants stay reachable.
    for (let i = messages.value.length - 1; i >= 0; i--) {
      const m = messages.value[i]
      if (m && m.role === 'assistant') {
        await swipe(m, input)
        return
      }
    }

    // No assistant tail — fall back to the destructive server regenerate.
    // Callers that hit this branch almost certainly want requestReplyFromLastUser
    // instead (user deleted the bot reply and wants a new one); we take the
    // safe route anyway.
    await runStream(
      () => regenerateStream(currentId.value!, input, streamAbort!.signal),
      null,
    )
  }

  /** Ask the model to reply to the existing last-user-message without typing
   *  anything new. Fires when the user's tail message has no assistant
   *  follow-up yet — typically after deleting the bot's previous reply.
   *
   *  Server treats `content: ''` as "no new user turn; just stream a reply
   *  from current history", so we don't append a blank user row. */
  async function requestReplyFromLastUser(input: Partial<SendMessageInput> = {}) {
    if (!currentId.value || streaming.value) return
    await send({ ...(input as SendMessageInput), content: '' })
  }

  /** Swipe — append a NEW variant to an existing assistant message (keeps
   *  the previous versions addressable via swipes[]). Streams the new
   *  content into the same message row. */
  async function swipe(message: Message, input: Partial<SendMessageInput> = {}) {
    if (!currentId.value || streaming.value) return
    // Optimistic: clear the message's content locally; stream will refill.
    const row = messages.value.find(m => m.id === message.id)
    if (row) {
      const oldContent = row.content
      const swipes = Array.isArray(row.swipes) ? [...row.swipes] : []
      if (swipes.length === 0) swipes.push(oldContent)
      swipes.push('')
      row.swipes = swipes
      row.swipe_id = swipes.length - 1
      row.content = ''
    }
    await runStream(
      () => swipeStreamLazy(currentId.value!, message.id, input),
      null,
    )
  }

  /** Select an existing swipe by index — simple PATCH, no stream. */
  async function selectSwipe(message: Message, swipeID: number) {
    if (!currentId.value) return
    try {
      const updated = await chatsApi.selectSwipe(currentId.value, message.id, swipeID)
      const row = messages.value.find(m => m.id === message.id)
      if (row) {
        row.content = updated.content
        row.swipe_id = updated.swipe_id
        row.swipes = updated.swipes
      }
    } catch (e) {
      streamError.value = (e as Error).message
    }
  }

  // Lazy wrapper because swipeMessageStream needs the signal which is
  // instantiated inside runStream; we capture it via a closure at call-time.
  function swipeStreamLazy(chatID: string, mid: number, input: Partial<SendMessageInput>) {
    return swipeMessageStream(chatID, mid, input, streamAbort!.signal)
  }
  function continueStreamLazy(chatID: string, mid: number, input: Partial<SendMessageInput>) {
    return continueMessageStream(chatID, mid, input, streamAbort!.signal)
  }

  /** Continue — extend the target assistant message with more content.
   *  Tokens append to the existing row; final `done` event carries the
   *  combined text. Only valid on the latest assistant message (server
   *  rejects non-assistant + non-latest targets). */
  async function continueAssistant(message: Message, input: Partial<SendMessageInput> = {}) {
    if (!currentId.value || streaming.value) return
    if (message.role !== 'assistant') return
    await runStream(
      () => continueStreamLazy(currentId.value!, message.id, input),
      null,
    )
  }

  // runStream is the shared driver for the generator-based SSE flows
  // (send + regenerate). Centralises the streaming state lifecycle.
  async function runStream(
    gen: () => AsyncGenerator<StreamEvent, void, unknown>,
    optimistic: Message | null,
  ) {
    streaming.value = true
    streamError.value = null
    streamAbort = new AbortController()

    let assistantId: number | null = null
    try {
      for await (const ev of gen()) {
        switch (ev.event) {
          case 'user_message': {
            // Replace optimistic user row with persisted one (send only).
            if (optimistic) {
              const idx = messages.value.indexOf(optimistic)
              if (idx >= 0) messages.value.splice(idx, 1, ev.data)
            }
            break
          }
          case 'assistant_start': {
            assistantId = ev.data.id
            messages.value = [
              ...messages.value,
              {
                id: assistantId,
                chat_id: currentId.value!,
                role: 'assistant',
                content: '',
                // character_id attributes this bubble to whoever the server
                // actually picked as speaker — critical in group chats so the
                // bubble header shows the right name immediately instead of
                // falling back to the chat's default character. Server
                // emits it on assistant_start; we thread it onto the row
                // so MessageBubble.displayName resolves correctly on the
                // first paint.
                character_id: ev.data.character_id ?? undefined,
                swipe_id: 0,
                extras: { model: ev.data.model },
                created_at: new Date().toISOString(),
              },
            ]
            break
          }
          case 'swipe_start': {
            // Existing message row — just route subsequent tokens into it.
            assistantId = ev.data.id
            const existing = messages.value.find(m => m.id === assistantId)
            if (existing) {
              existing.content = ''
              existing.swipe_id = ev.data.swipe_id
            }
            break
          }
          case 'continue_start': {
            // Existing message row — tokens APPEND (not replace). Keep
            // current content intact; the row reference is unchanged.
            assistantId = ev.data.id
            break
          }
          case 'token': {
            if (assistantId === null) break
            // When "disable streaming" is on, we buffer tokens server-side
            // by simply not painting them to the row. The `done` event
            // below still fires and sets the full content in one shot —
            // so the UX is "Thinking…" → complete message, no jitter.
            if (usePreferencesStore().disableStreaming) break
            const row = messages.value.find(m => m.id === assistantId)
            if (row) row.content += ev.data.content
            break
          }
          case 'done': {
            const row = messages.value.find(m => m.id === ev.data.id)
            if (row) {
              row.content = ev.data.content
              row.extras = {
                ...(row.extras ?? {}),
                reasoning: ev.data.reasoning,
                tokens_in: ev.data.tokens_in,
                tokens_out: ev.data.tokens_out,
                latency_ms: ev.data.latency_ms,
                finish_reason: ev.data.finish_reason,
              }
            }
            // Sync the monotonic spend counter onto the cached chat so
            // chat-level surfaces (header chip, stats panel) update
            // immediately — no refetch, works across swipes/regenerates.
            if (ev.data.usage_total && currentChat.value && currentChat.value.id === currentId.value) {
              currentChat.value.chat_metadata = {
                ...(currentChat.value.chat_metadata ?? {}),
                usage_total: ev.data.usage_total,
              }
            }
            break
          }
          case 'error': {
            streamError.value = formatStreamError(ev.data.kind, ev.data.message)
            break
          }
        }
      }
    } catch (e) {
      if ((e as Error).name !== 'AbortError') {
        // Upstream may return a JSON error body (auth gate, rate limit, etc.).
        // Pull out the human-readable message instead of dumping raw JSON at
        // the user.
        streamError.value = formatStreamError('', (e as Error).message)
      }
    } finally {
      streaming.value = false
      streamAbort = null
    }
  }

  /**
   * Take the (often JSON-shaped) server/upstream error soup and turn it
   * into a single human-readable line. Recognizes:
   *
   *   - kind="upstream_status" with an "upstream 402: {json}" suffix
   *     → extract nested error.message + map known error.type codes
   *     (insufficient_gold, rate_limited) to friendlier text with a CTA.
   *   - nest_access_required                              → "Need access code…"
   *   - plain JSON body {error, message}                  → message || error
   *   - anything else                                      → passthrough
   */
  function formatStreamError(kind: string, message: string): string {
    // Try to dig the embedded JSON out of the message — server-side
    // writeSSEError wraps upstream body as `"upstream 402: {json}"`.
    const jsonStart = message.indexOf('{')
    if (jsonStart >= 0) {
      try {
        const parsed = JSON.parse(message.slice(jsonStart))
        const inner = parsed?.error ?? parsed
        if (inner && typeof inner === 'object') {
          const type = typeof inner.type === 'string' ? inner.type : ''
          const msg = typeof inner.message === 'string' ? inner.message : ''
          if (type === 'insufficient_gold') {
            // WuApi ships a Telegram bot for top-up — surface the command
            // cleanly. Room for a real /gold button later.
            return msg || 'Недостаточно wu-gold. Пополните через бота /gold в Telegram.'
          }
          if (type === 'nest_access_required') {
            return msg || 'WuNest в закрытой бете — введите код доступа в профиле.'
          }
          if (type === 'rate_limited') {
            return msg || 'Слишком много запросов. Попробуйте через минуту.'
          }
          if (msg) return msg
        }
      } catch {
        // Fall through to raw message if JSON can't be parsed.
      }
    }
    // Tolerate the older plain-JSON body shape (no nested "error" key).
    try {
      const parsed = JSON.parse(message) as { error?: string; message?: string }
      if (parsed.message) return parsed.message
      if (typeof parsed.error === 'string') return parsed.error
    } catch {
      // Not JSON — just show kind + message.
    }
    return kind ? `${kind}: ${message}` : message
  }

  function stopStreaming() {
    if (streamAbort) streamAbort.abort()
  }

  /** Persist sampler settings into the current chat's chat_metadata. */
  async function setSampler(sampler: ChatSamplerMetadata) {
    if (!currentId.value) return
    await chatsApi.setSampler(currentId.value, sampler)
    if (currentChat.value) {
      currentChat.value.chat_metadata = {
        ...(currentChat.value.chat_metadata ?? {}),
        sampler,
      }
    }
  }

  /** Persist (or clear) Author's Note on the current chat. */
  async function setAuthorsNote(note: AuthorsNote | null) {
    if (!currentId.value) return
    await chatsApi.setAuthorsNote(currentId.value, note)
    if (currentChat.value) {
      currentChat.value.chat_metadata = {
        ...(currentChat.value.chat_metadata ?? {}),
        authors_note: note,
      }
    }
  }

  /** Edit a message's content in place (no re-stream). */
  async function editMessage(message: Message, newContent: string) {
    if (!currentId.value) return
    await chatsApi.editMessage(currentId.value, message.id, newContent)
    const row = messages.value.find(m => m.id === message.id)
    if (row) row.content = newContent
  }

  /** Delete one message from the current chat. */
  async function deleteMessage(message: Message) {
    if (!currentId.value) return
    await chatsApi.deleteMessage(currentId.value, message.id)
    messages.value = messages.value.filter(m => m.id !== message.id)
  }

  /** Delete the chosen message + every message below it in the same
   *  chat. Server returns how many rows were dropped; we return that
   *  so the UI can show a "deleted N messages" confirmation. */
  async function deleteMessagesAfter(message: Message) {
    if (!currentId.value) return 0
    const res = await chatsApi.deleteMessagesAfter(currentId.value, message.id)
    messages.value = messages.value.filter(m => m.id < message.id)
    return res.deleted
  }

  /** Fork — clone this chat's history up to `message` into a new sibling
   *  chat and return the new chat's id so the caller can navigate to it.
   *  The current chat is unchanged; the branch lives beside it. */
  async function fork(message: Message): Promise<string | null> {
    if (!currentId.value) return null
    const newChat = await chatsApi.fork(currentId.value, message.id)
    // Prepend into the sidebar list so user sees it top of "Today".
    list.value = [newChat, ...list.value]
    return newChat.id
  }

  return {
    list, listLoading, listError,
    currentId, currentChat, messages, messagesLoading,
    streaming, streamError,
    currentCharacterId,
    fetchList, open, createForCharacter, createGroupChat, existingForCharacter, remove, rename,
    send, regenerate, requestReplyFromLastUser, swipe, selectSwipe, continueAssistant, stopStreaming,
    editMessage, deleteMessage, deleteMessagesAfter, fork,
    setSampler, setAuthorsNote,
  }
})
