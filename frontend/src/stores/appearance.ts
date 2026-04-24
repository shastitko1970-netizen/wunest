import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import { appearanceApi, type Appearance } from '@/api/appearance'
import { scopeCSS } from '@/lib/cssScope'

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

/**
 * Safe mode flag — read once at boot from the URL query string.
 * When `?safe` (or `?safe=1`) is present, we skip injecting the user's
 * custom CSS and their background image so the app's own chrome stays
 * usable even if the user's theme rules broke layout. The user can then
 * open Settings → Appearance and either Reset or edit out the bad CSS.
 *
 * Recovery instructions for a completely broken shell:
 *   Tap the browser URL bar, append `?safe` to the host, reload.
 *
 * This is read once on module init — Vue's reactivity doesn't need to
 * re-read it; a URL change reloads the SPA anyway.
 */
export const SAFE_MODE = (() => {
  if (typeof window === 'undefined') return false
  try {
    const params = new URLSearchParams(window.location.search)
    return params.has('safe') || params.has('nest-safe')
  } catch { return false }
})()

export const useAppearanceStore = defineStore('appearance', () => {
  const appearance = ref<Appearance>({})
  const loaded = ref(false)
  const saving = ref(false)
  // Exposed as a ref so the SafeModeBanner can react without re-reading URL.
  const safeMode = ref(SAFE_MODE)

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

  return { appearance, loaded, saving, safeMode, fetchFromServer, update, reset }
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

  // Font scale — expose as a CSS variable consumed by chat-content
  // stylesheets (MessageBubble/MessageContent) via calc(). We intentionally
  // DO NOT mutate <html> font-size: that also scaled Vuetify button
  // heights, icon sizes and header chips via rem, which users read as
  // "the send button shrunk for no reason". Only message text obeys the
  // slider now; UI chrome stays stable.
  if (typeof a.fontScale === 'number' && a.fontScale > 0) {
    root.style.setProperty('--nest-chat-font-scale', String(a.fontScale))
  } else {
    root.style.removeProperty('--nest-chat-font-scale')
  }
  // Legacy --nest-font-scale kept for any older CSS that still reads
  // it; always removed so Reset cleans up fully (prior versions wrote it
  // together with the root font-size change).
  root.style.removeProperty('--nest-font-scale')
  root.style.fontSize = ''

  // Chat width: writes a var consumed by .nest-chat-messages max-width.
  if (typeof a.chatWidth === 'number' && a.chatWidth > 0) {
    root.style.setProperty('--nest-chat-width', `${a.chatWidth}%`)
  } else {
    root.style.removeProperty('--nest-chat-width')
  }

  // Avatar shape — two axes: corner radius and aspect ratio.
  //
  //   round    → circle (default) — small identity markers
  //   square   → rounded rect, 1:1
  //   portrait → rounded rect, 3:4 — matches SillyTavern card art
  //              (avatars "look like the character" instead of
  //              center-cropping the face into a circle)
  //
  // The aspect var is consumed by `.v-avatar` via the override in
  // global.scss so every Vuetify avatar in the app participates. Tiny
  // message-bubble avatars can opt out with `.nest-avatar--forced-round`.
  if (a.avatarStyle === 'square') {
    root.style.setProperty('--nest-avatar-radius', '4px')
    root.setAttribute('data-nest-avatar-style', 'square')
  } else if (a.avatarStyle === 'round') {
    root.style.setProperty('--nest-avatar-radius', '50%')
    root.setAttribute('data-nest-avatar-style', 'round')
  } else if (a.avatarStyle === 'portrait') {
    root.style.setProperty('--nest-avatar-radius', '12px')
    root.setAttribute('data-nest-avatar-style', 'portrait')
  } else {
    root.style.removeProperty('--nest-avatar-radius')
    root.removeAttribute('data-nest-avatar-style')
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

  // Background image. Skipped in safe mode so an unreachable / oversized
  // remote image can't be the reason the page won't paint.
  if (typeof document !== 'undefined') {
    const body = document.body
    if (a.bgImageUrl && !SAFE_MODE) {
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
  // external loads anyway. SKIP in safe mode: this is the most common
  // reason the app's own shell breaks, so `?safe` in the URL always gets
  // the user back to a working Settings page.
  //
  // Scope resolution — see resolveScope() for the backwards-compat rules.
  // Short version: fresh installs default to 'chat', pre-M26 users who
  // already had CSS get 'global' (their legacy behaviour) unless they
  // explicitly flip the toggle.
  let styleEl = document.getElementById(CUSTOM_STYLE_ID) as HTMLStyleElement | null
  if (a.customCss && a.customCss.trim() && !SAFE_MODE) {
    if (!styleEl) {
      styleEl = document.createElement('style')
      styleEl.id = CUSTOM_STYLE_ID
      document.head.appendChild(styleEl)
    }
    const scope = resolveScope(a)
    styleEl.textContent = scope === 'chat'
      ? scopeCSS(a.customCss, '#chat')
      : a.customCss
  } else if (styleEl) {
    styleEl.remove()
  }
}

/**
 * resolveScope maps an Appearance to the actual scope to apply. Explicit
 * `customCssScope` wins. When unset (legacy users from before M26, or
 * brand-new users), we pick a default that doesn't silently change the
 * behaviour of their CSS:
 *
 *   - Already has customCss in their stored appearance → 'global'
 *     (their CSS was applied globally before M26, keep it that way).
 *   - No CSS yet → 'chat' (new default; safer for ST imports).
 *
 * Once the user flips the toggle or imports via ST JSON (both set the
 * field explicitly), this fallback doesn't fire.
 */
export function resolveScope(a: Appearance): 'chat' | 'global' {
  if (a.customCssScope) return a.customCssScope
  return (a.customCss && a.customCss.trim()) ? 'global' : 'chat'
}

// Minimal CSS-string escape for URL values (quote handling). Keeps us off
// the DOMPurify dependency for such a targeted use.
function cssEscape(s: string): string {
  return s.replace(/"/g, '\\"').replace(/\n/g, '')
}
