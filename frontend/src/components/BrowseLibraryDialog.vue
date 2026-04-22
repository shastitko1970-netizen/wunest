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
const totalCount = ref(0)
const page = ref(1)
const loading = ref(false)
const error = ref<string | null>(null)

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
    preview.value = null
    query.value = ''
    void runSearch()
  }
})

watch([query, sort, nsfw], () => {
  // Debounce search 250ms — avoids hammering CHUB on every keystroke.
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(() => {
    page.value = 1
    results.value = []
    void runSearch()
  }, 250)
})

async function runSearch() {
  if (!props.modelValue) return
  const seq = ++searchSeq
  loading.value = true
  error.value = null
  try {
    const res = await libraryApi.searchChub({
      q: query.value || undefined,
      page: page.value,
      per_page: 24,
      sort: sort.value,
      nsfw: nsfw.value,
    })
    if (seq !== searchSeq) return // stale response
    if (page.value === 1) {
      results.value = res.items
    } else {
      results.value = [...results.value, ...res.items]
    }
    totalCount.value = res.count
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

function loadMore() {
  page.value += 1
  void runSearch()
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
          {{ totalCount ? t('browse.total', { n: totalCount.toLocaleString() }) : '' }}
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

        <div v-else-if="results.length > 0 && results.length < totalCount" class="nest-browse-state">
          <v-btn variant="outlined" @click="loadMore">
            {{ t('browse.loadMore') }}
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
  height: 100vh;
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
