<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { libraryApi, type LibraryResult, type SortOption } from '@/api/library'
import { useCharactersStore } from '@/stores/characters'

// Full-screen dialog that browses chub.ai. Debounces the query, infinite
// scrolls over pages, and imports a card with one click. The imported
// character lands in the user's library and is also surfaced in the grid
// as "✓ Added" so users can spot what they already have.

const { t } = useI18n()

const props = defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'imported', id: string): void
}>()

const characters = useCharactersStore()

const query = ref('')
const sort = ref<SortOption>('trending_downloads')
const nsfw = ref(false)
const results = ref<LibraryResult[]>([])
// CHUB's search API returns `count` as the number of nodes on the current
// page (NOT the total match count), so it's useless as a "more pages?"
// signal. We infer `hasMore` from the batch size instead: if the last
// fetch returned a full page, we assume another page exists. Works the
// same as cursor-based pagination in practice.
const hasMore = ref(false)
const PER_PAGE = 24
const page = ref(1)
const loading = ref(false)
const error = ref<string | null>(null)
// Tag filter panel collapsed by default on small viewports; user toggles
// via the "Фильтры" button in the filter row.
const tagsOpen = ref(false)

// Curated quick-filter chip groups. CHUB has thousands of tags but most
// users only ever want to narrow by 3-4 axes (language, gender, genre,
// purpose). Chips are additive — picking multiple within the same group
// expands the match (OR within group, AND across groups). Tag strings
// match CHUB's canonical lowercase form.
interface TagGroup {
  label: string      // i18n key under `browse.tagGroups.*`
  tags: string[]
}
const TAG_GROUPS: TagGroup[] = [
  { label: 'language', tags: ['english', 'russian', 'japanese', 'korean', 'chinese', 'spanish', 'french', 'german'] },
  { label: 'gender',   tags: ['female', 'male', 'non-binary', 'futa', 'trap', 'group'] },
  { label: 'genre',    tags: ['fantasy', 'sci-fi', 'slice-of-life', 'horror', 'romance', 'adventure', 'historical', 'modern', 'anime'] },
  { label: 'purpose',  tags: ['roleplay', 'companion', 'storytelling', 'assistant', 'game-character'] },
]
// Flat active-tag set — preserves insertion order for readable URLs.
const activeTags = ref<string[]>([])
function toggleTag(t: string) {
  const idx = activeTags.value.indexOf(t)
  if (idx >= 0) activeTags.value.splice(idx, 1)
  else activeTags.value.push(t)
}
function clearTags() {
  activeTags.value = []
}

// fullPath → character ID, so we can render "Added" badges on cards the
// user already imported from CHUB.
const imported = ref<Record<string, string>>({})
const importing = ref<Record<string, boolean>>({})

// Detail preview (clicked card pins here for a fuller look).
const preview = ref<LibraryResult | null>(null)

let searchSeq = 0
let debounceTimer: ReturnType<typeof setTimeout> | null = null

watch(() => props.modelValue, (open) => {
  if (open) {
    results.value = []
    page.value = 1
    hasMore.value = false
    preview.value = null
    query.value = ''
    void runSearch()
  }
})

// Any filter edit resets pagination to page 1.
watch([query, sort, nsfw, activeTags], () => {
  // Debounce search 250ms — avoids hammering CHUB on every keystroke.
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(() => {
    page.value = 1
    results.value = []
    hasMore.value = false
    void runSearch()
  }, 250)
}, { deep: true })

async function runSearch() {
  if (!props.modelValue) return
  const seq = ++searchSeq
  loading.value = true
  error.value = null
  try {
    const res = await libraryApi.searchChub({
      q: query.value || undefined,
      page: page.value,
      per_page: PER_PAGE,
      sort: sort.value,
      nsfw: nsfw.value,
      tags: activeTags.value.length ? activeTags.value : undefined,
    })
    if (seq !== searchSeq) return // stale response
    // Page switch mode: replace results on every page. We don't
    // concatenate — prev/next arrows navigate, not infinite-scroll.
    results.value = res.items
    hasMore.value = res.items.length >= PER_PAGE
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

function goToPage(n: number) {
  if (n < 1 || loading.value) return
  page.value = n
  void runSearch()
  // Scroll the grid back to top so users see the new page from its start.
  const scroller = document.querySelector('.nest-browse-body')
  if (scroller) scroller.scrollTo({ top: 0, behavior: 'smooth' })
}

function nextPage() {
  if (!hasMore.value || loading.value) return
  goToPage(page.value + 1)
}
function prevPage() {
  if (page.value <= 1 || loading.value) return
  goToPage(page.value - 1)
}

async function doImport(card: LibraryResult) {
  if (importing.value[card.full_path] || imported.value[card.full_path]) return
  importing.value = { ...importing.value, [card.full_path]: true }
  try {
    const created = await libraryApi.importChub(card.full_path)
    // Hot-inject into the characters store so the Library grid reflects it
    // immediately without a refetch.
    characters.items.unshift(created)
    imported.value = { ...imported.value, [card.full_path]: created.id }
    emit('imported', created.id)
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    importing.value = { ...importing.value, [card.full_path]: false }
  }
}

function close() {
  emit('update:modelValue', false)
}

function stars(n: number): string {
  if (n >= 1000) return (n / 1000).toFixed(1).replace(/\.0$/, '') + 'k'
  return String(n)
}
</script>

<template>
  <v-dialog
    :model-value="modelValue"
    fullscreen
    scrollable
    transition="dialog-bottom-transition"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <v-card class="nest-browse">
      <!-- Header -->
      <div class="nest-browse-head">
        <div class="d-flex align-center ga-3">
          <v-btn icon="mdi-arrow-left" variant="text" @click="close" />
          <div>
            <div class="nest-eyebrow">{{ t('browse.subtitle') }}</div>
            <h2 class="nest-h2">{{ t('browse.title') }}</h2>
          </div>
        </div>
        <div class="nest-mono nest-browse-count">
          <!-- CHUB's `count` is the current-page size, not a total, so we
               show the current page index instead of a bogus "N cards". -->
          {{ results.length ? t('browse.pageN', { n: page }) : '' }}
        </div>
      </div>

      <!-- Filter bar -->
      <div class="nest-browse-filters">
        <v-text-field
          v-model="query"
          :placeholder="t('browse.searchPlaceholder')"
          prepend-inner-icon="mdi-magnify"
          hide-details
          density="compact"
          clearable
          class="nest-browse-search"
        />
        <v-select
          v-model="sort"
          :items="[
            { value: 'trending_downloads', title: t('browse.sort.trending') },
            { value: 'last_activity_at',   title: t('browse.sort.recent') },
            { value: 'created_at',          title: t('browse.sort.newest') },
            { value: 'star_count',          title: t('browse.sort.stars') },
          ]"
          hide-details
          density="compact"
          style="max-width: 220px"
        />
        <v-switch
          v-model="nsfw"
          :label="t('browse.nsfw')"
          color="error"
          hide-details
          density="compact"
        />
        <v-btn
          size="small"
          :variant="tagsOpen ? 'tonal' : 'outlined'"
          :color="activeTags.length ? 'primary' : undefined"
          :append-icon="tagsOpen ? 'mdi-chevron-up' : 'mdi-chevron-down'"
          @click="tagsOpen = !tagsOpen"
        >
          {{ t('browse.filtersToggle') }}
          <span v-if="activeTags.length" class="nest-mono ml-1">· {{ activeTags.length }}</span>
        </v-btn>
      </div>

      <!-- Tag filter chips — collapsible. Curated quick-picks grouped by axis. -->
      <div v-show="tagsOpen" class="nest-browse-tags">
        <div v-for="group in TAG_GROUPS" :key="group.label" class="nest-browse-tag-group">
          <div class="nest-browse-tag-group-label">
            {{ t('browse.tagGroups.' + group.label) }}
          </div>
          <div class="nest-browse-tag-row">
            <v-chip
              v-for="tag in group.tags"
              :key="tag"
              :variant="activeTags.includes(tag) ? 'tonal' : 'outlined'"
              :color="activeTags.includes(tag) ? 'primary' : undefined"
              size="small"
              class="nest-browse-tag-chip"
              @click="toggleTag(tag)"
            >
              {{ t('browse.tags.' + tag, tag) }}
            </v-chip>
          </div>
        </div>
        <v-btn
          v-if="activeTags.length"
          variant="text"
          size="x-small"
          prepend-icon="mdi-close"
          @click="clearTags"
        >
          {{ t('browse.clearTags') }}
          <span class="nest-mono ml-1 text-medium-emphasis">{{ activeTags.length }}</span>
        </v-btn>
      </div>

      <!-- Grid -->
      <div class="nest-browse-body">
        <div v-if="error" class="nest-browse-state">
          <v-alert type="error" variant="tonal">{{ error }}</v-alert>
        </div>

        <div v-if="results.length === 0 && !loading && !error" class="nest-browse-state text-center">
          <v-icon size="40" color="surface-variant">mdi-magnify</v-icon>
          <div class="nest-h3 mt-2">{{ t('browse.emptyTitle') }}</div>
          <p class="nest-subtitle">{{ t('browse.emptyHint') }}</p>
        </div>

        <div class="nest-browse-grid">
          <div
            v-for="card in results"
            :key="card.full_path"
            class="nest-browse-card"
            :class="{ 'is-imported': !!imported[card.full_path] }"
            @click="preview = card"
          >
            <div class="nest-browse-thumb">
              <img
                v-if="card.avatar_url"
                :src="card.avatar_url"
                :alt="card.name"
                loading="lazy"
                referrerpolicy="no-referrer"
              />
              <div v-else class="nest-browse-thumb-fallback">?</div>
              <div v-if="card.nsfw" class="nest-browse-nsfw-badge">NSFW</div>
            </div>
            <div class="nest-browse-body-text">
              <div class="nest-browse-name">{{ card.name }}</div>
              <div class="nest-browse-creator nest-mono">
                <v-icon size="10">mdi-at</v-icon>{{ card.creator }}
              </div>
              <div v-if="card.tagline" class="nest-browse-tagline">{{ card.tagline }}</div>
              <div class="nest-browse-meta nest-mono">
                <span class="d-flex align-center">
                  <v-icon size="12">mdi-star</v-icon>{{ stars(card.star_count) }}
                </span>
                <span v-if="card.tags?.length" class="nest-browse-tag">
                  {{ card.tags[0] }}
                </span>
              </div>
            </div>
            <v-btn
              class="nest-browse-import-btn"
              :color="imported[card.full_path] ? 'success' : 'primary'"
              :variant="imported[card.full_path] ? 'tonal' : 'flat'"
              :loading="importing[card.full_path]"
              :disabled="!!imported[card.full_path]"
              size="small"
              block
              @click.stop="doImport(card)"
            >
              {{
                imported[card.full_path]
                  ? t('browse.added')
                  : t('browse.import')
              }}
            </v-btn>
          </div>
        </div>

        <div v-if="loading" class="nest-browse-state">
          <v-progress-circular indeterminate color="primary" size="28" />
        </div>

        <!-- Page nav — shown once we have results. Prev disables on page 1,
             Next disables when the last batch came back short (our
             "reached the end" heuristic since CHUB doesn't give a total). -->
        <div v-else-if="results.length > 0" class="nest-browse-pager">
          <v-btn
            variant="outlined"
            size="small"
            prepend-icon="mdi-chevron-left"
            :disabled="page <= 1"
            @click="prevPage"
          >
            {{ t('browse.pageBack') }}
          </v-btn>
          <span class="nest-mono nest-browse-pager-label">
            {{ t('browse.pageN', { n: page }) }}
          </span>
          <v-btn
            variant="outlined"
            size="small"
            append-icon="mdi-chevron-right"
            :disabled="!hasMore"
            @click="nextPage"
          >
            {{ t('browse.pageNext') }}
          </v-btn>
        </div>
      </div>

      <!-- Detail-preview drawer -->
      <v-navigation-drawer
        v-if="preview"
        :model-value="true"
        location="right"
        temporary
        width="420"
        class="nest-browse-preview"
        @update:model-value="v => !v && (preview = null)"
      >
        <div class="nest-preview-head">
          <v-btn icon="mdi-close" variant="text" size="small" @click="preview = null" />
        </div>
        <div class="nest-preview-body">
          <img
            v-if="preview.max_res_url || preview.avatar_url"
            :src="preview.max_res_url || preview.avatar_url"
            :alt="preview.name"
            class="nest-preview-img"
            loading="lazy"
            referrerpolicy="no-referrer"
          />
          <h2 class="nest-h2 mt-3">{{ preview.name }}</h2>
          <div class="nest-mono text-medium-emphasis">
            <v-icon size="12">mdi-at</v-icon>{{ preview.creator }}
          </div>
          <p v-if="preview.tagline" class="nest-preview-tagline">{{ preview.tagline }}</p>
          <p v-if="preview.description" class="nest-preview-desc">{{ preview.description }}</p>
          <div v-if="preview.tags?.length" class="nest-preview-tags">
            <v-chip
              v-for="tag in preview.tags"
              :key="tag"
              size="x-small"
              variant="outlined"
              class="mr-1 mb-1"
            >
              {{ tag }}
            </v-chip>
          </div>
          <v-btn
            class="mt-4"
            block
            :color="imported[preview.full_path] ? 'success' : 'primary'"
            :variant="imported[preview.full_path] ? 'tonal' : 'flat'"
            :loading="importing[preview.full_path]"
            :disabled="!!imported[preview.full_path]"
            :prepend-icon="imported[preview.full_path] ? 'mdi-check' : 'mdi-download'"
            @click="doImport(preview)"
          >
            {{ imported[preview.full_path] ? t('browse.added') : t('browse.importToLibrary') }}
          </v-btn>
          <a
            :href="'https://chub.ai/characters/' + preview.full_path"
            target="_blank"
            rel="noopener"
            class="nest-preview-link nest-mono"
          >
            {{ t('browse.openOnChub') }} ↗
          </a>
        </div>
      </v-navigation-drawer>
    </v-card>
  </v-dialog>
</template>

<style lang="scss" scoped>
.nest-browse {
  background: var(--nest-bg) !important;
  display: flex;
  flex-direction: column;
  // Fullscreen dialog — 100dvh so it reflows when the mobile URL bar
  // collapses or the IME keyboard pops up. 100vh here left a dead
  // strip at the bottom on mobile Safari.
  height: 100dvh;
}

.nest-browse-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 20px;
  border-bottom: 1px solid var(--nest-border);
  flex-shrink: 0;
}

.nest-browse-count {
  font-size: 11px;
  color: var(--nest-text-muted);
  letter-spacing: 0.05em;
}

.nest-browse-filters {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 20px;
  border-bottom: 1px solid var(--nest-border);
  background: var(--nest-bg-elevated);
  flex-shrink: 0;
  flex-wrap: wrap;
}

// 260px was too chunky on phones — eats the whole filter row. Soft
// floor at 160px; flex-wrap in the parent lets the sort/nsfw drop
// below the input when there's not enough horizontal room.
.nest-browse-search { flex: 1 1 160px; min-width: 0; }

.nest-browse-tags {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 20px 12px;
  border-bottom: 1px solid var(--nest-border);
  background: var(--nest-bg);
  flex-shrink: 0;
}
.nest-browse-tag-group {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.nest-browse-tag-group-label {
  font-family: var(--nest-font-mono);
  font-size: 10px;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--nest-text-muted);
  min-width: 70px;
}
.nest-browse-tag-row {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  flex: 1;
  min-width: 0;
}
.nest-browse-tag-chip {
  cursor: pointer;
}

@media (max-width: 520px) {
  .nest-browse-tags { padding: 8px 12px 10px; }
  .nest-browse-tag-group-label { min-width: 0; flex-basis: 100%; }

  // On phones the header takes less horizontal real estate. Bigger cards
  // (2 per row instead of minmax auto-fill) so cover art stays legible.
  .nest-browse-head     { padding: 10px 12px; }
  .nest-browse-filters  { padding: 10px 12px; gap: 8px; }
  .nest-browse-body     { padding: 12px; }
  .nest-browse-grid     {
    grid-template-columns: repeat(2, 1fr);
    gap: 10px;
  }
  .nest-browse-name     { font-size: 13px; }
  .nest-browse-tagline  { font-size: 11.5px; }
  .nest-browse-pager    { padding: 16px 10px 20px; gap: 8px; }
  .nest-browse-search   { flex-basis: 100%; }
  // The pager's Prev / Next arrows should each take half the row so the
  // page-index label sits above on a line by itself when wrap kicks in.
  .nest-browse-pager .v-btn { flex: 1 1 auto; }
}

.nest-browse-body {
  overflow-y: auto;
  flex: 1;
  padding: 20px;
  max-width: 1400px;
  margin: 0 auto;
  width: 100%;
}

.nest-browse-state {
  padding: 40px;
  display: grid;
  place-items: center;
}

.nest-browse-pager {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 32px 16px 24px;
  flex-wrap: wrap;
}
.nest-browse-pager-label {
  font-size: 12px;
  color: var(--nest-text-secondary);
  letter-spacing: 0.04em;
  padding: 0 8px;
}

.nest-browse-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 14px;
}

.nest-browse-card {
  position: relative;
  display: flex;
  flex-direction: column;
  border: 1px solid var(--nest-border);
  border-radius: var(--nest-radius);
  background: var(--nest-surface);
  overflow: hidden;
  cursor: pointer;
  transition: border-color var(--nest-transition-fast), transform var(--nest-transition-fast);

  &:hover {
    border-color: var(--nest-accent);
    transform: translateY(-2px);
  }
  &.is-imported { border-color: var(--nest-green); }
}

.nest-browse-thumb {
  position: relative;
  aspect-ratio: 2 / 3;
  background: var(--nest-bg-elevated);
  overflow: hidden;

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }
}
.nest-browse-thumb-fallback {
  width: 100%;
  height: 100%;
  display: grid;
  place-items: center;
  font-size: 48px;
  color: var(--nest-text-muted);
}

.nest-browse-nsfw-badge {
  position: absolute;
  top: 6px;
  right: 6px;
  padding: 2px 6px;
  font-family: var(--nest-font-mono);
  font-size: 9px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: #fff;
  background: rgba(239, 68, 68, 0.85);
  border-radius: 3px;
}

.nest-browse-body-text {
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 4px;
  flex: 1;
  min-width: 0;
}

.nest-browse-name {
  font-family: var(--nest-font-display);
  font-size: 14px;
  font-weight: 500;
  color: var(--nest-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.nest-browse-creator {
  font-size: 10.5px;
  color: var(--nest-text-muted);
  display: flex;
  align-items: center;
  gap: 2px;
}

.nest-browse-tagline {
  font-size: 12px;
  color: var(--nest-text-secondary);
  line-height: 1.4;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.nest-browse-meta {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 10.5px;
  color: var(--nest-text-muted);
  margin-top: auto;
  padding-top: 4px;
}
.nest-browse-tag {
  color: var(--nest-text-secondary);
}

.nest-browse-import-btn {
  border-radius: 0;
  border-top: 1px solid var(--nest-border-subtle);
}

// ── Preview drawer ─────────────────────────────────────────────────
.nest-browse-preview {
  background: var(--nest-bg-elevated) !important;
  border-left: 1px solid var(--nest-border) !important;
}
.nest-preview-head {
  display: flex;
  justify-content: flex-end;
  padding: 8px;
}
.nest-preview-body {
  padding: 0 20px 40px;
}
.nest-preview-img {
  width: 100%;
  max-height: 420px;
  object-fit: contain;
  background: var(--nest-bg);
  border-radius: var(--nest-radius-sm);
}
.nest-preview-tagline {
  font-size: 14px;
  color: var(--nest-text);
  font-style: italic;
  margin-top: 8px;
}
.nest-preview-desc {
  font-size: 13px;
  color: var(--nest-text-secondary);
  line-height: 1.55;
  white-space: pre-wrap;
  word-wrap: break-word;
  margin-top: 8px;
}
.nest-preview-tags {
  margin-top: 12px;
}
.nest-preview-link {
  display: inline-block;
  margin-top: 12px;
  font-size: 11px;
  color: var(--nest-text-muted);
  text-decoration: none;

  &:hover { color: var(--nest-accent); }
}
</style>
