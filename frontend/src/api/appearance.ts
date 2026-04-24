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
  italicsColor?: string      // italic emphasis
  quoteColor?: string        // blockquote text
  borderColor?: string       // --nest-border

  // Density & size.
  fontScale?: number         // 0.85–1.3 (multiplier on base 14px)
  chatWidth?: number         // 50–100 (percent of chat main column)
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
  avatar_style?: number      // 0 = round, 1 = square
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
  if (a.avatarStyle === 'round') out.avatar_style = 0
  if (a.avatarStyle === 'square') out.avatar_style = 1
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
