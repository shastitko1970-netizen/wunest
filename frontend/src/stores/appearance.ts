import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import { appearanceApi, type Appearance } from '@/api/appearance'
import { scopeCSS, globalGuardCSS } from '@/lib/cssScope'
import { applyCSPNonce } from '@/lib/cspNonce'

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
        // M51 Sprint 1 wave 3 — if the server-stored preset differs
        // from whatever theme.ts loaded from localStorage on cold-boot,
        // switch to it now. Lazy-import to avoid circular dep at module
        // init (theme.ts already lazy-imports us). Wrapped in try because
        // a stale id from a removed preset is non-fatal — theme.apply()
        // surfaces the error and falls back to nest-default-dark.
        if (server.themePreset) {
          try {
            const { useThemeStore } = await import('@/stores/theme')
            const theme = useThemeStore()
            if (theme.currentId !== server.themePreset) {
              await theme.apply(server.themePreset as never)
            }
          } catch { /* non-fatal — preset stays as cold-loaded */ }
        }
        // M51 Sprint 2 wave 3 — wire the system-prefers-color-scheme
        // listener AFTER the themePreset switch above, so the listener
        // attaches with the user's anchor preset already current. The
        // listener will immediately re-evaluate and flip to the pair
        // if system kind differs from anchor kind.
        if (server.followSystemTheme === true) {
          try {
            const { useThemeStore } = await import('@/stores/theme')
            useThemeStore().syncSystemPrefListener(true)
          } catch { /* non-fatal */ }
        }
      }
      loaded.value = true
    } catch {
      loaded.value = true  // fall back to whatever localStorage had
    }
  }

  // Debounced save: collect rapid edits (slider drag) into one PUT.
  //
  // M52.6 — also tracks `dirty` flag so a pending unsaved change can be
  // flushed via `flushPendingSave()` on tab unload. Without that flush,
  // closing the tab inside the 400ms debounce window dropped the edit
  // (PUT never fired) → on next load, fetchFromServer overwrote LS with
  // stale server state → user saw "settings reset themselves".
  let saveTimer: ReturnType<typeof setTimeout> | null = null
  let dirty = false
  function save() {
    dirty = true
    if (saveTimer) clearTimeout(saveTimer)
    saveTimer = setTimeout(async () => {
      saving.value = true
      try {
        await appearanceApi.put(appearance.value)
        dirty = false
      } finally {
        saving.value = false
      }
    }, 400)
  }

  /**
   * flushPendingSave — fires the pending PUT immediately, bypassing the
   * 400ms debounce. Used by:
   *
   *   - `beforeunload` window listener (registered below) so tab-close
   *     inside the debounce window doesn't drop the edit.
   *   - Future call sites that want a hard guarantee the server got
   *     the latest state before navigating somewhere risky.
   *
   * Uses `fetch(... keepalive: true)` so the request survives page
   * unload — supported in all modern browsers (Chrome 66+, Firefox 65+,
   * Safari 15+). Limit 64 KB; our Appearance blobs are usually well
   * under 1 KB unless customCss is huge (256 KB cap on server).
   */
  function flushPendingSave() {
    if (!dirty) return
    if (saveTimer) {
      clearTimeout(saveTimer)
      saveTimer = null
    }
    try {
      // Direct fetch with keepalive — bypassing apiFetch wrapper which
      // doesn't expose the keepalive flag. Same payload shape as
      // appearanceApi.put.
      void fetch('/api/me/appearance', {
        method: 'PUT',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(appearance.value),
        keepalive: true,
      })
      dirty = false
    } catch {
      // Best-effort. If the browser refused keepalive (rare), the LS
      // copy still has the change — next session will see stale-server
      // overwrite (existing behaviour, not worse).
    }
  }

  // Wire the beforeunload listener once at store init. `pagehide` is
  // the more modern equivalent that fires on bfcache too; we listen on
  // both for max coverage.
  if (typeof window !== 'undefined') {
    const flush = () => flushPendingSave()
    window.addEventListener('beforeunload', flush)
    window.addEventListener('pagehide', flush)
  }

  /** Mutate + auto-save. Callers pass the full Appearance (or a patch merged in). */
  function update(patch: Partial<Appearance>) {
    appearance.value = { ...appearance.value, ...patch }
    save()
  }

  /**
   * saveNow — fire a normal PUT immediately, bypassing the 400ms
   * debounce. Unlike `flushPendingSave` (which uses `fetch keepalive`
   * for unload paths and has tight Safari support), this is the right
   * tool for **discrete UI picks** like switching theme or font:
   *
   *   - Awaits the response so we can show a toast / error banner.
   *   - No keepalive — full normal fetch, reliable on every browser.
   *   - Cancels any pending debounce so the change isn't double-PUT.
   *
   * Slider-driven mutations (color picker, font scale) keep using
   * `update()` because their high-frequency edits benefit from the
   * 400ms coalescing.
   */
  async function saveNow(patch: Partial<Appearance>) {
    appearance.value = { ...appearance.value, ...patch }
    if (saveTimer) {
      clearTimeout(saveTimer)
      saveTimer = null
    }
    dirty = false
    saving.value = true
    try {
      await appearanceApi.put(appearance.value)
    } catch (e) {
      // Surface in console; UI doesn't need to halt — the mutation is
      // still applied locally and persisted to localStorage by the
      // watcher above. Worst case the change is lost on next cold-load
      // from server (which is the existing behaviour for any save
      // failure), but the user's session keeps the picked theme.
      console.warn('appearance.saveNow', e)
    } finally {
      saving.value = false
    }
  }

  /** Reset to empty — falls back to the Vuetify theme's defaults. */
  function reset() {
    appearance.value = {}
    save()
  }

  return { appearance, loaded, saving, safeMode, fetchFromServer, update, saveNow, reset, flushPendingSave }
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
  // M51 Sprint 1 wave 2 — wire previously-stored-but-not-applied fields.
  // `italicsColor` and `quoteColor` were imported from ST and persisted
  // for ages but the applier never read them, so users who imported a
  // tavern-style ST theme and then noticed quotes weren't tinted had no
  // recourse short of writing custom CSS. The variables are consumed by
  // chat-content stylesheets (MessageContent for em/i, blockquote).
  set('--nest-text-italic', a.italicsColor)
  set('--nest-text-quote', a.quoteColor)

  // M51 Sprint 2 wave 1 — first-class background + text-hierarchy
  // controls. Previously these tokens were derived (for text-secondary
  // /muted via color-mix) or only reachable via custom CSS (for bg /
  // surface). When the user explicitly sets a value here we write it
  // inline; clearing falls back to the preset cascade.
  set('--nest-bg', a.bgColor)
  // surfaceColor pumps both --nest-surface and --nest-bg-elevated.
  // The two are sibling tokens used for cards/sidebar/elevated chrome
  // and cohabit visually — a single user-control prevents them
  // drifting apart. If a future user wants them split, we'll add a
  // separate `bgElevatedColor` field; for now one knob keeps the UI
  // honest.
  set('--nest-surface', a.surfaceColor)
  set('--nest-bg-elevated', a.surfaceColor)
  set('--nest-text-secondary', a.textSecondaryColor)
  set('--nest-text-muted', a.textMutedColor)
  // M52.3 — uniform icon colour. Consumed by global rule on .mdi
  // (см. tokens/customization.css). Semantic Vuetify-coloured icons
  // (color="error" etc.) исключены через :not([class*="text-"]).
  set('--nest-icon-color', a.iconColor)

  // M51 Sprint 2 wave 1 — typography family. Maps the picker enum
  // onto a CSS font-family stack. 'custom' = pass user's literal
  // string verbatim. Browsers gracefully ignore stacks pointing at
  // unloaded fonts — we never fail-loud here.
  if (a.fontFamily) {
    const stack = resolveFontStack(a.fontFamily)
    if (stack) {
      root.style.setProperty('--nest-font-body', stack)
      // Display (h1..h4) follows body by default — single decision.
      // Mono is intentionally untouched.
      root.style.setProperty('--nest-font-display', stack)
    }
  } else {
    root.style.removeProperty('--nest-font-body')
    root.style.removeProperty('--nest-font-display')
  }

  // M51 Sprint 2 wave 1 — radius scale multiplier. Just writes the
  // multiplier; tokens/colors_and_type.css consumes it via calc()
  // for sm / base / lg radii. Pill is unaffected.
  if (typeof a.radiusScale === 'number' && a.radiusScale > 0) {
    root.style.setProperty('--nest-radius-scale', String(a.radiusScale))
  } else {
    root.style.removeProperty('--nest-radius-scale')
  }

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
  //
  // M52.4 — also flip a `data-nest-bg` attribute on <body> so Vuetify's
  // own `.v-application { background: rgb(var(--v-theme-background)) }`
  // can be made transparent (see customization.css). Without that
  // override, Vuetify's root container sits on top of the body's
  // background-image и закрывает его — юзер видит сплошной theme-bg.
  // Background image — M52.8 scoped to chat only.
  //
  // Previous approach put `body.style.backgroundImage = url(...)` which
  // bled into Library/Settings/Account/Docs (anywhere body is visible).
  // Now: write `--nest-bg-image` CSS var on :root, consumed by
  // `.nest-chat-main` rule (Chat.vue). Only the chat view renders the
  // image; other routes are unaffected. The `data-nest-bg` attribute
  // on body still flips so the translucent-chrome rules know to apply.
  if (typeof document !== 'undefined') {
    const body = document.body
    if (a.bgImageUrl && !SAFE_MODE) {
      root.style.setProperty('--nest-bg-image', `url("${cssEscape(a.bgImageUrl)}")`)
      body.setAttribute('data-nest-bg', '1')
    } else {
      root.style.removeProperty('--nest-bg-image')
      body.removeAttribute('data-nest-bg')
    }
    // Clean up any legacy inline body bg-image left from previous
    // sessions (M52.4-M52.7 wrote to body.style — clear it once on
    // upgrade so users don't see leftover bg outside chat).
    body.style.backgroundImage = ''
    body.style.backgroundSize = ''
    body.style.backgroundPosition = ''
    body.style.backgroundAttachment = ''
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
      // M51 Sprint 3 wave 3 — CSP nonce wiring (no-op until CSP enabled).
      applyCSPNonce(styleEl)
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
  // Belt-and-suspenders protection for admin panels. CRITICAL: this
  // layer is only useful when the user has custom CSS that might
  // break admin surfaces. Applying it universally (как было в первой
  // версии M43.1) ломало Vuetify overlay transitions у юзеров БЕЗ
  // custom CSS — !important visibility/opacity/pointer-events на
  // `.v-overlay__content *` убивало Vuetify fade-in/fade-out анимации
  // меню/диалогов/тултипов, кнопки "прыгали", весь UI выглядел как
  // после взрыва. Тестер: «все кнопки сломались, визуал весь сломался».
  //
  // Fix — make the guard CONDITIONAL:
  //   - Only inject when user actually has customCss set (the only
  //     scenario it guards against).
  //   - Keep the rules narrow: force visibility/opacity ONLY on
  //     .nest-admin subtree (our own elements, safe to hard-force);
  //     DON'T touch .v-overlay__content internals (Vuetify's own
  //     transition property states, off-limits). Overlay isolation is
  //     already handled by `globalGuardCSS` via @scope-exclusion.
  const GUARD_ID = 'nest-admin-guard'
  let guardEl = document.getElementById(GUARD_ID) as HTMLStyleElement | null
  const hasUserCSS = a.customCss && a.customCss.trim() && !SAFE_MODE
  if (!hasUserCSS) {
    // Nothing to guard against — remove any prior guard style so we
    // don't leave !important rules stuck on the page from a theme the
    // user just cleared.
    if (guardEl) guardEl.remove()
  } else {
    if (!guardEl) {
      guardEl = document.createElement('style')
      guardEl.id = GUARD_ID
      // M51 Sprint 3 wave 3 — CSP nonce wiring (no-op until CSP enabled).
      applyCSPNonce(guardEl)
      document.head.appendChild(guardEl)
    }
    guardEl.textContent = `
/* WuNest admin guard — keeps Settings/Account/Docs/Converter usable
 * even with aggressive user themes in scope=global. Placed AFTER
 * nest-user-css so these rules win on specificity ties.
 *
 * Narrow on purpose: force visibility/opacity ONLY on our own
 * .nest-admin subtree. Modal isolation (.v-overlay__content) is
 * handled by globalGuardCSS via @scope-exclusion — we must NOT touch
 * Vuetify overlay internals here, or overlay fade transitions break
 * for every user. */
.nest-admin,
.nest-admin * {
  visibility: visible !important;
  opacity: initial !important;
  pointer-events: auto !important;
}
.nest-admin [hidden] { display: none !important; }
/* Preserve Vuetify field internals inside .nest-admin — user themes
 * often zero-out borders/backgrounds on inputs/textareas, which hides
 * v-select triggers and v-text-field frames in Settings. Confined
 * to .nest-admin so modal field styling remains Vuetify's job. */
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

// M51 Sprint 2 wave 1 — map fontFamily picker enum onto a real CSS
// font-family stack. The four named presets reuse Google Fonts that
// are already preloaded by `tokens/colors_and_type.css` (via the
// @import on first paint), so picking them is a zero-network change.
// 'custom' = user wrote their own stack; we pass it through verbatim
// (the browser silently ignores unknown families and walks the stack).
function resolveFontStack(family: string): string | null {
  switch (family) {
    case 'system':
      // OS-native — fastest, most familiar look for utilitarian users.
      // Order mirrors common system-font advice: SF on macOS/iOS, Segoe
      // on Windows, Roboto on Android, generic sans-serif fallback.
      return 'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif'
    case 'sans':
      // Modern, clean — current default Outfit + system fallback.
      return '"Outfit", system-ui, -apple-system, "Segoe UI", sans-serif'
    case 'serif':
      // Reading-focused — Fraunces is preloaded for h1/h2 already.
      return '"Fraunces", "Source Serif 4", Georgia, serif'
    case 'mono':
      // For users who genuinely want everything monospaced — code-vibe.
      return '"JetBrains Mono", "Fira Code", Consolas, monospace'
    default:
      // Anything else = literal user string. Trim to avoid empty/space.
      const trimmed = family.trim()
      return trimmed.length > 0 ? trimmed : null
  }
}
