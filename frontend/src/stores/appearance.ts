import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import { appearanceApi, type Appearance } from '@/api/appearance'

/**
 * Appearance store — user-authored theming.
 *
 * Boot sequence:
 *   1. Load from localStorage (so first-paint carries the user's last choice
 *      even on a cold session before /api/me/appearance returns).
 *   2. Apply CSS vars + custom CSS to <document>.
 *   3. Fetch server copy and replace local if newer.
 *
 * Writes are debounced 400ms so slider dragging doesn't hammer the server.
 */

const LS_KEY = 'nest:appearance'
const CUSTOM_STYLE_ID = 'nest-custom-appearance-css'

export const useAppearanceStore = defineStore('appearance', () => {
  const appearance = ref<Appearance>({})
  const loaded = ref(false)
  const saving = ref(false)

  // Load from localStorage so first-paint reflects user choice.
  try {
    const raw = localStorage.getItem(LS_KEY)
    if (raw) appearance.value = JSON.parse(raw)
  } catch { /* ignore — corrupt LS is fine, we'll overwrite */ }

  // Hot-apply on any change. runs on mount + every mutation.
  watch(appearance, (v) => {
    applyAppearance(v)
    try { localStorage.setItem(LS_KEY, JSON.stringify(v)) } catch { /* quota etc. */ }
  }, { deep: true, immediate: true })

  async function fetchFromServer() {
    try {
      const server = await appearanceApi.get()
      // Server may return `{}` for never-customised users — don't wipe local
      // settings unless the server has real content.
      if (server && Object.keys(server).length > 0) {
        appearance.value = server
      }
      loaded.value = true
    } catch {
      loaded.value = true  // fall back to whatever localStorage had
    }
  }

  // Debounced save: collect rapid edits (slider drag) into one PUT.
  let saveTimer: ReturnType<typeof setTimeout> | null = null
  function save() {
    if (saveTimer) clearTimeout(saveTimer)
    saveTimer = setTimeout(async () => {
      saving.value = true
      try {
        await appearanceApi.put(appearance.value)
      } finally {
        saving.value = false
      }
    }, 400)
  }

  /** Mutate + auto-save. Callers pass the full Appearance (or a patch merged in). */
  function update(patch: Partial<Appearance>) {
    appearance.value = { ...appearance.value, ...patch }
    save()
  }

  /** Reset to empty — falls back to the Vuetify theme's defaults. */
  function reset() {
    appearance.value = {}
    save()
  }

  return { appearance, loaded, saving, fetchFromServer, update, reset }
})

// ─── CSS applier ──────────────────────────────────────────────────────
//
// Given an Appearance, write CSS custom properties onto :root so all
// component-level `var(--nest-…)` reads pick up the new values instantly.
// Also injects a <style> block for the customCss field and body-level
// background image.

function applyAppearance(a: Appearance) {
  if (typeof document === 'undefined') return
  const root = document.documentElement
  const set = (name: string, value: string | undefined) => {
    if (value) root.style.setProperty(name, value)
    else root.style.removeProperty(name)
  }

  set('--nest-accent', a.accent)
  set('--nest-text', a.mainTextColor)
  set('--nest-border', a.borderColor)

  // Font scale — multiply the root font-size. 14px is our baseline.
  if (typeof a.fontScale === 'number' && a.fontScale > 0) {
    root.style.setProperty('--nest-font-scale', String(a.fontScale))
    root.style.fontSize = `${14 * a.fontScale}px`
  } else {
    root.style.removeProperty('--nest-font-scale')
    root.style.fontSize = ''
  }

  // Chat width: writes a var consumed by .nest-chat-messages max-width.
  if (typeof a.chatWidth === 'number' && a.chatWidth > 0) {
    root.style.setProperty('--nest-chat-width', `${a.chatWidth}%`)
  } else {
    root.style.removeProperty('--nest-chat-width')
  }

  // Avatar corner radius.
  if (a.avatarStyle === 'square') {
    root.style.setProperty('--nest-avatar-radius', '4px')
  } else if (a.avatarStyle === 'round') {
    root.style.setProperty('--nest-avatar-radius', '50%')
  } else {
    root.style.removeProperty('--nest-avatar-radius')
  }

  // Chat display mode (consumed by MessageBubble).
  if (a.chatDisplay) {
    root.setAttribute('data-nest-chat-display', a.chatDisplay)
  } else {
    root.removeAttribute('data-nest-chat-display')
  }

  // Shadow toggle.
  if (a.shadows === false) {
    root.style.setProperty('--nest-shadow', 'none')
  } else {
    root.style.removeProperty('--nest-shadow')
  }

  // Reduced motion: disables transitions project-wide.
  if (a.reducedMotion) {
    root.style.setProperty('--nest-transition-fast', '0s')
    root.style.setProperty('--nest-transition-base', '0s')
  } else {
    root.style.removeProperty('--nest-transition-fast')
    root.style.removeProperty('--nest-transition-base')
  }

  // Background image.
  if (typeof document !== 'undefined') {
    const body = document.body
    if (a.bgImageUrl) {
      body.style.backgroundImage = `url("${cssEscape(a.bgImageUrl)}")`
      body.style.backgroundSize = 'cover'
      body.style.backgroundPosition = 'center'
      body.style.backgroundAttachment = 'fixed'
    } else {
      body.style.backgroundImage = ''
    }
  }

  // Blur strength (used by surfaces with backdrop-filter when bg image is set).
  if (typeof a.blurStrength === 'number') {
    root.style.setProperty('--nest-blur', `${a.blurStrength}px`)
  } else {
    root.style.removeProperty('--nest-blur')
  }

  // Custom CSS — inject as a <style> tag so users can hand-write selectors.
  // Trusting the authenticated user's own input; the CSP would block any
  // external loads anyway.
  let styleEl = document.getElementById(CUSTOM_STYLE_ID) as HTMLStyleElement | null
  if (a.customCss && a.customCss.trim()) {
    if (!styleEl) {
      styleEl = document.createElement('style')
      styleEl.id = CUSTOM_STYLE_ID
      document.head.appendChild(styleEl)
    }
    styleEl.textContent = a.customCss
  } else if (styleEl) {
    styleEl.remove()
  }
}

// Minimal CSS-string escape for URL values (quote handling). Keeps us off
// the DOMPurify dependency for such a targeted use.
function cssEscape(s: string): string {
  return s.replace(/"/g, '\\"').replace(/\n/g, '')
}
