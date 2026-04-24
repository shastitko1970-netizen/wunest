import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import { appearanceApi, type Appearance } from '@/api/appearance'
import { scopeCSS, globalGuardCSS } from '@/lib/cssScope'

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
// Public Selector Contract (M42.1): user CSS injected into <style id="nest-
// user-css">. Theme preset CSS lives in <style id="nest-theme"> appended
// earlier so our injection here wins by DOM order.
const CUSTOM_STYLE_ID = 'nest-user-css'
// Legacy id used pre-M42.4. Removed on first apply so browsers don't end
// up with two stale <style> blocks after upgrade.
const LEGACY_CUSTOM_STYLE_ID = 'nest-custom-appearance-css'

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
  // One-time cleanup: if a previous build of WuNest injected under the
  // legacy id, remove it so upgrade doesn't leave a stale <style> in the
  // head. Safe to call every apply — document.getElementById is cheap.
  const legacyEl = document.getElementById(LEGACY_CUSTOM_STYLE_ID)
  if (legacyEl) legacyEl.remove()

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
      // Global: protect admin surfaces (Settings/Account/Docs) from
      // aggressive themes. On modern browsers this wraps in
      // `@scope (body) to (.nest-admin)`; on Firefox CSS is applied
      // as-is (scope-exclusion не поддерживается, trust the user).
      : globalGuardCSS(a.customCss)
  } else if (styleEl) {
    styleEl.remove()
  }

  // ── Admin guard layer ────────────────────────────────────────────
  // Belt-and-suspenders protection for admin panels. Even with
  // `@scope (body) to (.nest-admin)` some rules leak through on
  // browsers that don't fully implement `to (...)` or when the user
  // CSS has `all: initial` / `visibility: hidden` tricks that survive
  // scope-exclusion via inheritance. This injects a tiny reset AFTER
  // the user style so admin elements stay visible/usable regardless.
  // Rules are narrow — we force properties we've seen break (display,
  // visibility, opacity, pointer-events) to sane defaults with
  // `!important` — not full `all: revert` so legitimate Vuetify
  // styling stays.
  const GUARD_ID = 'nest-admin-guard'
  let guardEl = document.getElementById(GUARD_ID) as HTMLStyleElement | null
  if (!guardEl) {
    guardEl = document.createElement('style')
    guardEl.id = GUARD_ID
    document.head.appendChild(guardEl)
  }
  guardEl.textContent = `
/* WuNest admin guard — keeps Settings/Account/Docs/Converter usable
 * even with aggressive user themes in scope=global. Placed AFTER
 * nest-user-css so these rules win on specificity ties. */
.nest-admin,
.nest-admin * {
  visibility: visible !important;
  opacity: initial !important;
  pointer-events: auto !important;
}
.nest-admin [hidden] { display: none !important; }
/* Preserve Vuetify field internals — user themes often zero-out
 * borders on inputs/textareas, which hides v-select triggers and
 * v-text-field frames inside Settings. */
.nest-admin .v-field,
.nest-admin .v-field__input,
.nest-admin .v-field__outline,
.nest-admin .v-selection-control,
.nest-admin .v-input,
.nest-admin .v-label {
  color: inherit !important;
  background: initial;
}
`.trim()
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
