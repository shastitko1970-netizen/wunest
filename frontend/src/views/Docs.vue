<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import MarkdownIt from 'markdown-it'
import DOMPurify from 'dompurify'
import {
  TOPICS,
  CATEGORY_ORDER,
  CATEGORY_LABEL,
  findTopic,
  type DocTopic,
} from '@/docs'

// Docs view. Public; renders markdown from src/docs/pages. Two modes:
//   - /docs       → index (ToC grouped by category)
//   - /docs/:slug → one topic with rendered content + next/prev nav
//
// Keeps its own small markdown-it instance (independent from
// MessageContent's) because docs have different safety requirements —
// no inline HTML, no DOMPurify on allow-list (but still run through
// sanitizer to be explicit).

const { t, locale } = useI18n()
const route = useRoute()

const md = new MarkdownIt({
  html: false,
  breaks: false,
  linkify: true,
  typographer: true,
})

/** Active topic — driven by `:slug` route param. */
const currentSlug = computed(() => route.params.slug as string | undefined)
const currentTopic = computed<DocTopic | null>(() =>
  currentSlug.value ? findTopic(currentSlug.value) : null,
)

/** Render-ready HTML for the current topic. */
const renderedHTML = computed(() => {
  if (!currentTopic.value) return ''
  const source = currentTopic.value.content[locale.value as 'ru' | 'en']
    ?? currentTopic.value.content.en
    ?? ''
  const html = md.render(source)
  // Paranoid: docs markdown is ours, but the sanitizer catches bugs where
  // we'd otherwise smuggle an admin-authored <script>.
  return DOMPurify.sanitize(html, {
    ADD_ATTR: ['target', 'rel'],
  })
})

/** ToC grouped by category, in CATEGORY_ORDER. */
const toc = computed(() => {
  const out: Array<{ cat: DocTopic['category']; label: string; items: DocTopic[] }> = []
  for (const cat of CATEGORY_ORDER) {
    const items = TOPICS.filter(t => t.category === cat)
    if (items.length === 0) continue
    out.push({
      cat,
      label: CATEGORY_LABEL[cat][locale.value as 'ru' | 'en'] ?? CATEGORY_LABEL[cat].en,
      items,
    })
  }
  return out
})

/** Sibling navigation — flat prev/next across all topics. */
const siblings = computed(() => {
  if (!currentTopic.value) return { prev: null, next: null }
  const flat = CATEGORY_ORDER.flatMap(cat => TOPICS.filter(t => t.category === cat))
  const idx = flat.findIndex(x => x.slug === currentTopic.value!.slug)
  return {
    prev: idx > 0 ? flat[idx - 1] : null,
    next: idx >= 0 && idx < flat.length - 1 ? flat[idx + 1] : null,
  }
})

function topicTitle(t: DocTopic) {
  return t.title[locale.value as 'ru' | 'en'] ?? t.title.en
}
function topicSummary(t: DocTopic) {
  return t.summary[locale.value as 'ru' | 'en'] ?? t.summary.en
}
</script>

<template>
  <div class="nest-docs nest-admin">
    <div class="nest-docs-layout">
      <!-- Left ToC -->
      <aside class="nest-docs-toc">
        <div class="nest-eyebrow">{{ t('docs.tocTitle') }}</div>
        <div
          v-for="group in toc"
          :key="group.cat"
          class="nest-docs-toc-group"
        >
          <div class="nest-docs-toc-group-label nest-mono">{{ group.label }}</div>
          <router-link
            v-for="topic in group.items"
            :key="topic.slug"
            :to="`/docs/${topic.slug}`"
            class="nest-docs-toc-item"
            :class="{ active: currentSlug === topic.slug }"
          >
            {{ topicTitle(topic) }}
          </router-link>
        </div>
      </aside>

      <!-- Right content -->
      <main class="nest-docs-main">
        <!-- Index -->
        <template v-if="!currentTopic">
          <h1 class="nest-h1">{{ t('docs.indexTitle') }}</h1>
          <p class="nest-subtitle nest-docs-lead">{{ t('docs.indexLead') }}</p>

          <section
            v-for="group in toc"
            :key="group.cat"
            class="nest-docs-cat"
          >
            <h2 class="nest-h3">{{ group.label }}</h2>
            <div class="nest-docs-cards">
              <router-link
                v-for="topic in group.items"
                :key="topic.slug"
                :to="`/docs/${topic.slug}`"
                class="nest-docs-card"
              >
                <div class="nest-docs-card-title">{{ topicTitle(topic) }}</div>
                <div class="nest-docs-card-summary">{{ topicSummary(topic) }}</div>
              </router-link>
            </div>
          </section>
        </template>

        <!-- Single topic -->
        <template v-else>
          <article class="nest-docs-article" v-html="renderedHTML" />

          <nav class="nest-docs-pager">
            <router-link
              v-if="siblings.prev"
              :to="`/docs/${siblings.prev.slug}`"
              class="nest-docs-pager-link prev"
            >
              <v-icon size="18" class="mr-1">mdi-chevron-left</v-icon>
              <span class="nest-docs-pager-meta">{{ t('docs.prev') }}</span>
              <span class="nest-docs-pager-title">{{ topicTitle(siblings.prev) }}</span>
            </router-link>
            <div v-else class="nest-docs-pager-spacer"></div>
            <router-link
              v-if="siblings.next"
              :to="`/docs/${siblings.next.slug}`"
              class="nest-docs-pager-link next"
            >
              <span class="nest-docs-pager-meta">{{ t('docs.next') }}</span>
              <span class="nest-docs-pager-title">{{ topicTitle(siblings.next) }}</span>
              <v-icon size="18" class="ml-1">mdi-chevron-right</v-icon>
            </router-link>
          </nav>
        </template>
      </main>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.nest-docs {
  // 100dvh — DS rule: mobile browser URL-bar reflow + IME keyboard.
  min-height: 100dvh;
  background: var(--nest-bg);
  color: var(--nest-text);
}

// ── Top strip ─────────────────────────────────────────────────
.nest-docs-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 24px;
  border-bottom: 1px solid var(--nest-border);
  background: var(--nest-bg);
  position: sticky;
  top: 0;
  z-index: 10;
}
.nest-docs-logo {
  all: unset;
  display: inline-flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
  font-family: var(--nest-font-display);
  font-size: 18px;
  color: var(--nest-text);
}
.nest-docs-logo-mark {
  width: 28px;
  height: 28px;
  display: grid;
  place-items: center;
  color: var(--nest-accent);
  font-size: 18px;
  font-weight: 600;
  border: 1px solid var(--nest-accent);
  // Token instead of hardcoded 6 — matches --nest-radius-sm (6px),
  // mod authors controlling the small-radius scale pick this up.
  border-radius: var(--nest-radius-sm);
  font-family: var(--nest-font-mono);
}
.nest-docs-top-nav {
  display: flex;
  gap: 4px;
  align-items: center;
}
.nest-docs-top-link {
  all: unset;
  padding: 6px 12px;
  font-size: 13.5px;
  color: var(--nest-text-secondary);
  border-radius: var(--nest-radius-sm);
  cursor: pointer;
  text-decoration: none;
  &:hover { color: var(--nest-text); background: var(--nest-bg-elevated); }
  &.active { color: var(--nest-text); box-shadow: inset 0 -2px 0 var(--nest-accent); }
  &.login { background: var(--nest-accent); color: #fff; padding: 6px 18px; &:hover { background: var(--nest-accent); filter: brightness(1.1); } }
}

// ── Layout ────────────────────────────────────────────────────
.nest-docs-layout {
  display: grid;
  grid-template-columns: 260px 1fr;
  max-width: 1100px;
  margin: 0 auto;
  gap: 40px;
  padding: 32px 24px 80px;
}

.nest-docs-toc {
  position: sticky;
  top: 80px;
  align-self: start;
  // 100dvh — TOC's max-height must shrink with the URL bar so the
  // list doesn't extend behind it on mobile Safari.
  max-height: calc(100dvh - 120px);
  overflow-y: auto;
}
.nest-docs-toc-group {
  margin-top: 14px;
  &:first-of-type { margin-top: 6px; }
}
.nest-docs-toc-group-label {
  font-size: 10px;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--nest-text-muted);
  margin-bottom: 4px;
}
.nest-docs-toc-item {
  display: block;
  padding: 6px 10px;
  font-size: 13px;
  color: var(--nest-text-secondary);
  border-radius: var(--nest-radius-sm);
  text-decoration: none;
  transition: background var(--nest-transition-fast), color var(--nest-transition-fast);
  &:hover { background: var(--nest-bg-elevated); color: var(--nest-text); }
  &.active {
    background: var(--nest-bg-elevated);
    color: var(--nest-text);
    box-shadow: inset 2px 0 0 var(--nest-accent);
  }
}

// ── Content ──────────────────────────────────────────────────
.nest-docs-main { min-width: 0; }
.nest-docs-lead { font-size: 1.1rem; margin: 12px 0 28px; max-width: 680px; }

.nest-docs-cat { margin-top: 24px; }
.nest-docs-cat h2 { margin-bottom: 12px; }
.nest-docs-cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 12px;
}
.nest-docs-card {
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius);
  padding: 16px;
  background: var(--nest-surface);
  text-decoration: none;
  transition: border-color var(--nest-transition-fast), transform var(--nest-transition-fast);
  &:hover { border-color: var(--nest-accent); transform: translateY(-1px); }
}
.nest-docs-card-title {
  font-family: var(--nest-font-display);
  font-size: 15px;
  color: var(--nest-text);
  margin-bottom: 4px;
}
.nest-docs-card-summary {
  font-size: 12.5px;
  color: var(--nest-text-secondary);
  line-height: 1.5;
}

// ── Article — full markdown styling ──────────────────────────
.nest-docs-article {
  font-size: 15px;
  line-height: 1.7;
  color: var(--nest-text);

  :deep(h1) {
    font-family: var(--nest-font-display);
    font-weight: 400;
    font-size: clamp(1.8rem, 3vw, 2.4rem);
    letter-spacing: -0.02em;
    line-height: 1.15;
    margin: 0 0 20px;
  }
  :deep(h2) {
    font-family: var(--nest-font-display);
    font-weight: 400;
    font-size: 1.5rem;
    letter-spacing: -0.01em;
    margin: 36px 0 12px;
    padding-bottom: 6px;
    border-bottom: 1px solid var(--nest-border-subtle);
  }
  :deep(h3) {
    font-family: var(--nest-font-display);
    font-weight: 500;
    font-size: 1.15rem;
    margin: 24px 0 10px;
  }
  :deep(p) { margin: 10px 0; }
  :deep(ul),
  :deep(ol) { margin: 10px 0; padding-left: 22px; }
  :deep(li) { margin: 4px 0; }
  :deep(strong) { color: var(--nest-text); font-weight: 600; }
  :deep(a) {
    color: var(--nest-accent);
    text-decoration: none;
    border-bottom: 1px dashed currentColor;
    &:hover { filter: brightness(1.2); }
  }
  :deep(code) {
    font-family: var(--nest-font-mono);
    font-size: 12.5px;
    padding: 2px 6px;
    background: var(--nest-bg-elevated);
    border: 1px solid var(--nest-border-subtle);
    border-radius: 3px;
  }
  :deep(pre) {
    margin: 14px 0;
    padding: 14px 16px;
    background: var(--nest-bg-elevated);
    border: 1px solid var(--nest-border-subtle);
    border-radius: var(--nest-radius-sm);
    overflow-x: auto;
    font-family: var(--nest-font-mono);
    font-size: 12.5px;
    line-height: 1.55;

    code {
      padding: 0;
      background: transparent;
      border: 0;
    }
  }
  :deep(table) {
    border-collapse: collapse;
    width: 100%;
    margin: 14px 0;
    font-size: 13.5px;
  }
  :deep(th),
  :deep(td) {
    border: 1px solid var(--nest-border-subtle);
    padding: 8px 12px;
    text-align: left;
  }
  :deep(th) {
    background: var(--nest-bg-elevated);
    font-weight: 500;
  }
  :deep(blockquote) {
    margin: 14px 0;
    padding: 2px 14px;
    border-left: 2px solid var(--nest-accent);
    color: var(--nest-text-secondary);
    font-style: italic;
  }
}

// ── Pager ────────────────────────────────────────────────────
.nest-docs-pager {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-top: 48px;
  padding-top: 24px;
  border-top: 1px solid var(--nest-border-subtle);
}
.nest-docs-pager-link {
  display: grid;
  grid-template-rows: auto auto;
  gap: 2px;
  padding: 14px 16px;
  background: var(--nest-surface);
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius);
  text-decoration: none;
  color: var(--nest-text);
  transition: border-color var(--nest-transition-fast);

  &:hover { border-color: var(--nest-accent); }
  &.next { text-align: right; }
}
.nest-docs-pager-meta {
  font-family: var(--nest-font-mono);
  font-size: 10.5px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--nest-text-muted);
}
.nest-docs-pager-title {
  font-size: 14px;
  font-weight: 500;
}
.nest-docs-pager-spacer { /* empty column filler */ }

// ── Mobile ───────────────────────────────────────────────────
// DS primary 960px: docs-layout TOC collapses to a topmost row.
// Previously 760 — unified to the DS-canonical primary breakpoint.
@media (max-width: 960px) {
  .nest-docs-layout {
    grid-template-columns: 1fr;
    gap: 24px;
    padding: 20px 16px 60px;
  }
  .nest-docs-toc {
    position: static;
    max-height: none;
    border-bottom: 1px solid var(--nest-border-subtle);
    padding-bottom: 20px;
  }
  .nest-docs-top { padding: 12px 16px; }
  .nest-docs-top-link { padding: 6px 8px; font-size: 13px; }
  .nest-docs-pager { grid-template-columns: 1fr; }
}
</style>
