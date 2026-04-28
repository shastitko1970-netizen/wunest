import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { applyCSPNonce } from '@/lib/cspNonce'

// ThemeManager (M42.1 → M51 Sprint 3 wave 1: data-driven registry)
//
// WuNest ships with five built-in themes authored against the Design System
// contract. Each theme is a plain CSS file paired with a sibling
// `*.theme.json` manifest. The manifest carries label, description, kind,
// accent, swatches, pair-id, sort-order, and the css filename. Vite's
// `import.meta.glob` discovers them at build time → THEME_PRESETS is
// derived; no hardcoded list to drift.
//
// On top of the presets, a user can paste their own CSS into the Appearance
// panel (M42.4). Custom CSS is stored server-side under
// `user.settings.custom_css` and injected as a second `<style>` block after
// the preset. Scope (`chat` | `global`) controls whether the custom payload
// is wrapped in `@scope (#chat) { ... }` before insertion.
//
// Loading strategy:
//   - Presets are bundled as separate Vite chunks via `?raw` imports.
//     They live in their own JS chunk so the initial page load doesn't pull
//     five themes the user never picked.
//   - On theme change we fetch the chunk (in practice cached after first
//     load), drop the <style id="nest-theme"> element, re-create it with the
//     new CSS. No FOUC because Vue re-render is gated on our store.
//   - Custom CSS lives in a SEPARATE <style id="nest-user-css"> so toggling
//     presets doesn't wipe the user's overlay.
//
// Safe mode: if the current URL has `?safe=1`, we skip BOTH the preset AND
// the user CSS. Gives users an emergency bail-out when a broken theme has
// hidden every interactive element of the app. See `src/main.ts` for wire-up.

// ── Manifest schema ─────────────────────────────────────────────────
// What `*.theme.json` files declare. Authors of new themes write this
// JSON directly; the runtime parses + sorts via Vite glob below.

/** A single 6-color palette used to render gallery card previews and
 *  the AppearancePanel mini-swatches. Roles map roughly to:
 *    - bg / surface — backgrounds (canvas, elevated cards)
 *    - border — frame strokes
 *    - text — primary readable color
 *    - accent — brand color (matches Vuetify primary)
 *    - accentOn — readable color used on accent backgrounds (e.g. CTA text) */
export interface ThemeSwatches {
  bg: string
  surface: string
  border: string
  text: string
  accent: string
  accentOn: string
}

export type ThemePreset =
  | 'nest-default-dark'
  | 'nest-default-light'
  | 'cyber-neon'
  | 'minimal-reader'
  | 'tavern-warm'

export interface ThemePresetMeta {
  id: ThemePreset
  label: string
  description: string
  kind: 'dark' | 'light'
  /** Brand color for this preset, mirrored from CSS `--SmartThemeQuoteColor`.
   *  Used to keep Vuetify's `primary` palette aligned with the active
   *  preset (so `<v-btn color="primary">` matches the visual brand). */
  accent: string
  /** Id of the same-family theme in the opposite kind. Flipping dark↔light
   *  prefers pair > kind default. Cyber-neon ↔ minimal-reader by design
   *  (both reader-focused narrow-margin vibes in their own kind).
   *  Omitted when there's no honest sibling in the bundled set. */
  pair?: ThemePreset
  /** Stable sort key for gallery / picker ordering. Lower goes first. */
  order: number
  /** 6-color palette for static previews — see ThemeSwatches doc. */
  swatches: ThemeSwatches
  /** CSS filename relative to `styles/themes/`. The loader resolves this
   *  to a `?raw` dynamic import on first apply. */
  css: string
}

// Vite glob — eager so all manifests are bundled at build time and
// available synchronously for THEME_PRESETS. Files matched: every
// `*.theme.json` in `styles/themes/`. Runtime parses each into a
// ThemePresetMeta; Vite throws on malformed JSON during build.
const manifestModules = import.meta.glob<{ default: ThemePresetMeta }>(
  '@/styles/themes/*.theme.json',
  { eager: true },
)

// Build the registry once on module init. Sort by `order` (asc),
// secondary by `id` for stable UI when two share the same order.
export const THEME_PRESETS: ThemePresetMeta[] = Object.values(manifestModules)
  .map(mod => mod.default)
  .filter((m): m is ThemePresetMeta => !!m && typeof m.id === 'string')
  .sort((a, b) => (a.order - b.order) || a.id.localeCompare(b.id))

// CSS file loaders — dynamic per-preset chunks via `?raw`. Built from
// the manifest's `css` field so adding a 6th theme is one file
// (`my-theme.theme.json` + `my-theme.css`), not three changes here.
//
// Map keyed by preset id. We pre-build it once so apply() can look up
// the loader synchronously without filtering THEME_PRESETS each time.
const THEME_LOADERS: Record<string, () => Promise<{ default: string }>> = {
  // Note: explicit `?raw` literal queries are required by Vite — it
  // can't resolve a fully-dynamic glob for `?raw` imports. We list
  // each `.css` once here. When adding a new theme, also add a line
  // here so the chunk gets bundled.
  'nest-default-dark.css':  () => import('@/styles/themes/nest-default-dark.css?raw'),
  'nest-default-light.css': () => import('@/styles/themes/nest-default-light.css?raw'),
  'cyber-neon.css':         () => import('@/styles/themes/cyber-neon.css?raw'),
  'minimal-reader.css':     () => import('@/styles/themes/minimal-reader.css?raw'),
  'tavern-warm.css':        () => import('@/styles/themes/tavern-warm.css?raw'),
}

function loaderForPreset(meta: ThemePresetMeta): (() => Promise<{ default: string }>) | null {
  return THEME_LOADERS[meta.css] ?? null
}

// Stored separately from `nest:theme` (used by AppShell/Settings for the
// Vuetify light/dark toggle) so migrating users don't get their preset
// cleared by a legacy value. M42.2 will collapse both concepts into one.
const LS_THEME = 'nest:theme-preset'
const STYLE_ID = 'nest-theme'

// `window.__NEST_SAFE_MODE__` is set very early in main.ts if `?safe=1` is
// in the URL. Read via typed helper; missing flag === normal mode.
function isSafeMode(): boolean {
  if (typeof window === 'undefined') return false
  return (window as unknown as { __NEST_SAFE_MODE__?: boolean }).__NEST_SAFE_MODE__ === true
}

// Defensive read from localStorage — only accept values that exist in the
// current preset list. A legacy / typo'd entry would otherwise silently
// break first paint; here it falls back to the safe default.
function readStoredPreset(): ThemePreset {
  const raw = localStorage.getItem(LS_THEME)
  if (raw && THEME_PRESETS.some(p => p.id === raw)) {
    return raw as ThemePreset
  }
  return 'nest-default-dark'
}

// M51 Sprint 2 wave 3 — module-scope state for the system-prefs
// listener. Lives outside the store factory so the listener survives
// HMR / re-instantiation gracefully — at most one MQL is attached at
// any time, and `syncSystemPrefListener` toggles it idempotently.
let systemMql: MediaQueryList | null = null
let systemMqlHandler: ((e: MediaQueryListEvent) => void) | null = null

export const useThemeStore = defineStore('theme', () => {
  // Current preset id. On first mount we hydrate from localStorage, falling
  // back to the dark default so brand-new users get a usable shell.
  const currentId = ref<ThemePreset>(readStoredPreset())
  const loading = ref(false)
  const error = ref<string | null>(null)

  const current = computed<ThemePresetMeta>(() => {
    return THEME_PRESETS.find(t => t.id === currentId.value) ?? THEME_PRESETS[0]
  })

  /**
   * apply — low-level: load the preset CSS, swap the <style> tag,
   * persist to localStorage + Appearance.themePreset.
   *
   * Does NOT touch `followSystemTheme`. Use `userPick(id)` for paths
   * that should disable system-follow (gallery clicks, picker clicks).
   * The boot path, server-sync path, and system-prefs flip path all
   * call this directly so they don't accidentally turn off follow-mode.
   */
  async function apply(id: ThemePreset) {
    // Safe mode bypass — keep shell pristine regardless of user picks.
    if (isSafeMode()) {
      error.value = 'safe mode: theme switch disabled'
      return
    }
    const meta = THEME_PRESETS.find(p => p.id === id)
    if (!meta) {
      error.value = `unknown theme: ${id}`
      return
    }
    const loader = loaderForPreset(meta)
    if (!loader) {
      error.value = `theme css not bundled: ${meta.css}`
      return
    }
    loading.value = true
    error.value = null
    try {
      const mod = await loader()
      const css = mod.default
      let el = document.getElementById(STYLE_ID) as HTMLStyleElement | null
      if (!el) {
        el = document.createElement('style')
        el.id = STYLE_ID
        // M51 Sprint 3 wave 3 — apply CSP nonce if a per-request meta
        // is present. No-op when CSP is off (current state); ensures
        // we don't break when CSP gets tightened.
        applyCSPNonce(el)
        // Place BEFORE the user-css slot so user overrides win naturally by
        // DOM order. `nest-user-css` is injected in src/stores/appearance.ts
        // (M42.4 — custom CSS); linking here keeps the layering explicit.
        document.head.appendChild(el)
      }
      el.textContent = css
      currentId.value = id
      localStorage.setItem(LS_THEME, id)
      // Clear any user-applied accent override: the preset's own
      // --SmartThemeQuoteColor should now cascade into --nest-accent
      // without fighting an inline :root override from Appearance.
      // Lazy-import so theme store doesn't hard-depend on appearance
      // store (keeps the store graph acyclic).
      //
      // M51 Sprint 1 wave 3 — also persist the picked preset into the
      // appearance blob so it travels server-side with the rest of the
      // user's settings. localStorage stays the cold-load cache, but
      // server is source of truth on second device login.
      try {
        const { useAppearanceStore } = await import('@/stores/appearance')
        const appearance = useAppearanceStore()
        const patch: Record<string, unknown> = {}
        if (appearance.appearance.accent) patch.accent = undefined
        if (appearance.appearance.themePreset !== id) patch.themePreset = id
        if (Object.keys(patch).length > 0) {
          // M53 — theme picks are DISCRETE (a tap, not a slider drag),
          // so the 400ms debounce buys us nothing and risks the save
          // being dropped if the user navigates away or backgrounds
          // the tab before it fires. Mobile testers hit this regularly:
          // Pick theme → close tab → reopen → "default theme again".
          //
          // `saveNow` issues a normal PUT immediately, awaiting the
          // response. Reliable on all browsers (Safari included —
          // unlike `keepalive` flushes which have tight support).
          // Awaited so we don't return from `apply()` before the PUT
          // is at least in flight; if the network is slow the user
          // still gets the local-state update via the patch above.
          await appearance.saveNow(patch as Partial<typeof appearance.appearance>)
        }
      } catch {
        // Non-fatal — worst case the old accent stays and the user
        // can clear it manually from the Appearance color picker, and
        // the preset is still in localStorage so this device remembers.
      }
    } catch (e) {
      error.value = (e as Error).message
    } finally {
      loading.value = false
    }
  }

  /**
   * userPick — manual preset pick from a UI control (gallery card,
   * picker button). Differs from apply() in one way: it disables
   * `followSystemTheme` because explicitly choosing a theme is a
   * strong signal "I want this exact look", overriding the auto
   * dark/light flipping.
   *
   * macOS-style: toggling Auto-mode in System Settings then picking
   * Light manually disables Auto. Same idea here.
   */
  async function userPick(id: ThemePreset) {
    await apply(id)
    try {
      const { useAppearanceStore } = await import('@/stores/appearance')
      const appearance = useAppearanceStore()
      if (appearance.appearance.followSystemTheme === true) {
        appearance.update({ followSystemTheme: false })
        // Also detach the listener so the next system-flip doesn't
        // fight the user's explicit choice.
        syncSystemPrefListener(false)
      }
    } catch {
      // Non-fatal — store-graph acyclic constraint.
    }
  }

  /**
   * applyForSystemPref — picks the preset that best matches the
   * supplied OS color-scheme, using the current preset as the anchor:
   *
   *   - If current already matches the system kind → noop (no flicker).
   *   - Otherwise prefer current.pair if it's of the right kind.
   *   - Else fallback to the bundled default of that kind.
   *
   * Goes through apply(), NOT userPick(), so an OS-driven flip doesn't
   * disable followSystemTheme — that would defeat the feature.
   */
  function applyForSystemPref(systemKind: 'dark' | 'light') {
    const cur = current.value
    if (cur.kind === systemKind) return

    let next: ThemePreset | null = null
    if (cur.pair) {
      const pair = THEME_PRESETS.find(p => p.id === cur.pair)
      if (pair && pair.kind === systemKind) next = pair.id
    }
    if (!next) {
      next = systemKind === 'dark' ? 'nest-default-dark' : 'nest-default-light'
    }
    if (next !== cur.id) {
      void apply(next)
    }
  }

  /**
   * syncSystemPrefListener — idempotent toggle for the
   * matchMedia('prefers-color-scheme: dark') subscription.
   *
   *   enabled=true   — attach listener if not already; immediately
   *                    apply current system pref so the on-screen
   *                    state reflects the toggle landing.
   *   enabled=false  — detach listener if attached. Stays on the
   *                    current preset (we don't auto-revert to
   *                    themePreset; users would be surprised to see
   *                    their theme jump after toggling Off).
   *
   * Safe to call repeatedly; the module-scope `systemMql` guard
   * prevents stacking listeners.
   */
  function syncSystemPrefListener(enabled: boolean) {
    if (typeof window === 'undefined' || !window.matchMedia) return
    if (enabled && !systemMql) {
      systemMql = window.matchMedia('(prefers-color-scheme: dark)')
      systemMqlHandler = (e) => applyForSystemPref(e.matches ? 'dark' : 'light')
      systemMql.addEventListener('change', systemMqlHandler)
      // Apply current system state once — covers the case where the
      // user just toggled the feature on and the current preset
      // doesn't match the OS color-scheme.
      applyForSystemPref(systemMql.matches ? 'dark' : 'light')
    } else if (!enabled && systemMql) {
      if (systemMqlHandler) systemMql.removeEventListener('change', systemMqlHandler)
      systemMql = null
      systemMqlHandler = null
    }
  }

  /** Bootstrap on app-start — call once from main.ts after the store is ready. */
  async function init() {
    if (isSafeMode()) return
    await apply(currentId.value)
  }

  return {
    currentId,
    current,
    loading,
    error,
    apply,
    userPick,
    applyForSystemPref,
    syncSystemPrefListener,
    init,
    presets: THEME_PRESETS,
  }
})
