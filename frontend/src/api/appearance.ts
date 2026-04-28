import { apiFetch } from '@/api/client'

// ─── Types ────────────────────────────────────────────────────────────

export type AvatarStyle = 'round' | 'square' | 'portrait'
export type ChatDisplay = 'flat' | 'bubbles' | 'document'

/**
 * Appearance — user-customisable theming. Mirrors the subset of SillyTavern
 * theme fields we support, plus our own additions (accent override, etc.).
 *
 * Every field is optional: unset = use the current Vuetify theme's default.
 * Server stores this as an opaque JSON blob, so adding a new field doesn't
 * need a backend change.
 */
export interface Appearance {
  // Core tokens (override --nest-accent etc.).
  accent?: string            // hex or rgba: becomes --nest-accent
  mainTextColor?: string     // --nest-text
  italicsColor?: string      // italic emphasis → --nest-text-italic (M51 Sprint 1)
  quoteColor?: string        // blockquote text → --nest-text-quote (M51 Sprint 1)
  borderColor?: string       // --nest-border

  // M51 Sprint 2 wave 1 — first-class background controls. Previously
  // only reachable via raw custom CSS or by importing an ST theme that
  // bundled --SmartThemeBlurTintColor. Each maps 1:1 onto a --nest-*
  // token; clearing the field removes the inline override and lets the
  // active preset's CSS cascade win again.
  bgColor?: string           // --nest-bg (page backdrop)
  surfaceColor?: string      // --nest-surface AND --nest-bg-elevated (cards, sidebar)
  textSecondaryColor?: string // --nest-text-secondary
  textMutedColor?: string    // --nest-text-muted

  // M52.3 — uniform icon colour. Drives `--nest-icon-color`, consumed
  // by a global `.mdi:not([class*="text-"])` rule so all decorative
  // Material Design icons (topbar nav, drawer chrome, dialog actions)
  // pick up the user's chosen colour. Vuetify-classed semantic icons
  // (text-error / text-success / text-warning) are deliberately
  // EXEMPTED — green check / red error keep their meaning regardless
  // of decorative palette choice. When unset, icons inherit currentColor
  // from their parent (today's behaviour, identical to pre-M52.3).
  iconColor?: string


  // M51 Sprint 2 wave 1 — typography family picker. Five named presets +
  // a 'custom' escape hatch where the user drops in a literal CSS
  // font-family stack. The picker writes both `--nest-font-body` (used
  // by .nest-body / paragraph chrome) and `--nest-font-display` (used
  // by .nest-h1..h4) — keeping them in sync is what makes "pick a font"
  // feel like one decision rather than three. Mono stays bound to
  // JetBrains Mono unless user sets it directly via custom CSS — code
  // blocks shouldn't suddenly become serif.
  fontFamily?: 'system' | 'sans' | 'serif' | 'mono' | string

  // M51 Sprint 2 wave 1 — radius scale multiplier (0.5 … 2). Pushes
  // through `--nest-radius-scale` which the base radii consume via
  // calc() in tokens/colors_and_type.css. 1 = stock; 0.5 = sharp;
  // 2 = generous. Pill (100px) is intentionally NOT scaled — its
  // semantics are "always max-round".
  radiusScale?: number

  // M51 Sprint 2 wave 3 — follow OS dark/light setting. When true,
  // the theme store attaches a `prefers-color-scheme` listener and
  // auto-flips between the active preset and its `pair` (or the
  // bundled default of the matching kind if no pair). Default
  // undefined ≡ false — opt-in to avoid surprising users with a
  // jarring first-paint flip. A manual preset pick disables this
  // automatically (matching macOS Auto-mode UX).
  followSystemTheme?: boolean

  // Density & size.
  fontScale?: number         // 0.7–1.5 (chat-only multiplier; clamped in fromST)
  chatWidth?: number         // 40–100 (percent of chat main column; clamped in fromST)
  avatarStyle?: AvatarStyle  // round | square | portrait (3:4 aspect)
  chatDisplay?: ChatDisplay  // bubbles | flat | document

  // Visual flourish.
  shadows?: boolean          // if false, .nest-* shadows are suppressed
  reducedMotion?: boolean    // disables hover transforms / transitions
  bgImageUrl?: string        // applied to body background
  blurStrength?: number      // 0–20, matches ST semantics

  // Custom CSS — appended to <head> after all other styles.
  customCss?: string

  /** Where user CSS applies. `chat` (default) wraps it in `@scope (#chat)`
   *  so rules for `body`/`input`/`textarea` don't paint over settings /
   *  library / menu — they only hit elements inside the chat container.
   *  `global` applies raw, matching classic ST behaviour. User picks in
   *  Appearance → Custom CSS. */
  customCssScope?: 'chat' | 'global'

  // Render inline HTML in messages (sanitized). On by default; turn off if
  // you're worried about models smuggling markup in.
  htmlRendering?: boolean

  // M51 Sprint 1 wave 3 — server-synced theme preset. Previously the
  // active preset id lived ONLY in localStorage (`nest:theme-preset`),
  // so a login on a new device painted with the local fallback rather
  // than the user's last pick. By piggybacking on the appearance blob
  // it ride-alongs the existing per-user PUT and arrives in the same
  // GET as accent/customCss/etc., keeping cross-device parity.
  //
  // Typed loosely as string so this file doesn't import from
  // stores/theme.ts (avoid circular dep). The theme store validates
  // unknown values on apply().
  themePreset?: string

  // Metadata — when imported from ST, we stash the original name here.
  importedFrom?: string
}

// ─── API ──────────────────────────────────────────────────────────────

export const appearanceApi = {
  get: () => apiFetch<Appearance>('/api/me/appearance'),

  /** Full replacement — send the whole object every time. */
  put: (payload: Appearance) =>
    apiFetch<void>('/api/me/appearance', {
      method: 'PUT',
      body: JSON.stringify(payload),
    }),
}

// ─── SillyTavern theme shape (subset we translate) ────────────────────

export interface STTheme {
  name?: string
  main_text_color?: string
  italics_text_color?: string
  quote_text_color?: string
  border_color?: string
  blur_tint_color?: string
  font_scale?: number
  chat_width?: number
  avatar_style?: number      // 0 = round, 1 = square, 2 = portrait
  chat_display?: number      // 0 = flat, 1 = bubbles, 2 = document
  noShadows?: boolean
  reduced_motion?: boolean
  blur_strength?: number
  custom_css?: string
}

/**
 * Translate an ST theme JSON into our Appearance shape. Unknown/unmapped
 * fields are silently dropped — we don't try to preserve every ST toggle,
 * just the ones that have a meaningful analogue in our UI.
 */
export function fromST(st: STTheme): Appearance {
  const out: Appearance = {}
  if (st.main_text_color) out.mainTextColor = st.main_text_color
  if (st.italics_text_color) out.italicsColor = st.italics_text_color
  if (st.quote_text_color) out.quoteColor = st.quote_text_color
  if (st.border_color) out.borderColor = st.border_color
  // ST doesn't have a dedicated "accent" field; the border color is the
  // closest functional match for focus rings, so we reuse it as accent if
  // the user hasn't picked one explicitly.
  if (st.border_color && !out.accent) out.accent = st.border_color
  // ST's `blur_tint_color` = the main background tint (== our primary
  // --SmartThemeBlurTintColor). We don't have a dedicated Appearance
  // field for it, but fromST is the ENTRY POINT for ST imports — the
  // `custom_css` we emit below carries the `:root { --SmartTheme…}`
  // rules, so the variable propagates via CSS. Here we additionally
  // surface it as the top-level accent-less backdrop by NOT nulling
  // out any existing bgImageUrl (the converter can keep both). This
  // mapping was previously missing → themes imported via converter
  // lost their background tint after "Apply to me".
  if (typeof st.font_scale === 'number') out.fontScale = clamp(st.font_scale, 0.7, 1.5)
  if (typeof st.chat_width === 'number') out.chatWidth = clamp(st.chat_width, 40, 100)
  // Avatar style values follow ST's historic numeric encoding:
  //   0 = round, 1 = square, 2 = portrait (3:4 aspect). The third option
  //   was added in SillyTavern 1.12 — our previous mapping silently
  //   dropped it, leaving imported Lilac-Witch-like themes stuck on
  //   the default round-avatar shape.
  if (st.avatar_style === 0) out.avatarStyle = 'round'
  if (st.avatar_style === 1) out.avatarStyle = 'square'
  if (st.avatar_style === 2) out.avatarStyle = 'portrait'
  if (st.chat_display === 0) out.chatDisplay = 'flat'
  if (st.chat_display === 1) out.chatDisplay = 'bubbles'
  if (st.chat_display === 2) out.chatDisplay = 'document'
  if (typeof st.noShadows === 'boolean') out.shadows = !st.noShadows
  if (typeof st.reduced_motion === 'boolean') out.reducedMotion = st.reduced_motion
  if (typeof st.blur_strength === 'number') out.blurStrength = clamp(st.blur_strength, 0, 30)
  if (st.custom_css) out.customCss = st.custom_css
  if (st.name) out.importedFrom = st.name
  return out
}

/**
 * Translate our Appearance back into an ST-compatible JSON. Designed so a
 * WuNest user can export their theme and hand it to an ST user, or vice
 * versa. Field names match ST's on-wire schema verbatim.
 */
export function toST(a: Appearance): STTheme {
  const out: STTheme = {
    name: a.importedFrom || 'WuNest Export',
  }
  if (a.mainTextColor) out.main_text_color = a.mainTextColor
  if (a.italicsColor) out.italics_text_color = a.italicsColor
  if (a.quoteColor) out.quote_text_color = a.quoteColor
  if (a.borderColor) out.border_color = a.borderColor
  if (typeof a.fontScale === 'number') out.font_scale = a.fontScale
  if (typeof a.chatWidth === 'number') out.chat_width = a.chatWidth
  // Symmetric round-trip with fromST. Value 2 (portrait) was missing
  // before M51-Sprint1 so a portrait → export → import cycle quietly
  // collapsed avatars back to square.
  if (a.avatarStyle === 'round') out.avatar_style = 0
  if (a.avatarStyle === 'square') out.avatar_style = 1
  if (a.avatarStyle === 'portrait') out.avatar_style = 2
  if (a.chatDisplay === 'flat') out.chat_display = 0
  if (a.chatDisplay === 'bubbles') out.chat_display = 1
  if (a.chatDisplay === 'document') out.chat_display = 2
  if (typeof a.shadows === 'boolean') out.noShadows = !a.shadows
  if (typeof a.reducedMotion === 'boolean') out.reduced_motion = a.reducedMotion
  if (typeof a.blurStrength === 'number') out.blur_strength = a.blurStrength
  if (a.customCss) out.custom_css = a.customCss
  return out
}

function clamp(n: number, lo: number, hi: number): number {
  return Math.max(lo, Math.min(hi, n))
}
