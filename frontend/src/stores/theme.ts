import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

// ThemeManager (M42.1 foundation)
//
// WuNest ships with five built-in themes authored against the Design System
// contract. Each theme is a plain CSS file that overrides the `--SmartTheme*`
// family and optionally adds scoped tweaks to the ST alias selectors
// (`.mes`, `#chat`, `#send_textarea`). Because our global.scss maps every
// `--nest-*` variable through a `--SmartTheme*` fallback chain, the themes
// propagate automatically across the app shell without per-component wiring.
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
  /** Id of the same-family theme in the opposite kind. Flipping dark↔light
   *  from the Settings toggle prefers pair > last-picked > kind default.
   *  Cyber-neon and minimal-reader pair with each other by design
   *  (both reader-focused narrow-margin vibes in their own kind).
   *  Omitted when there's no honest sibling in the 5-preset set. */
  pair?: ThemePreset
}

// Metadata that surfaces in the Appearance picker — single source of truth
// for preview labels, so we don't hard-code strings in the Vue template.
export const THEME_PRESETS: ThemePresetMeta[] = [
  {
    id: 'nest-default-dark',
    label: 'Nest — dark',
    description: 'Фирменная тёмная тема. Серый-угольный фон, coral-акцент.',
    kind: 'dark',
    pair: 'nest-default-light',
  },
  {
    id: 'nest-default-light',
    label: 'Nest — light',
    description: 'Бумажно-светлая. Инспирирована Dossier CRM.',
    kind: 'light',
    pair: 'nest-default-dark',
  },
  {
    id: 'cyber-neon',
    label: 'Cyber neon',
    description: 'Тёмный фиолет, магента-акцент, свечение.',
    kind: 'dark',
    // Cyber-neon's reader-focused narrow column pairs best with
    // minimal-reader on the light side; both prioritise text density
    // over chrome. There's no dedicated "cyber-pastel" light twin yet.
    pair: 'minimal-reader',
  },
  {
    id: 'minimal-reader',
    label: 'Minimal reader',
    description: 'Максимальная плотность текста без декора — для длинных чтений.',
    kind: 'light',
    pair: 'cyber-neon',
  },
  {
    id: 'tavern-warm',
    label: 'Tavern warm',
    description: 'Теплый янтарный. Роудтрип-эстетика старой корчмы.',
    kind: 'dark',
    // No warm-light sibling; defaults to nest-default-light on flip
    // (no pair declared).
  },
]

/**
 * Preferred pair resolution for dark↔light flips from Settings toggle.
 * Order:
 *   1. Current preset has `pair` → use it.
 *   2. Last-picked of the target kind (per-kind LS memory) → use it.
 *   3. Kind's default (nest-default-{dark,light}) → fallback.
 *
 * This keeps UX predictable: cyber-neon flips to minimal-reader and
 * back as a coherent pair, and custom user round-trips still preserve
 * their last-picked via LS memory.
 */
export function resolvePairFor(currentId: ThemePreset, targetKind: 'dark' | 'light'): ThemePreset {
  const current = THEME_PRESETS.find(p => p.id === currentId)
  if (current?.pair) {
    const pair = THEME_PRESETS.find(p => p.id === current.pair)
    if (pair && pair.kind === targetKind) return pair.id
  }
  const remembered = localStorage.getItem(`nest:last-theme-${targetKind}`) as
    ThemePreset | null
  if (remembered && THEME_PRESETS.some(p => p.id === remembered && p.kind === targetKind)) {
    return remembered
  }
  return targetKind === 'dark' ? 'nest-default-dark' : 'nest-default-light'
}

// Vite bundles each theme as its own chunk via `?raw` + dynamic import.
// Type the function map so TS catches typos at compile time.
const THEME_LOADERS: Record<ThemePreset, () => Promise<{ default: string }>> = {
  'nest-default-dark':  () => import('@/styles/themes/nest-default-dark.css?raw'),
  'nest-default-light': () => import('@/styles/themes/nest-default-light.css?raw'),
  'cyber-neon':         () => import('@/styles/themes/cyber-neon.css?raw'),
  'minimal-reader':     () => import('@/styles/themes/minimal-reader.css?raw'),
  'tavern-warm':        () => import('@/styles/themes/tavern-warm.css?raw'),
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

export const useThemeStore = defineStore('theme', () => {
  // Current preset id. On first mount we hydrate from localStorage, falling
  // back to the dark default so brand-new users get a usable shell.
  const currentId = ref<ThemePreset>(readStoredPreset())
  const loading = ref(false)
  const error = ref<string | null>(null)

  const current = computed<ThemePresetMeta>(() => {
    return THEME_PRESETS.find(t => t.id === currentId.value) ?? THEME_PRESETS[0]
  })

  async function apply(id: ThemePreset) {
    // Safe mode bypass — keep shell pristine regardless of user picks.
    if (isSafeMode()) {
      error.value = 'safe mode: theme switch disabled'
      return
    }
    const loader = THEME_LOADERS[id]
    if (!loader) {
      error.value = `unknown theme: ${id}`
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
        // Place BEFORE the user-css slot so user overrides win naturally by
        // DOM order. `nest-user-css` is injected in src/stores/appearance.ts
        // (M42.4 — custom CSS); linking here keeps the layering explicit.
        document.head.appendChild(el)
      }
      el.textContent = css
      currentId.value = id
      localStorage.setItem(LS_THEME, id)
      // Per-kind memory so the Settings light/dark toggle can round-trip
      // (user in cyber-neon flips to light → nest-default-light; flips
      // back to dark → cyber-neon again, not the generic default).
      const meta = THEME_PRESETS.find(p => p.id === id)
      if (meta) {
        localStorage.setItem(`nest:last-theme-${meta.kind}`, id)
      }
      // Clear any user-applied accent override: the preset's own
      // --SmartThemeQuoteColor should now cascade into --nest-accent
      // without fighting an inline :root override from Appearance.
      // Lazy-import so theme store doesn't hard-depend on appearance
      // store (keeps the store graph acyclic).
      try {
        const { useAppearanceStore } = await import('@/stores/appearance')
        const appearance = useAppearanceStore()
        if (appearance.appearance.accent) {
          appearance.update({ accent: undefined })
        }
      } catch {
        // Non-fatal — worst case the old accent stays and the user
        // can clear it manually from the Appearance color picker.
      }
    } catch (e) {
      error.value = (e as Error).message
    } finally {
      loading.value = false
    }
  }

  /** Bootstrap on app-start — call once from main.ts after the store is ready. */
  async function init() {
    if (isSafeMode()) return
    await apply(currentId.value)
  }

  return { currentId, current, loading, error, apply, init, presets: THEME_PRESETS }
})
