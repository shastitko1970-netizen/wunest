import { defineStore } from 'pinia'
import { ref, watch } from 'vue'

// Local-only user preferences that don't round-trip through the backend.
// Kept in localStorage so tabs stay in sync via the `storage` event and
// we never block on a network call to render a checkbox.
//
// When a preference becomes multi-device (e.g. BYOK default), move it
// into nest_users.settings + /api/me/... endpoints instead of adding
// it here.

const LS_KEY = 'nest:preferences:v1'

interface PersistedPrefs {
  // When true, the chat view waits for the full response before rendering
  // the assistant message — useful if watching tokens stream in is
  // distracting. The SSE stream still runs; we just don't repaint per
  // token (server doesn't need to change).
  disableStreaming?: boolean
  // Group-chat flow: after each assistant message, auto-trigger the
  // next speaker's turn (round-robin, no user message). Gives the
  // "NPCs chatter among themselves" feel without click-every-turn.
  groupAutoNext?: boolean
  // Group-chat flow: when sending a message, scan it for participant
  // names and auto-switch the speaker accordingly. Saves a click when
  // the user wrote "Alice, what do you think?" — Alice responds.
  groupDetectMention?: boolean
}

function loadFromStorage(): PersistedPrefs {
  try {
    const raw = localStorage.getItem(LS_KEY)
    if (!raw) return {}
    const parsed = JSON.parse(raw) as unknown
    if (parsed && typeof parsed === 'object') return parsed as PersistedPrefs
    return {}
  } catch {
    return {}
  }
}

export const usePreferencesStore = defineStore('preferences', () => {
  const stored = loadFromStorage()
  const disableStreaming = ref<boolean>(stored.disableStreaming ?? false)
  // Group-chat preferences default ON — tester feedback said manual-only
  // group chats feel tedious, and mention-detect is a strict UX win (it
  // only fires when you actually name someone, so there's no surprise
  // when you don't).
  const groupAutoNext = ref<boolean>(stored.groupAutoNext ?? false)
  const groupDetectMention = ref<boolean>(stored.groupDetectMention ?? true)

  function persist() {
    const next: PersistedPrefs = {
      disableStreaming: disableStreaming.value,
      groupAutoNext: groupAutoNext.value,
      groupDetectMention: groupDetectMention.value,
    }
    try { localStorage.setItem(LS_KEY, JSON.stringify(next)) } catch { /* quota */ }
  }

  // Any mutation → persist the whole bag. Small object, don't bother with
  // a debounce.
  watch([disableStreaming, groupAutoNext, groupDetectMention], persist)

  // Cross-tab sync — a different tab toggling the pref should reflect here
  // without waiting for a reload.
  if (typeof window !== 'undefined') {
    window.addEventListener('storage', (e) => {
      if (e.key !== LS_KEY || e.newValue == null) return
      try {
        const parsed = JSON.parse(e.newValue) as PersistedPrefs
        if (typeof parsed.disableStreaming === 'boolean') {
          disableStreaming.value = parsed.disableStreaming
        }
        if (typeof parsed.groupAutoNext === 'boolean') {
          groupAutoNext.value = parsed.groupAutoNext
        }
        if (typeof parsed.groupDetectMention === 'boolean') {
          groupDetectMention.value = parsed.groupDetectMention
        }
      } catch { /* ignore */ }
    })
  }

  return { disableStreaming, groupAutoNext, groupDetectMention }
})
