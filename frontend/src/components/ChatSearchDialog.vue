<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { chatsApi, type SearchHit } from '@/api/chats'

// Full-text chat search dialog. Opens on Ctrl+K / ⌘K anywhere in the
// app (wired from AppShell). Debounced query → /api/chats/search.
//
// Server returns snippets wrapped with `<<<...>>>` around match
// highlights; we swap those for `<mark>` at render time. Pure string
// replace is safe — server escapes content for HTML output via
// ts_headline's default StartSel/StopSel, which we opted out of in
// favour of predictable markers. If user content contains literal
// `<<<` it'll collide, but it's cheap enough to ignore for v1.

const { t } = useI18n()
const router = useRouter()

const props = defineProps<{
  modelValue: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
}>()

const query = ref('')
const loading = ref(false)
const error = ref<string | null>(null)
const results = ref<SearchHit[]>([])

// Reset every time the dialog opens so the query state isn't stale
// from a previous session.
watch(() => props.modelValue, (open) => {
  if (open) {
    query.value = ''
    results.value = []
    error.value = null
    // Auto-focus on input — handled by v-text-field autofocus prop.
  }
})

// Debounced search. 250ms balances "feels live" with "don't spam the
// backend on every keystroke". Min query length 2 to avoid 1-char
// scans (which return huge result sets and aren't useful).
let searchTimer: ReturnType<typeof setTimeout> | null = null
watch(query, (q) => {
  if (searchTimer) clearTimeout(searchTimer)
  const trimmed = q.trim()
  if (trimmed.length < 2) {
    results.value = []
    loading.value = false
    return
  }
  loading.value = true
  searchTimer = setTimeout(async () => {
    try {
      error.value = null
      const res = await chatsApi.search(trimmed, { limit: 50 })
      // Only apply if query is still current (user may have typed more
      // while fetch was in flight).
      if (query.value.trim() === trimmed) {
        results.value = res.items
      }
    } catch (e: any) {
      error.value = e?.message || String(e)
    } finally {
      loading.value = false
    }
  }, 250)
})

// Markup from ts_headline with <<<matched>>> → <mark>matched</mark>.
// Keeps everything else as plain text (escape HTML first to prevent
// XSS from adversarial message content).
function renderSnippet(snippet: string): string {
  const escaped = snippet
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
  return escaped
    .replace(/&lt;&lt;&lt;/g, '<mark>')
    .replace(/&gt;&gt;&gt;/g, '</mark>')
}

function formatDate(s: string): string {
  const d = new Date(s)
  return d.toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' })
}

function openHit(hit: SearchHit) {
  router.push(`/chat/${hit.chat_id}?message=${hit.message_id}`)
  emit('update:modelValue', false)
}

const emptyState = computed(() => {
  if (query.value.trim().length < 2) return 'hint'
  if (loading.value) return 'loading'
  if (results.value.length === 0) return 'nothing'
  return 'results'
})
</script>

<template>
  <v-dialog
    :model-value="modelValue"
    max-width="680"
    scrollable
    @update:model-value="emit('update:modelValue', $event)"
  >
    <v-card class="nest-search-dialog">
      <v-card-text class="pa-3">
        <v-text-field
          v-model="query"
          :placeholder="t('chat.search.placeholder')"
          prepend-inner-icon="mdi-magnify"
          autofocus
          hide-details
          variant="outlined"
          density="compact"
          clearable
        />
      </v-card-text>

      <v-divider />

      <div class="nest-search-results">
        <template v-if="emptyState === 'hint'">
          <div class="nest-search-state">
            <v-icon size="28" color="medium-emphasis">mdi-keyboard</v-icon>
            <div class="text-body-2 mt-2">{{ t('chat.search.hint') }}</div>
          </div>
        </template>
        <template v-else-if="emptyState === 'loading'">
          <div class="nest-search-state">
            <v-progress-circular indeterminate size="24" />
          </div>
        </template>
        <template v-else-if="emptyState === 'nothing'">
          <div class="nest-search-state">
            <v-icon size="28" color="medium-emphasis">mdi-magnify-close</v-icon>
            <div class="text-body-2 mt-2">
              {{ t('chat.search.nothing', { q: query.trim() }) }}
            </div>
          </div>
        </template>
        <template v-else>
          <button
            v-for="hit in results"
            :key="`${hit.chat_id}-${hit.message_id}`"
            type="button"
            class="nest-search-hit"
            @click="openHit(hit)"
          >
            <div class="nest-search-hit-head">
              <span class="nest-search-hit-chat">{{ hit.chat_name }}</span>
              <span v-if="hit.character_name" class="nest-search-hit-char">
                · {{ hit.character_name }}
              </span>
              <span class="nest-search-hit-date">{{ formatDate(hit.created_at) }}</span>
            </div>
            <div class="nest-search-hit-role">
              <v-icon size="12">
                {{ hit.role === 'user' ? 'mdi-account' : hit.role === 'assistant' ? 'mdi-robot' : 'mdi-cog' }}
              </v-icon>
              <span>{{ hit.role }}</span>
            </div>
            <!-- eslint-disable-next-line vue/no-v-html — snippet is pre-sanitised -->
            <div class="nest-search-hit-snippet" v-html="renderSnippet(hit.snippet)" />
          </button>
        </template>

        <v-alert
          v-if="error"
          type="error"
          variant="tonal"
          density="compact"
          class="ma-3"
          closable
          @click:close="error = null"
        >
          {{ error }}
        </v-alert>
      </div>
    </v-card>
  </v-dialog>
</template>

<style lang="scss" scoped>
.nest-search-dialog {
  background: var(--nest-surface);
}
.nest-search-results {
  max-height: 60vh;
  overflow-y: auto;
  padding: 6px 8px 10px;
}
.nest-search-state {
  padding: 40px 16px;
  text-align: center;
  color: var(--nest-text-muted);
}

.nest-search-hit {
  display: block;
  width: 100%;
  padding: 10px 14px;
  border: 0;
  border-radius: var(--nest-radius);
  background: transparent;
  text-align: left;
  cursor: pointer;
  margin-bottom: 4px;
  transition: background var(--nest-transition-fast);

  &:hover {
    background: var(--nest-bg-elevated);
  }
  &:focus-visible {
    outline: 2px solid var(--nest-accent);
    outline-offset: 2px;
  }
}
.nest-search-hit-head {
  display: flex;
  align-items: baseline;
  gap: 6px;
  font-size: 11.5px;
  color: var(--nest-text-muted);
  margin-bottom: 2px;
}
.nest-search-hit-chat {
  color: var(--nest-text);
  font-weight: 600;
}
.nest-search-hit-date {
  margin-left: auto;
  font-variant-numeric: tabular-nums;
  flex-shrink: 0;
}
.nest-search-hit-role {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 10.5px;
  color: var(--nest-text-muted);
  margin-bottom: 4px;
  text-transform: capitalize;
}
.nest-search-hit-snippet {
  font-size: 13px;
  line-height: 1.5;
  color: var(--nest-text-secondary);
  :deep(mark) {
    background: color-mix(in srgb, var(--nest-accent) 30%, transparent);
    color: var(--nest-text);
    padding: 0 2px;
    border-radius: 2px;
  }
}
</style>
