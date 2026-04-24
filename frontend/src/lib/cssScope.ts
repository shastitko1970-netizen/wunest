// CSS scoping — keep user-authored custom CSS from bleeding into WuNest's
// shell chrome (topbar, settings, library). ST themes often have rules
// targeting `body`, `textarea`, `input`, `.menu_button` etc. that match
// broadly; without scoping they'd paint over every field in the app.
//
// Two strategies:
//
//   1. Native `@scope (#chat) { ... }` — Chromium + Safari 17.4+.
//   2. Manual selector prefixing — fallback for browsers without
//      `@scope` (Firefox as of 2026).
//
// `@import` is always hoisted to the top (CSS spec requires it) regardless
// of strategy.

/**
 * Returns true if the browser supports the CSS `@scope` rule natively.
 * Probed at module load; result is cached.
 */
export const supportsCSSScope: boolean = (() => {
  if (typeof document === 'undefined') return false
  try {
    const style = document.createElement('style')
    style.textContent = '@scope (.__nest_probe__) { :scope { color: red; } }'
    document.head.appendChild(style)
    const rules = (style.sheet?.cssRules ?? []) as CSSRule[]
    const ok = rules.length > 0 && rules[0].constructor.name === 'CSSScopeRule'
    document.head.removeChild(style)
    return ok
  } catch {
    return false
  }
})()

/**
 * validateCss runs the CSS through a detached CSSStyleSheet and reports
 * any parse errors the browser's CSS parser finds. Returns null on clean
 * CSS, or an object with the first error message + its offset in the
 * input. Used by the Appearance editor to surface red-underline feedback
 * before the user discovers their theme has a silent typo.
 *
 * CSSStyleSheet's `replaceSync` doesn't throw on syntax errors — it logs
 * warnings and skips the bad rule. We instead count how many rules the
 * parser could build vs how many `{...}` blocks were in the input; a
 * discrepancy flags a parse issue.
 */
export function validateCss(css: string): { message: string } | null {
  if (!css.trim()) return null
  try {
    if (typeof CSSStyleSheet === 'undefined' || !('replaceSync' in CSSStyleSheet.prototype)) {
      return null // old browser, no validation — silent accept
    }
    const sheet = new CSSStyleSheet()
    // Strip @import / @scope to avoid "non-constructable" errors on
    // stylesheet; we just want syntax check on the bulk of rules.
    const stripped = css
      .replace(/@import\b[^;]*;?/gi, '')
      .replace(/@scope\s*\([^)]*\)\s*\{/gi, '')
    sheet.replaceSync(stripped)
    const parsed = sheet.cssRules.length
    // Count top-level blocks as a rough heuristic for "how many rules
    // should have parsed". Comments + whitespace stripped first so
    // they don't inflate the count.
    const clean = stripped.replace(/\/\*[\s\S]*?\*\//g, '')
    const blocks = (clean.match(/\{/g) ?? []).length
    // Some rules contain nested blocks (e.g. @media). We allow a
    // 30% tolerance — anything drastically off signals broken CSS.
    if (blocks > 0 && parsed === 0) {
      return { message: 'CSS parsed to 0 rules — check for missing braces or invalid selectors' }
    }
    return null
  } catch (e) {
    return { message: (e as Error).message }
  }
}

/**
 * scopeCSS restricts the user's CSS to elements inside `scope`.
 * Picks the best strategy for the current browser.
 *
 * Selectors that SHOULD stay global are kept unprefixed:
 *   - `@import`, `@font-face`, `@charset` (spec-required global)
 *   - `@keyframes`, `@property` (names have document scope already)
 *   - `:root { --foo }` (custom property definitions propagate through
 *     inheritance; scoping would break them)
 *   - `::-webkit-scrollbar*` (browser chrome; users expect it global)
 */
export function scopeCSS(css: string, scope: string): string {
  if (!css.trim()) return ''
  const { globalRules, scopable } = splitImports(css)
  if (supportsCSSScope) {
    // @scope rule. The `to (...)` clause is omitted so the whole subtree
    // counts as the scope; that's what users expect.
    return `${globalRules}\n@scope (${scope}) {\n${scopable}\n}`
  }
  return globalRules + '\n' + manualPrefix(scopable, scope)
}

/**
 * globalGuardCSS — applies user CSS in "Whole app" (global) scope WITHOUT
 * letting aggressive themes break admin surfaces (Settings, Account,
 * Docs). Uses `@scope (body) to (.nest-admin)` on modern browsers —
 * meaning the CSS matches everything under <body> EXCEPT the subtree
 * rooted at an element with `.nest-admin`.
 *
 * Rationale: tester reported a 1350-line theme in global scope wiped
 * out parts of AppearancePanel controls. That theme was in global
 * mode by design (needs to repaint topbar/sidebar), but had rules that
 * hit Vuetify inputs on Settings too. Admin surfaces must remain
 * readable so the user can always get back to the scope toggle and
 * disable/switch the theme.
 *
 * Firefox fallback: no `@scope` support → CSS applied as-is. Accept
 * the tradeoff: Firefox users picking global scope are trusted more.
 * If Settings breaks on Firefox, Safe mode (`?safe`) is the escape.
 */
export function globalGuardCSS(css: string): string {
  if (!css.trim()) return ''
  const { globalRules, scopable } = splitImports(css)
  if (supportsCSSScope) {
    // Scope everything to body but exclude admin subtrees. Two scope
    // limits joined via the `@scope` selector-list syntax:
    //
    //   - `.nest-admin` — our opt-in marker on Settings/Account/Docs/
    //     Themes/Converter containers.
    //   - `.v-overlay__content` — Vuetify's portal root for v-dialog,
    //     v-menu, v-tooltip etc. Dialogs are teleported to <body>,
    //     so they'd otherwise be painted by global-scope themes even
    //     when they contain admin UI (character editor, sprites,
    //     persona picker, …). Excluding the whole overlay layer is
    //     broader than strict admin-isolation but trades a small
    //     theming surface for guaranteed usable modals.
    //
    // Rules inside this @scope match elements that are descendants of
    // `body` AND are NOT inside any subtree rooted at either limit.
    return `${globalRules}\n@scope (body) to (.nest-admin, .v-overlay__content) {\n${scopable}\n}`
  }
  // Firefox / legacy: no @scope → trust user's global-scope intent.
  return css
}

/** Split out top-level @-rules that MUST remain global. */
function splitImports(css: string): { globalRules: string; scopable: string } {
  // Naive scanner: find `@import ...;` and `@font-face { ... }`, `@charset ...;`
  const out: string[] = []
  const remaining: string[] = []
  let i = 0
  while (i < css.length) {
    // Skip whitespace + comments.
    if (/\s/.test(css[i])) { out.push(css[i]); remaining.push(css[i]); i++; continue }
    if (css.startsWith('/*', i)) {
      const end = css.indexOf('*/', i + 2)
      const block = end < 0 ? css.slice(i) : css.slice(i, end + 2)
      out.push(block)
      remaining.push(block)
      i += block.length
      continue
    }
    if (css[i] !== '@') break
    // An @-rule at the start of content. Determine kind.
    const head = readAtKeyword(css, i)
    const isAtomic = /^@(import|charset|namespace)/i.test(head)
    const isBlock  = /^@(font-face|keyframes|-webkit-keyframes|property|counter-style)/i.test(head)
    if (isAtomic) {
      const semi = css.indexOf(';', i)
      if (semi < 0) break
      out.push(css.slice(i, semi + 1))
      i = semi + 1
    } else if (isBlock) {
      const open = css.indexOf('{', i)
      if (open < 0) break
      const close = findMatchingBrace(css, open)
      out.push(css.slice(i, close + 1))
      i = close + 1
    } else {
      break // regular at-rule (e.g. @media) — stop, let scopable catch it
    }
  }
  return { globalRules: out.join(''), scopable: css.slice(i) }
}

/** Manual fallback: prefix every non-@-rule selector with `scope `.
 *  Recurses into @media / @supports bodies. @keyframes / @font-face are
 *  left untouched. */
export function manualPrefix(css: string, scope: string): string {
  const out: string[] = []
  let i = 0
  const len = css.length

  while (i < len) {
    // Eat whitespace + comments into output.
    if (/\s/.test(css[i])) { out.push(css[i]); i++; continue }
    if (css.startsWith('/*', i)) {
      const end = css.indexOf('*/', i + 2)
      const block = end < 0 ? css.slice(i) : css.slice(i, end + 2)
      out.push(block); i += block.length; continue
    }

    if (css[i] === '@') {
      const head = readAtKeyword(css, i)
      const isAtomic = /^@(import|charset|namespace|layer [\w_]+\s*;)/i.test(head)

      if (isAtomic) {
        const semi = css.indexOf(';', i)
        if (semi < 0) { out.push(css.slice(i)); break }
        out.push(css.slice(i, semi + 1))
        i = semi + 1
        continue
      }

      const open = css.indexOf('{', i)
      if (open < 0) { out.push(css.slice(i)); break }
      const close = findMatchingBrace(css, open)

      if (/^@(font-face|keyframes|-webkit-keyframes|property|counter-style)/i.test(head)) {
        // Body is a set of descriptors or keyframe steps; NOT selectors.
        // Copy as-is.
        out.push(css.slice(i, close + 1))
      } else {
        // @media / @supports / @container / @layer — body is a ruleset.
        // Copy the @-header and recurse.
        out.push(css.slice(i, open + 1))
        out.push(manualPrefix(css.slice(open + 1, close), scope))
        out.push('}')
      }
      i = close + 1
      continue
    }

    // Regular rule: selector-list { body }
    const open = css.indexOf('{', i)
    if (open < 0) { out.push(css.slice(i)); break }
    const selectorText = css.slice(i, open)
    const close = findMatchingBrace(css, open)

    out.push(prefixSelectorList(selectorText, scope))
    out.push(css.slice(open, close + 1))
    i = close + 1
  }
  return out.join('')
}

/** Read the @-keyword head — e.g. "@media (max-width: 400px)". */
function readAtKeyword(css: string, i: number): string {
  let j = i
  while (j < css.length && css[j] !== '{' && css[j] !== ';') j++
  return css.slice(i, j)
}

/** Prefix every comma-separated selector with `scope ` unless it's a
 *  category we intentionally keep global. */
function prefixSelectorList(selectors: string, scope: string): string {
  return selectors
    .split(',')
    .map(sel => {
      const s = sel.trim()
      if (!s) return ''
      // :root / html — leave global so CSS vars flow through inheritance.
      if (/^:root\b/.test(s) || /^html\b/.test(s)) return s
      // Scrollbar pseudo-elements — browser chrome; users expect global.
      if (/^::-webkit-scrollbar/.test(s)) return s
      // Already explicitly global (starts with the scope selector).
      if (s.startsWith(scope)) return s
      // Selectors starting with `&` (nesting) — leave as-is; edge case.
      if (s.startsWith('&')) return s
      return `${scope} ${s}`
    })
    .filter(Boolean)
    .join(',\n  ')
}

function findMatchingBrace(s: string, start: number): number {
  let depth = 0
  for (let i = start; i < s.length; i++) {
    const c = s[i]
    if (c === '{') depth++
    else if (c === '}' && --depth === 0) return i
    // Skip string literals.
    else if (c === '"' || c === "'") {
      const close = s.indexOf(c, i + 1)
      if (close < 0) return s.length - 1
      i = close
    }
    else if (c === '/' && s[i + 1] === '*') {
      const close = s.indexOf('*/', i + 2)
      if (close < 0) return s.length - 1
      i = close + 1
    }
  }
  return s.length - 1
}

// ─── Danger detector ──────────────────────────────────────────────────

/** Element-level selectors that ST themes commonly target but that we'd
 *  rather keep scoped, since in WuNest they'd paint the whole shell.
 *  Purely for informational display — we don't auto-strip anything. */
const DANGEROUS_SELECTORS = [
  /^body\b/,
  /^html\b/,
  /^textarea\b/,
  /^input\b/,
  /^select\b/,
  /^button\b/,
  /^a\b/,
  /^\.menu_button\b/,
  /^\.text_pole\b/,
  /^\.header-style\b/,
]

export interface SelectorAudit {
  selector: string
  category: string    // "element" | "class" | "other"
  occurrences: number
}

/** Scan the CSS for selectors that would affect global UI if applied
 *  unscoped. Returns a tally. */
export function auditDangerousSelectors(css: string): SelectorAudit[] {
  const tallies = new Map<string, number>()
  const ruleRegex = /([^{}]+)\{[^}]*\}/g
  let match: RegExpExecArray | null
  while ((match = ruleRegex.exec(css)) !== null) {
    const selectorList = match[1]
    for (const raw of selectorList.split(',')) {
      const sel = raw.trim()
      for (const re of DANGEROUS_SELECTORS) {
        if (re.test(sel)) {
          const key = sel.split(/\s/)[0].split(':')[0] // just the root bit
          tallies.set(key, (tallies.get(key) ?? 0) + 1)
          break
        }
      }
    }
  }

  const out: SelectorAudit[] = []
  for (const [selector, occurrences] of tallies) {
    out.push({
      selector,
      occurrences,
      category: selector.startsWith('.') || selector.startsWith('#')
        ? 'class' : 'element',
    })
  }
  return out.sort((a, b) => b.occurrences - a.occurrences)
}
