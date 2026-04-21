import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { chatsApi, sendMessageStream, type Chat, type Message, type SendMessageInput } from '@/api/chats'

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

    streaming.value = true
    streamError.value = null
    streamAbort = new AbortController()

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

    let assistantId: number | null = null

    try {
      for await (const ev of sendMessageStream(currentId.value, input, streamAbort.signal)) {
        switch (ev.event) {
          case 'user_message': {
            // Replace optimistic row by id match on position.
            const idx = messages.value.indexOf(optimistic)
            if (idx >= 0) messages.value.splice(idx, 1, ev.data)
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

  return {
    list, listLoading, listError,
    currentId, currentChat, messages, messagesLoading,
    streaming, streamError,
    currentCharacterId,
    fetchList, open, createForCharacter, remove, rename, send, stopStreaming,
  }
})
