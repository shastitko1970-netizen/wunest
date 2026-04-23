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
const md = computed(() => new MarkdownIt({
  html: app.value.htmlRendering !== false,
  breaks: true,
  linkify: true,
  typographer: false,
}))

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

function renderMd(body: string): string {
  const raw = md.value.render(body)
  // `sanitize` returns TrustedHTML on browsers with TT enabled; the string
  // form works for our v-html injection either way.
  return String(DOMPurify.sanitize(raw, DP_CFG))
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
  :deep(p) { margin: 0 0 8px; line-height: 1.55; }
  :deep(p:last-child) { margin-bottom: 0; }
  :deep(strong) { color: var(--nest-text); font-weight: 600; }
  :deep(em) { color: var(--nest-text-secondary); font-style: italic; }
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
    font-size: 12.5px;
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
  :deep(h1) { font-size: 20px; }
  :deep(h2) { font-size: 18px; }
  :deep(h3) { font-size: 16px; }
  :deep(h4) { font-size: 14px; text-transform: uppercase; letter-spacing: 0.05em; color: var(--nest-text-muted); }
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
    font-size: 12.5px;
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
    font-size: 12.5px;
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
}
</style>
