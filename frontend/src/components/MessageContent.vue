<script setup lang="ts">
import { computed, ref } from 'vue'
import { storeToRefs } from 'pinia'
import MarkdownIt from 'markdown-it'
import DOMPurify from 'dompurify'
import JsonPlate from './JsonPlate.vue'
import { splitContent, type Part } from '@/lib/splitContent'
import { useAppearanceStore } from '@/stores/appearance'
import { dispatchAction, applyDomUpdate, type PlateAction } from '@/lib/plateActions'

// MessageContent renders a single assistant/user message, with:
//   - Markdown (bold, italic, lists, links, blockquotes)
//   - Fenced code blocks with language tags
//   - Structured JSON plates: ```json {...} ``` and bare {...} blocks are
//     pulled out and rendered as key/value cards — the roleplay community
//     uses these as stat blocks, status panels, etc.
//   - Optional inline HTML (appearance.htmlRendering) — sanitized with
//     DOMPurify so the model can't paint a phishing popup.
//
// The splitter lives in lib/ so it's unit-testable without touching Vue.

const props = defineProps<{
  content: string
  // When true, suppress plate rendering (useful for preview in the edit box).
  raw?: boolean
}>()

/**
 * Events the content can raise on behalf of author-supplied plate
 * buttons. MessageBubble listens and routes to the chats store.
 * `action` is the generic channel; payload carries the specific ask.
 *
 * `toast` surfaces a brief confirmation for copy/dice/etc. Forwarded
 * up so the Chat.vue snackbar layer displays it.
 */
const emit = defineEmits<{
  (e: 'bubble-action', name: 'swipe-prev' | 'swipe-next' | 'regenerate' | 'edit' | 'delete'): void
  (e: 'draft', text: string, send: boolean): void
  (e: 'toast', level: 'info' | 'success' | 'error', text: string): void
}>()

const appearance = useAppearanceStore()
const { appearance: app } = storeToRefs(appearance)

// html: true lets raw <div>/<span> flow through markdown-it. We still route
// every output through DOMPurify before injecting, so the model can't ship
// <script> or event handlers. On by default — parity with SillyTavern.
const md = computed(() => {
  const inst = new MarkdownIt({
    html: app.value.htmlRendering !== false,
    breaks: true,
    linkify: true,
    typographer: false,
  })
  // Kill indent-level code blocks (4-space / tab indent). ST character
  // plates routinely indent their nested HTML for readability — markdown
  // would otherwise see `    <div …>` and wrap the whole thing in
  // <pre><code>, turning a coloured status card into escaped source on
  // the screen. Users who actually want code still have fenced blocks
  // (``` … ```).
  inst.disable('code')
  return inst
})

const parts = computed<Part[]>(() => splitContent(props.content ?? '', !!props.raw))

// DOMPurify config. The goal is "ST-themes should render as authored,
// minus anything that runs code". So we allow the full structural/semantic
// HTML a character-plate author typically reaches for (nav, aside, details,
// progress, inline SVG) and widen the attribute allowlist to cover ARIA,
// data-*, and media attrs, but KEEP all on* handlers and javascript: URIs
// out. A determined author can't paint a phishing popup or exfiltrate
// anything from a sandboxed <v-html /> fragment.
const DP_CFG = {
  ALLOWED_TAGS: [
    // Text-level formatting
    'b', 'strong', 'i', 'em', 'u', 's', 'del', 'ins', 'mark', 'small', 'sub', 'sup',
    'br', 'p', 'span', 'div', 'blockquote', 'q', 'cite', 'time', 'wbr',
    // Headings
    'h1', 'h2', 'h3', 'h4', 'h5', 'h6',
    // Lists
    'ul', 'ol', 'li', 'dl', 'dt', 'dd',
    // Media / inline
    'a', 'img', 'picture', 'source', 'video', 'audio', 'track',
    // Code / technical
    'code', 'pre', 'kbd', 'samp', 'var',
    // Tables
    'table', 'caption', 'colgroup', 'col', 'thead', 'tbody', 'tfoot', 'tr', 'th', 'td',
    // Block / layout
    'hr', 'abbr', 'details', 'summary', 'figure', 'figcaption',
    'section', 'article', 'header', 'footer', 'nav', 'aside', 'main',
    // Interactive-looking (clicks still inert — no onclick allowed)
    'button', 'progress', 'meter', 'fieldset', 'legend', 'label',
    // Inline SVG — common in character stat-block icons
    'svg', 'path', 'circle', 'rect', 'line', 'polyline', 'polygon',
    'ellipse', 'g', 'defs', 'use', 'symbol', 'title', 'desc', 'text', 'tspan',
    'linearGradient', 'radialGradient', 'stop', 'filter', 'feGaussianBlur',
    'feOffset', 'feMerge', 'feMergeNode', 'clipPath',
    // <style> is allowed — ST character cards routinely bundle CSS with
    // their plate HTML. We rewrite selectors to be message-scoped in a
    // post-sanitize pass (scopeStyleBlocks below) so a character's stylesheet
    // can't reach outside its own bubble.
    'style',
  ],
  ALLOWED_ATTR: [
    // Structural
    'href', 'title', 'alt', 'src', 'srcset', 'sizes', 'class', 'style', 'id',
    'lang', 'dir', 'type', 'name', 'value', 'placeholder',
    // Tables
    'colspan', 'rowspan', 'scope', 'headers',
    // Media
    'controls', 'loop', 'muted', 'poster', 'preload', 'autoplay',
    'width', 'height', 'crossorigin', 'referrerpolicy', 'loading', 'decoding',
    // Form-ish (decorative only — no JS = inert clicks)
    'disabled', 'checked', 'readonly', 'for', 'form', 'min', 'max', 'step',
    'pattern', 'multiple', 'selected', 'open',
    // Accessibility / semantics
    'role', 'tabindex', 'target', 'rel', 'hreflang', 'download',
    // SVG
    'viewBox', 'xmlns', 'fill', 'stroke', 'stroke-width', 'stroke-linecap',
    'stroke-linejoin', 'stroke-dasharray', 'stroke-dashoffset', 'opacity',
    'd', 'cx', 'cy', 'r', 'rx', 'ry', 'x', 'y', 'x1', 'y1', 'x2', 'y2',
    'points', 'transform', 'fill-opacity', 'stroke-opacity', 'stop-color',
    'stop-opacity', 'offset', 'gradientUnits', 'gradientTransform',
    'spreadMethod', 'text-anchor', 'dy', 'dx', 'font-family', 'font-size',
    'font-weight',
  ],
  // Accept `data-*` and `aria-*` wholesale via regex — both prefixes are
  // inherently safe (no code execution) and ST themes use them heavily for
  // state and accessibility.
  ALLOW_DATA_ATTR: true,
  ALLOW_ARIA_ATTR: true,
  // Don't allow javascript: / vbscript: URIs anywhere.
  ALLOWED_URI_REGEXP: /^(?:(?:https?|mailto|tel|data:image\/(?:png|jpeg|gif|webp|svg\+xml)):|[^a-z]|[a-z+.\-]+(?:[^a-z+.\-:]|$))/i,
}

// Make external links safe by default.
DOMPurify.addHook('afterSanitizeAttributes', (node) => {
  if (node.tagName === 'A' && node.getAttribute('href')) {
    node.setAttribute('rel', 'noopener noreferrer')
    node.setAttribute('target', '_blank')
  }
})

// Strip author-supplied inline styles that would escape the bubble's
// containment. This fires for EVERY style="..." attribute that reaches
// DOMPurify, so a character card can't ship a plate with
// `position: fixed; top: 0; width: 100vw;` and paint the whole viewport.
// The same predicate is reused to filter declarations inside <style>
// blocks (see sanitiseCssRuleBody below) — belt and suspenders.
DOMPurify.addHook('uponSanitizeAttribute', (_node, data) => {
  if (data.attrName === 'style' && typeof data.attrValue === 'string') {
    data.attrValue = filterInlineStyle(data.attrValue)
    // If every declaration got stripped there's nothing left — remove the
    // attribute entirely to keep the DOM clean.
    if (!data.attrValue.trim()) {
      data.keepAttr = false
    }
  }
})

// filterInlineStyle splits a `style=""` value into declarations, drops the
// ones that match a disallow-list, rejoins. Single-pass, permissive by
// default: we only strip what we explicitly recognise as dangerous. Authors
// keep full control over colour, typography, spacing, borders, animations.
function filterInlineStyle(style: string): string {
  return style
    .split(';')
    .map(d => d.trim())
    .filter(d => d && !isBadDeclaration(d))
    .join('; ')
}

// isBadDeclaration reports whether a single CSS declaration (property: value,
// no trailing semicolon) is one we refuse to honour. Keeps the author's
// colours, fonts, borders etc — only blocks the shapes that let content
// break out of its bubble:
//
//   - position: fixed / sticky / absolute — escapes the scrolling container,
//     paints over other UI.
//   - width/height in viewport units (vw/vh/svh/dvh/lvh) — lets a plate
//     claim the whole screen regardless of how narrow the chat column is.
//   - transform: scale(>2) — nuclear zoom.
//   - z-index > 100 — pushes above Vuetify overlays (dialog > 2400, tooltip
//     > 2000, nav-drawer > 1500 — 100 is well below all of those).
function isBadDeclaration(decl: string): boolean {
  const low = decl.toLowerCase()
  if (/^position\s*:\s*(fixed|sticky|absolute)\b/.test(low)) return true
  if (/^(?:width|height|min-width|min-height|max-width|max-height)\s*:.*\b\d+\s*(?:vw|vh|svw|svh|dvw|dvh|lvw|lvh)\b/.test(low)) return true
  if (/^transform\s*:.*\bscale\s*\(\s*[3-9]/.test(low)) return true
  if (/^inset\s*:/.test(low)) return true  // pairs with position:fixed; block defensively
  const z = /^z-index\s*:\s*(-?\d+)/.exec(low)
  if (z && parseInt(z[1], 10) > 100) return true
  return false
}

// sanitiseCssRuleBody — same predicate applied to a CSS declaration block
// from inside a <style> tag. Used by serializeRule below.
function sanitiseCssRuleBody(style: CSSStyleDeclaration): string {
  const parts: string[] = []
  for (const prop of Array.from(style)) {
    const value = style.getPropertyValue(prop)
    const priority = style.getPropertyPriority(prop)
    const decl = `${prop}: ${value}${priority ? ' !' + priority : ''}`
    if (isBadDeclaration(decl)) continue
    parts.push(decl)
  }
  return parts.join('; ')
}

// Unique scope for this message instance. Every <style> block inside this
// bubble gets its selectors prefixed with the matching data attribute, so
// character-authored CSS stays inside its own message bubble and never
// leaks to the rest of the app.
const scopeId = `m${Math.random().toString(36).slice(2, 10)}`
const scopeSel = `[data-nest-msg-scope="${scopeId}"]`

function renderMd(body: string): string {
  const raw = md.value.render(body)
  const purified = String(DOMPurify.sanitize(raw, DP_CFG))
  return scopeStyleBlocks(purified, scopeSel)
}

// scopeStyleBlocks finds every <style> element in a fragment of HTML and
// rewrites its CSS so selectors only match elements inside this message's
// scope. Author can drop a <style> in their character card and have it
// actually apply, without bleeding onto the app shell.
//
// Returns the original HTML unchanged when there are no style tags, the
// fast path for 99% of messages.
function scopeStyleBlocks(html: string, scope: string): string {
  if (!html.includes('<style')) return html
  // Parse the fragment into a detached container so we can manipulate
  // <style> element text content in isolation.
  const tmp = document.createElement('div')
  tmp.innerHTML = html
  const styles = tmp.querySelectorAll('style')
  for (const style of Array.from(styles)) {
    const scoped = scopeCssText(style.textContent ?? '', scope)
    if (scoped === null) {
      // Malformed CSS — drop the <style> rather than pollute the page.
      style.remove()
      continue
    }
    style.textContent = scoped
  }
  return tmp.innerHTML
}

// scopeCssText parses a CSS string via a constructable stylesheet (offline,
// no document effect), walks the rules, and rewrites each selector with a
// scope prefix. Returns null when parsing fails outright.
//
// Handles nested at-rules (@media / @supports) by recursing. @import is
// dropped to prevent remote CSS fetches. @keyframes / @font-face / etc
// pass through as-is.
function scopeCssText(css: string, scope: string): string | null {
  // Pre-strip @import — constructable stylesheets ignore them anyway but
  // being explicit documents intent.
  css = css.replace(/@import\b[^;]*;?/gi, '')
  try {
    // Some iOS Safari versions still ship without `CSSStyleSheet` ctor;
    // fall back to a detached <style> tag attached to an inert document.
    let sheet: CSSStyleSheet
    if (typeof CSSStyleSheet !== 'undefined' && 'replaceSync' in CSSStyleSheet.prototype) {
      sheet = new CSSStyleSheet()
      sheet.replaceSync(css)
    } else {
      const s = document.implementation.createHTMLDocument('').createElement('style')
      s.textContent = css
      document.implementation.createHTMLDocument('').head.appendChild(s)
      sheet = s.sheet as CSSStyleSheet
    }
    const rules = Array.from(sheet.cssRules)
    return rules.map(r => serializeRule(r, scope)).join('\n')
  } catch {
    return null
  }
}

function serializeRule(rule: CSSRule, scope: string): string {
  // Plain style rule: prefix every comma-separated selector with scope
  // AND filter the declaration block so dangerous properties (position:
  // fixed, viewport units, huge z-index) from authored <style> blocks
  // can't escape the bubble any more easily than the inline-style path.
  if (rule instanceof CSSStyleRule) {
    const selectors = rule.selectorText
      .split(',')
      .map(s => scopedSelector(s.trim(), scope))
      .filter(s => s)
      .join(', ')
    const body = sanitiseCssRuleBody(rule.style)
    if (!body) return ''  // whole rule was dangerous — drop it
    return `${selectors} { ${body} }`
  }
  // Grouping rules — @media, @supports. Recurse the inner rules.
  const groupRule = rule as CSSRule & { cssRules?: CSSRuleList; conditionText?: string; name?: string }
  if (typeof CSSMediaRule !== 'undefined' && rule instanceof CSSMediaRule) {
    const inner = Array.from(rule.cssRules).map(r => serializeRule(r, scope)).join('\n')
    return `@media ${rule.conditionText} { ${inner} }`
  }
  if (typeof CSSSupportsRule !== 'undefined' && rule instanceof CSSSupportsRule) {
    const inner = Array.from(rule.cssRules).map(r => serializeRule(r, scope)).join('\n')
    return `@supports ${rule.conditionText} { ${inner} }`
  }
  // @keyframes / @font-face / any other — no inner selectors to scope.
  return groupRule.cssText ?? ''
}

// scopedSelector prepends the scope to a selector unless it targets :root,
// html, or body — those would either never match (root is outside our tree)
// or intentionally reach up to the chat shell (html/body) which we forbid.
//
// Doesn't double-scope if the selector already contains the scope (author
// manually scoped already — respect their intent).
function scopedSelector(sel: string, scope: string): string {
  if (!sel) return ''
  if (sel.includes(scope)) return sel
  // Strip anything that would escape the scope. Authors sometimes write
  // `:root { --accent: red; }` or `body.dark { ... }`; both are too global.
  if (/^(?::root\b|html\b|body\b)/i.test(sel)) return `${scope} ${sel.replace(/^(?::root|html|body)\S*/i, '').trim() || '*'}`
  return `${scope} ${sel}`
}

// ─── Plate action bridge (M32 interactive plates) ────────────────
//
// Author-supplied HTML can include `<button data-nest-action="...">`
// which we wire up via a single delegated click listener on the
// content root. dispatchAction() walks the DOM, looks the name up in
// the whitelist, and returns a structured PlateAction we dispatch
// based on kind:
//   - 'toast' → emit('toast') → Chat.vue shows a snackbar
//   - 'bubble' → emit('bubble-action') → MessageBubble invokes the
//     corresponding message action (swipe, regenerate)
//   - 'draft' → emit('draft') → Chat.vue fills composer + optionally sends
//   - 'dom-update' → mutate the same author-controlled subtree
//     (safe — no privileged data there)
//
// Event delegation means we attach ONE listener per message instead
// of one per button, which matters for long stat-block plates.
const contentRoot = ref<HTMLElement | null>(null)

function onContentClick(event: MouseEvent) {
  const result = dispatchAction(event, contentRoot.value)
  if (!result) return
  if (result.prevent) event.preventDefault()
  runAction(result.action)
}

function runAction(action: PlateAction) {
  switch (action.kind) {
    case 'toast':
      emit('toast', action.level, action.text)
      break
    case 'bubble':
      emit('bubble-action', action.name)
      break
    case 'draft':
      emit('draft', action.text, action.send ?? false)
      break
    case 'dom-update':
      applyDomUpdate(action.update, action.target)
      break
    case 'noop':
      break
  }
}
</script>

<template>
  <div
    ref="contentRoot"
    class="nest-message-content"
    @click="onContentClick"
  >
    <template v-for="(part, i) in parts" :key="i">
      <JsonPlate
        v-if="part.kind === 'plate'"
        :data="part.body"
        :label="part.label"
      />
      <div
        v-else
        class="nest-md"
        :data-nest-msg-scope="scopeId"
        v-html="renderMd(part.body)"
      />
    </template>
  </div>
</template>

<style lang="scss" scoped>
.nest-message-content {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.nest-md {
  // Layout+style containment on every message bubble. Authored CSS
  // inside can't affect anything outside this box (layout calculations,
  // counters, container queries) — paired with the scoped <style>
  // rewrite above, this is the second line of defence against
  // "a plate that ate the whole app" bugs.
  contain: layout style;
  max-width: 100%;
  // Block-level children shouldn't be able to overflow the text column
  // via absurd inline widths ("width: 1500px") — cap them. `!important`
  // because authors love inline styles and we need to win.
  :deep(div), :deep(section), :deep(article), :deep(aside),
  :deep(header), :deep(footer), :deep(nav), :deep(main),
  :deep(blockquote), :deep(figure) {
    max-width: 100% !important;
    box-sizing: border-box;
  }
  // A runaway height (`height: 2000px`) on a plate makes the chat jump
  // past the composer into the void. Cap every author-set height to the
  // viewport; if they wanted a full scene, they get a scrollable column.
  :deep(div[style*="height"]),
  :deep(section[style*="height"]),
  :deep(header[style*="height"]),
  :deep(article[style*="height"]) {
    max-height: 80vh;
    overflow: auto;
  }

  :deep(p) { margin: 0 0 8px; line-height: 1.55; }
  :deep(p:last-child) { margin-bottom: 0; }
  // strong/em intentionally DON'T override color — inherit from the
  // enclosing context so author-coloured plaques ("status: red" etc)
  // keep their colour through bolded words. Distinction comes from
  // weight/style alone, which is enough visual signal without fighting
  // the author's palette.
  :deep(strong), :deep(b) { font-weight: 600; color: inherit; }
  :deep(em), :deep(i)     { font-style: italic; color: inherit; }
  :deep(u) { text-decoration: underline; text-underline-offset: 2px; }
  :deep(s), :deep(del), :deep(strike) { text-decoration: line-through; opacity: 0.7; }
  :deep(mark) {
    background: color-mix(in srgb, var(--nest-accent) 30%, transparent);
    color: inherit;
    padding: 0 2px;
    border-radius: 2px;
  }
  :deep(small) { font-size: 0.88em; opacity: 0.85; }
  :deep(sub), :deep(sup) { font-size: 0.75em; }
  :deep(blockquote) {
    margin: 6px 0;
    padding: 4px 10px;
    border-left: 3px solid var(--nest-accent);
    color: var(--nest-text-secondary);
    background: rgba(0, 0, 0, 0.08);
    border-radius: 4px;
  }
  :deep(code) {
    background: rgba(255, 255, 255, 0.08);
    padding: 1px 5px;
    border-radius: 4px;
    font-family: var(--nest-font-mono);
    font-size: 0.9em;
  }
  :deep(pre) {
    background: var(--nest-bg);
    border: 1px solid var(--nest-border-subtle);
    border-radius: var(--nest-radius-sm);
    padding: 10px 12px;
    overflow-x: auto;
    font-family: var(--nest-font-mono);
    // em-relative to the message body so pre blocks scale with
    // fontScale (which lives on .nest-msg-body via calc()).
    font-size: 0.85em;
    line-height: 1.5;
    margin: 8px 0;
    code { background: transparent; padding: 0; }
  }
  :deep(a) {
    color: var(--nest-accent);
    text-decoration: none;
    &:hover { text-decoration: underline; }
  }
  :deep(ul), :deep(ol) { margin: 4px 0 8px; padding-left: 20px; }
  :deep(li) { margin: 2px 0; }
  :deep(h1), :deep(h2), :deep(h3), :deep(h4) {
    font-family: var(--nest-font-display);
    font-weight: 500;
    margin: 10px 0 6px;
    letter-spacing: -0.01em;
  }
  // em-relative headings — inherit the fontScale from .nest-msg-body
  // while keeping the original 20/18/16/14 hierarchy at scale=1.
  :deep(h1) { font-size: 1.33em; }
  :deep(h2) { font-size: 1.2em; }
  :deep(h3) { font-size: 1.07em; }
  :deep(h4) { font-size: 0.93em; text-transform: uppercase; letter-spacing: 0.05em; color: var(--nest-text-muted); }
  // Horizontal rule. Markdown `---` maps here. The previous 1px dashed
  // border on --nest-border was basically invisible on the dark chat
  // bubble — users complained they couldn't see their scene-breaks.
  // Now solid, 2px, with a gradient fade at the edges so it reads as
  // "section break" without being a heavy divider.
  :deep(hr) {
    border: none;
    height: 2px;
    margin: 16px 10%;
    background: linear-gradient(
      to right,
      transparent 0%,
      color-mix(in srgb, var(--nest-accent) 55%, transparent) 50%,
      transparent 100%
    );
    border-radius: 1px;
  }

  // ── Interactive / structural elements ──────────────────────────
  // Character plates often rely on native HTML semantics to look
  // right out-of-the-box. These rules give them a baseline style so
  // author CSS (class + inline style) has something sensible to
  // override — avoids the "raw UA defaults in the middle of a chat
  // bubble" look that prompted the complaint.

  // Collapsible panels (<details>/<summary>) — common for "scene
  // details", "inventory", stat cards.
  :deep(details) {
    margin: 8px 0;
    padding: 6px 10px;
    background: var(--nest-bg-elevated);
    border: 1px solid var(--nest-border-subtle);
    border-radius: var(--nest-radius-sm);
  }
  :deep(details[open]) {
    padding-bottom: 10px;
  }
  :deep(summary) {
    cursor: pointer;
    font-weight: 500;
    color: var(--nest-text);
    padding: 2px 0;
    list-style: none;
    outline: none;

    &::-webkit-details-marker { display: none; }
    &::before {
      content: '▸';
      display: inline-block;
      margin-right: 6px;
      transition: transform var(--nest-transition-fast);
      color: var(--nest-text-muted);
    }
  }
  :deep(details[open]) > :deep(summary)::before {
    transform: rotate(90deg);
  }

  // Progress bars / meters — simple plate vocab.
  :deep(progress), :deep(meter) {
    display: inline-block;
    width: 160px;
    height: 10px;
    vertical-align: middle;
    accent-color: var(--nest-accent);
  }

  // Buttons. Base style for decorative ones (no data-nest-action =
  // dormant, author-supplied onclick is stripped by DOMPurify so the
  // click goes nowhere — show that honestly via cursor:not-allowed).
  // Interactive buttons WITH a known `data-nest-action` get
  // cursor:pointer + a subtle hover/active state so users feel the
  // click respond. See frontend/src/lib/plateActions.ts for the
  // whitelist (copy / dice / reroll / swipe-prev / swipe-next /
  // regenerate / say / send / toggle-attr / toggle-class).
  :deep(button) {
    display: inline-block;
    padding: 4px 12px;
    margin: 2px;
    font: inherit;
    font-size: 0.85em;
    color: var(--nest-text);
    background: var(--nest-bg-elevated);
    border: 1px solid var(--nest-border);
    border-radius: var(--nest-radius-sm);
    cursor: not-allowed;
    opacity: 0.85;
    transition: border-color var(--nest-transition-fast), background var(--nest-transition-fast);
  }
  :deep(button[data-nest-action]),
  :deep([data-nest-action]) {
    cursor: pointer;
    opacity: 1;

    &:hover {
      border-color: var(--nest-accent);
      background: color-mix(in srgb, var(--nest-accent) 12%, var(--nest-bg-elevated));
    }
    &:active {
      transform: translateY(1px);
    }
  }

  // Tables — authors use these for stat blocks; make them readable
  // instead of UA-default crunched.
  :deep(table) {
    border-collapse: collapse;
    margin: 8px 0;
    font-size: 0.85em;
    max-width: 100%;
    display: block;
    overflow-x: auto;
  }
  :deep(th), :deep(td) {
    border: 1px solid var(--nest-border-subtle);
    padding: 4px 10px;
    text-align: left;
  }
  :deep(th) {
    background: var(--nest-bg-elevated);
    font-weight: 600;
    color: var(--nest-text);
  }

  // Generic layout tags used as plate containers — collapse the
  // default margins so plates look tight like the author intended.
  :deep(section), :deep(article), :deep(aside), :deep(nav),
  :deep(header), :deep(footer), :deep(main) {
    margin: 6px 0;
  }

  // Inline SVGs — keep them inline with text unless the author flexed
  // width/height via inline style or class.
  :deep(svg) {
    vertical-align: middle;
    max-width: 100%;
  }

  // Media — fit the bubble width with rounded corners.
  :deep(img), :deep(video) {
    max-width: 100%;
    border-radius: var(--nest-radius-sm);
  }
  :deep(video), :deep(audio) {
    display: block;
    margin: 6px 0;
  }

  // ── Baseline styling for ST / roleplay class conventions ──────────
  // Character cards from CHUB + mirrors use a narrow vocabulary of
  // class names to tag narrative text. Most authors DON'T ship CSS for
  // these (they assume the host renders them) — so we provide sensible
  // defaults here. Author <style> still wins thanks to source order
  // (author block gets scoped + appended to the DOM after this CSS).

  :deep(.quote), :deep(.q), :deep(q) {
    color: var(--nest-text-secondary);
    font-style: italic;
  }
  :deep(.thought), :deep(.think), :deep(.thinking),
  :deep(.inner-thoughts), :deep(.inner_thoughts) {
    font-style: italic;
    color: color-mix(in srgb, var(--nest-text) 70%, var(--nest-accent) 30%);
    opacity: 0.92;
  }
  :deep(.whisper), :deep(.whispered) {
    font-size: 0.92em;
    color: var(--nest-text-muted);
    font-style: italic;
  }
  :deep(.shout), :deep(.yelling) {
    text-transform: uppercase;
    font-weight: 600;
    letter-spacing: 0.02em;
  }
  :deep(.action), :deep(.narration), :deep(.narrate) {
    color: var(--nest-text-secondary);
    font-style: italic;
  }
  :deep(.speech), :deep(.dialogue), :deep(.said) {
    color: var(--nest-text);
    // No font-style — regular dialogue reads as normal text.
  }
  :deep(.ooc), :deep(.oocnote) {
    display: inline-block;
    padding: 1px 8px;
    margin: 2px 0;
    font-size: 0.88em;
    color: var(--nest-text-muted);
    background: var(--nest-bg-elevated);
    border-radius: var(--nest-radius-pill);
    font-style: italic;
  }
  :deep(.system), :deep(.systemmessage), :deep(.system-message) {
    background: rgba(0, 0, 0, 0.12);
    border-left: 2px solid var(--nest-border);
    padding: 4px 10px;
    margin: 4px 0;
    color: var(--nest-text-muted);
    font-size: 0.92em;
    border-radius: 0 var(--nest-radius-sm) var(--nest-radius-sm) 0;
  }
  // Info plaques — common name. Author CSS should customise; baseline
  // is a soft-bordered card so it doesn't look like raw text.
  :deep(.plate), :deep(.card), :deep(.info-box), :deep(.stat-block) {
    display: block;
    margin: 8px 0;
    padding: 8px 12px;
    background: var(--nest-bg-elevated);
    border: 1px solid var(--nest-border-subtle);
    border-radius: var(--nest-radius-sm);
  }
  // Status / HP / MP bar-like rows — common layout.
  :deep(.status), :deep(.stats), :deep(.statline) {
    font-family: var(--nest-font-mono);
    font-size: 0.92em;
    color: var(--nest-text-secondary);
  }

  // Author-supplied colour containers: once an author sets a `color` via
  // inline style, descendants should FULLY inherit (we already made
  // strong/em inherit globally above, but lists/links still have their
  // own rules — reset those here).
  :deep([style*="color"]) a { color: inherit; text-decoration: underline; }
  :deep([style*="color"]) code,
  :deep([style*="color"]) kbd,
  :deep([style*="color"]) samp { color: inherit; }
}
</style>
