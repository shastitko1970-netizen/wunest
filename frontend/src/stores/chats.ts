import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  chatsApi,
  regenerateStream,
  sendMessageStream,
  swipeMessageStream,
  type Chat,
  type ChatSamplerMetadata,
  type Message,
  type SendMessageInput,
  type StreamEvent,
} from '@/api/chats'

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
      messages.value = msgs.items
    } finally {
      messagesLoading.value = false
    }
  }

  async function createForCharacter(characterID: string): Promise<Chat> {
    const chat = await chatsApi.create({ character_id: characterID })
    list.value = [chat, ...list.value]
    return chat
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

  /** Send a user message and stream the assistant reply. */
  async function send(input: SendMessageInput) {
    if (!currentId.value || streaming.value) return

    // Optimistic: append a temporary user row that'll be swapped for the
    // persisted one once the `user_message` event arrives.
    const optimistic: Message = {
      id: -Date.now(),
      chat_id: currentId.value,
      role: 'user',
      content: input.content,
      swipe_id: 0,
      created_at: new Date().toISOString(),
    }
    messages.value = [...messages.value, optimistic]

    await runStream(
      () => sendMessageStream(currentId.value!, input, streamAbort!.signal),
      optimistic,
    )
  }

  /** Drop the last assistant reply and stream a fresh one. */
  async function regenerate(input: Partial<SendMessageInput> = {}) {
    if (!currentId.value || streaming.value) return

    // Remove the last assistant message locally so the UI reflects the
    // pending regen before the stream even starts. Server does the DB delete.
    for (let i = messages.value.length - 1; i >= 0; i--) {
      if (messages.value[i]!.role === 'assistant') {
        messages.value.splice(i, 1)
        break
      }
    }

    await runStream(
      () => regenerateStream(currentId.value!, input, streamAbort!.signal),
      null,
    )
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
          case 'token': {
            if (assistantId === null) break
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
            break
          }
          case 'error': {
            streamError.value = `${ev.data.kind}: ${ev.data.message}`
            break
          }
        }
      }
    } catch (e) {
      if ((e as Error).name !== 'AbortError') {
        streamError.value = (e as Error).message
      }
    } finally {
      streaming.value = false
      streamAbort = null
    }
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

  return {
    list, listLoading, listError,
    currentId, currentChat, messages, messagesLoading,
    streaming, streamError,
    currentCharacterId,
    fetchList, open, createForCharacter, remove, rename,
    send, regenerate, swipe, selectSwipe, stopStreaming,
    editMessage, deleteMessage,
    setSampler,
  }
})
