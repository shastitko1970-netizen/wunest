<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import MarkdownIt from 'markdown-it'
import DOMPurify from 'dompurify'
import JsonPlate from './JsonPlate.vue'
import { splitContent, type Part } from '@/lib/splitContent'
import { useAppearanceStore } from '@/stores/appearance'

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

// DOMPurify config: permit common formatting tags and data-* / style / class
// attrs, strip every JS handler. Hyperlinks get rel=noopener+target=_blank.
const DP_CFG = {
  ALLOWED_TAGS: [
    'b', 'strong', 'i', 'em', 'u', 's', 'del', 'ins', 'mark', 'small', 'sub', 'sup',
    'br', 'p', 'span', 'div', 'blockquote', 'q', 'cite',
    'h1', 'h2', 'h3', 'h4', 'h5', 'h6',
    'ul', 'ol', 'li', 'dl', 'dt', 'dd',
    'a', 'img',
    'code', 'pre', 'kbd', 'samp', 'var',
    'table', 'thead', 'tbody', 'tr', 'th', 'td',
    'hr', 'abbr', 'details', 'summary',
    'figure', 'figcaption',
  ],
  ALLOWED_ATTR: ['href', 'title', 'alt', 'src', 'class', 'style', 'id', 'lang', 'dir', 'colspan', 'rowspan'],
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
</script>

<template>
  <div class="nest-message-content">
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
  :deep(hr) {
    border: none;
    border-top: 1px dashed var(--nest-border);
    margin: 12px 0;
  }
}
</style>
