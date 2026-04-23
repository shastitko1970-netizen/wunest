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

  // Any mutation → persist the whole bag. Small object, don't bother with
  // a debounce.
  watch(disableStreaming, () => {
    const next: PersistedPrefs = { disableStreaming: disableStreaming.value }
    try { localStorage.setItem(LS_KEY, JSON.stringify(next)) } catch { /* quota */ }
  })

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
      } catch { /* ignore */ }
    })
  }

  return { disableStreaming }
})
