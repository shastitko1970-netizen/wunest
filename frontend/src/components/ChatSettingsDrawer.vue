<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useDisplay } from 'vuetify'
import { chatsApi, type Chat, type ChatStats, type Summary } from '@/api/chats'

// Per-chat settings drawer. Three tabs:
//   - Tags    — user-authored organisation. Edit inline; filter in
//               ChatList sidebar.
//   - Stats   — aggregate counts + token totals + unique models.
//   - Memory  — rolling auto-summary + manual notes + pinned facts.
//               Manual "Summarise now" button triggers regen.
//
// Opened from the chat header. Right-side drawer on desktop,
// full-screen dialog on mobile.

const { t } = useI18n()
const { smAndDown } = useDisplay()

const props = defineProps<{
  modelValue: boolean
  chat: Chat | null
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'tags-changed', tags: string[]): void
}>()

const tab = ref<'tags' | 'stats' | 'memory'>('tags')

// ─── Tags ──────────────────────────────────────────────────────────
const tagDraft = ref<string[]>([])
const tagInput = ref('')
const tagSuggestions = ref<string[]>([])
const tagsLoading = ref(false)

watch(() => props.chat?.tags, (v) => {
  tagDraft.value = v ? [...v] : []
}, { immediate: true })

watch(() => props.modelValue, async (open) => {
  if (!open) return
  tab.value = 'tags'
  // Refresh suggestions from distinct tags user has ever used.
  try {
    const res = await chatsApi.listTags()
    tagSuggestions.value = res.items
  } catch { /* non-fatal */ }
  // Load stats + summaries lazily when user clicks those tabs; no
  // upfront cost.
})

// Filter suggestions — hide already-applied tags, case-insensitive
// prefix match against the input value.
const filteredSuggestions = computed(() => {
  const lowerDraft = new Set(tagDraft.value.map(t => t.toLowerCase()))
  const q = tagInput.value.trim().toLowerCase()
  return tagSuggestions.value
    .filter(t => !lowerDraft.has(t.toLowerCase()))
    .filter(t => q === '' || t.toLowerCase().includes(q))
    .slice(0, 8)
})

function addTag(raw: string) {
  const t = raw.trim()
  if (!t) return
  // Dedup case-insensitively so "RP" and "rp" don't both exist.
  if (tagDraft.value.some(existing => existing.toLowerCase() === t.toLowerCase())) return
  tagDraft.value.push(t)
  tagInput.value = ''
  saveTags()
}

function removeTag(tag: string) {
  tagDraft.value = tagDraft.value.filter(t => t !== tag)
  saveTags()
}

async function saveTags() {
  if (!props.chat) return
  tagsLoading.value = true
  try {
    await chatsApi.setTags(props.chat.id, tagDraft.value)
    emit('tags-changed', tagDraft.value)
  } catch (e) {
    console.error('save tags', e)
  } finally {
    tagsLoading.value = false
  }
}

function onTagEnter() {
  if (tagInput.value.trim()) addTag(tagInput.value)
}

// ─── Stats ─────────────────────────────────────────────────────────
const stats = ref<ChatStats | null>(null)
const statsLoading = ref(false)

watch(tab, async (t) => {
  if (t !== 'stats' || !props.chat) return
  statsLoading.value = true
  try {
    stats.value = await chatsApi.stats(props.chat.id)
  } catch (e) {
    console.error('stats', e)
  } finally {
    statsLoading.value = false
  }
})

function formatDate(s?: string): string {
  if (!s) return '—'
  return new Date(s).toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' })
}
function compactNum(n: number): string {
  if (n < 1000) return String(n)
  if (n < 1_000_000) return (n / 1000).toFixed(1) + 'K'
  return (n / 1_000_000).toFixed(1) + 'M'
}

// ─── Memory ────────────────────────────────────────────────────────
const summaries = ref<Summary[]>([])
const summariesLoading = ref(false)
const summarizing = ref(false)
const summarizeError = ref<string | null>(null)
const newSummaryText = ref('')
const newSummaryPinned = ref(false)

// Split by role for rendering.
const autoSummary = computed(() => summaries.value.find(s => s.role === 'auto'))
const manualSummaries = computed(() => summaries.value.filter(s => s.role === 'manual'))
const pinnedSummaries = computed(() => summaries.value.filter(s => s.role === 'pinned'))

watch(tab, async (t) => {
  if (t !== 'memory' || !props.chat) return
  await refreshSummaries()
})

async function refreshSummaries() {
  if (!props.chat) return
  summariesLoading.value = true
  try {
    const res = await chatsApi.listSummaries(props.chat.id)
    summaries.value = res.items
  } catch (e) {
    console.error('list summaries', e)
  } finally {
    summariesLoading.value = false
  }
}

async function runSummarize() {
  if (!props.chat || summarizing.value) return
  summarizing.value = true
  summarizeError.value = null
  try {
    const res = await chatsApi.summarize(props.chat.id)
    if (res.summary) {
      // Refresh full list so ordering + existing manual/pinned stay in sync.
      await refreshSummaries()
    } else if (res.message) {
      summarizeError.value = res.message
    }
  } catch (e: any) {
    summarizeError.value = e?.message || String(e)
  } finally {
    summarizing.value = false
  }
}

async function addManualSummary() {
  const txt = newSummaryText.value.trim()
  if (!txt || !props.chat) return
  try {
    await chatsApi.createSummary(props.chat.id, txt, newSummaryPinned.value)
    newSummaryText.value = ''
    newSummaryPinned.value = false
    await refreshSummaries()
  } catch (e) {
    console.error('create summary', e)
  }
}

async function saveSummaryEdit(s: Summary, nextContent: string) {
  if (!props.chat) return
  try {
    await chatsApi.updateSummary(props.chat.id, s.id, { content: nextContent })
    s.content = nextContent
  } catch (e) {
    console.error('update summary', e)
  }
}

async function removeSummary(s: Summary) {
  if (!props.chat) return
  try {
    await chatsApi.deleteSummary(props.chat.id, s.id)
    summaries.value = summaries.value.filter(x => x.id !== s.id)
  } catch (e) {
    console.error('delete summary', e)
  }
}

async function promoteAutoToManual(s: Summary) {
  if (!props.chat) return
  try {
    // role=manual prevents the next auto regen from overwriting user edits.
    await chatsApi.updateSummary(props.chat.id, s.id, { role: 'manual' })
    await refreshSummaries()
  } catch (e) {
    console.error('promote summary', e)
  }
}

function close() { emit('update:modelValue', false) }
</script>

<template>
  <v-dialog
    :model-value="modelValue"
    :max-width="smAndDown ? undefined : 560"
    :fullscreen="smAndDown"
    scrollable
    @update:model-value="close"
  >
    <v-card class="nest-chat-settings">
      <v-card-title class="d-flex align-center ga-2 py-3">
        <v-icon size="20">mdi-tune-variant</v-icon>
        <span>{{ t('chat.settings.title') }}</span>
        <v-spacer />
        <v-btn variant="text" size="small" icon="mdi-close" @click="close" />
      </v-card-title>

      <v-tabs v-model="tab" density="compact" color="primary" :grow="false">
        <v-tab value="tags">
          <v-icon size="16" class="mr-1">mdi-tag-outline</v-icon>
          {{ t('chat.settings.tabs.tags') }}
        </v-tab>
        <v-tab value="stats">
          <v-icon size="16" class="mr-1">mdi-chart-box-outline</v-icon>
          {{ t('chat.settings.tabs.stats') }}
        </v-tab>
        <v-tab value="memory">
          <v-icon size="16" class="mr-1">mdi-brain</v-icon>
          {{ t('chat.settings.tabs.memory') }}
        </v-tab>
      </v-tabs>

      <v-divider />

      <v-card-text class="pa-3">
        <!-- ── Tags tab ───────────────────────────────────────── -->
        <div v-if="tab === 'tags'">
          <div class="nest-hint mb-2">{{ t('chat.settings.tagsHint') }}</div>
          <div class="nest-tag-chips mb-2">
            <v-chip
              v-for="tg in tagDraft"
              :key="tg"
              size="small"
              closable
              color="primary"
              variant="tonal"
              @click:close="removeTag(tg)"
            >{{ tg }}</v-chip>
            <v-chip v-if="tagDraft.length === 0" size="small" variant="text" class="text-medium-emphasis">
              {{ t('chat.settings.noTags') }}
            </v-chip>
          </div>
          <v-text-field
            v-model="tagInput"
            :placeholder="t('chat.settings.addTag')"
            density="compact"
            variant="outlined"
            hide-details
            append-inner-icon="mdi-plus"
            @click:append-inner="onTagEnter"
            @keydown.enter.prevent="onTagEnter"
          />
          <div v-if="filteredSuggestions.length" class="nest-tag-sugg mt-2">
            <span class="text-caption text-medium-emphasis">
              {{ t('chat.settings.tagsSuggest') }}
            </span>
            <v-chip
              v-for="s in filteredSuggestions"
              :key="s"
              size="x-small"
              variant="outlined"
              class="ma-1"
              @click="addTag(s)"
            >{{ s }}</v-chip>
          </div>
        </div>

        <!-- ── Stats tab ──────────────────────────────────────── -->
        <div v-else-if="tab === 'stats'">
          <div v-if="statsLoading" class="nest-state py-4">
            <v-progress-circular indeterminate size="24" />
          </div>
          <div v-else-if="stats" class="nest-stats-grid">
            <div class="nest-stat">
              <div class="nest-stat-label">{{ t('chat.settings.stats.messages') }}</div>
              <div class="nest-stat-value nest-mono">{{ compactNum(stats.messages_total) }}</div>
              <div class="nest-stat-sub">
                <span>{{ stats.messages_user }} / {{ stats.messages_assistant }}</span>
                <span v-if="stats.messages_hidden" class="ml-2 text-medium-emphasis">
                  ({{ stats.messages_hidden }} {{ t('chat.settings.stats.hidden') }})
                </span>
              </div>
            </div>
            <div class="nest-stat">
              <div class="nest-stat-label">{{ t('chat.settings.stats.tokens') }}</div>
              <div class="nest-stat-value nest-mono">
                {{ compactNum(stats.tokens_in_total + stats.tokens_out_total) }}
              </div>
              <div class="nest-stat-sub">
                ↓ {{ compactNum(stats.tokens_in_total) }} · ↑ {{ compactNum(stats.tokens_out_total) }}
              </div>
            </div>
            <div class="nest-stat">
              <div class="nest-stat-label">{{ t('chat.settings.stats.swipes') }}</div>
              <div class="nest-stat-value nest-mono">{{ compactNum(stats.swipes_total) }}</div>
            </div>
            <div class="nest-stat">
              <div class="nest-stat-label">{{ t('chat.settings.stats.models') }}</div>
              <div class="nest-stat-value nest-mono">{{ stats.unique_models_used }}</div>
            </div>
            <div class="nest-stat nest-stat-wide">
              <div class="nest-stat-label">{{ t('chat.settings.stats.timespan') }}</div>
              <div class="nest-stat-sub">
                {{ formatDate(stats.first_message_at) }} → {{ formatDate(stats.last_message_at) }}
              </div>
            </div>
          </div>
        </div>

        <!-- ── Memory tab ─────────────────────────────────────── -->
        <div v-else-if="tab === 'memory'">
          <div class="d-flex align-center mb-3">
            <div class="nest-hint flex-grow-1">{{ t('chat.settings.memory.hint') }}</div>
            <v-btn
              size="small"
              variant="tonal"
              color="primary"
              prepend-icon="mdi-auto-fix"
              :loading="summarizing"
              @click="runSummarize"
            >
              {{ t('chat.settings.memory.summarize') }}
            </v-btn>
          </div>

          <v-alert
            v-if="summarizeError"
            type="info"
            variant="tonal"
            density="compact"
            class="mb-3"
            closable
            @click:close="summarizeError = null"
          >
            {{ summarizeError }}
          </v-alert>

          <div v-if="summariesLoading" class="nest-state py-4">
            <v-progress-circular indeterminate size="24" />
          </div>
          <div v-else>
            <!-- Auto summary -->
            <div v-if="autoSummary" class="nest-summary nest-summary--auto">
              <div class="nest-summary-head">
                <v-icon size="14" class="mr-1">mdi-refresh-auto</v-icon>
                {{ t('chat.settings.memory.auto') }}
                <span class="nest-summary-model nest-mono ml-auto">
                  {{ autoSummary.model || '—' }}
                </span>
              </div>
              <v-textarea
                v-model="autoSummary.content"
                density="compact"
                variant="outlined"
                hide-details
                rows="4"
                auto-grow
                @blur="saveSummaryEdit(autoSummary!, autoSummary!.content)"
              />
              <div class="d-flex ga-2 mt-1">
                <v-btn size="x-small" variant="text" prepend-icon="mdi-pin" @click="promoteAutoToManual(autoSummary!)">
                  {{ t('chat.settings.memory.keepFromRegen') }}
                </v-btn>
                <v-btn size="x-small" variant="text" color="error" prepend-icon="mdi-delete-outline" @click="removeSummary(autoSummary!)">
                  {{ t('common.delete') }}
                </v-btn>
              </div>
            </div>

            <!-- Pinned -->
            <div v-if="pinnedSummaries.length" class="nest-summary-group">
              <div class="nest-summary-head-label">
                <v-icon size="14">mdi-pin</v-icon>
                {{ t('chat.settings.memory.pinned') }}
              </div>
              <div v-for="s in pinnedSummaries" :key="s.id" class="nest-summary nest-summary--pinned">
                <v-textarea
                  v-model="s.content"
                  density="compact"
                  variant="outlined"
                  hide-details
                  rows="2"
                  auto-grow
                  @blur="saveSummaryEdit(s, s.content)"
                />
                <v-btn size="x-small" variant="text" color="error" icon="mdi-close" class="mt-1" @click="removeSummary(s)" />
              </div>
            </div>

            <!-- Manual -->
            <div v-if="manualSummaries.length" class="nest-summary-group">
              <div class="nest-summary-head-label">
                <v-icon size="14">mdi-note-text-outline</v-icon>
                {{ t('chat.settings.memory.manual') }}
              </div>
              <div v-for="s in manualSummaries" :key="s.id" class="nest-summary">
                <v-textarea
                  v-model="s.content"
                  density="compact"
                  variant="outlined"
                  hide-details
                  rows="2"
                  auto-grow
                  @blur="saveSummaryEdit(s, s.content)"
                />
                <v-btn size="x-small" variant="text" color="error" icon="mdi-close" class="mt-1" @click="removeSummary(s)" />
              </div>
            </div>

            <!-- Add new manual/pinned -->
            <div class="nest-add-summary mt-4">
              <v-textarea
                v-model="newSummaryText"
                :placeholder="t('chat.settings.memory.addPlaceholder')"
                density="compact"
                variant="outlined"
                hide-details
                rows="2"
                auto-grow
              />
              <div class="d-flex align-center ga-2 mt-2">
                <v-switch
                  v-model="newSummaryPinned"
                  :label="t('chat.settings.memory.pinThis')"
                  density="compact"
                  hide-details
                  color="primary"
                />
                <v-spacer />
                <v-btn size="small" variant="flat" color="primary" prepend-icon="mdi-plus" :disabled="!newSummaryText.trim()" @click="addManualSummary">
                  {{ t('chat.settings.memory.add') }}
                </v-btn>
              </div>
            </div>
          </div>
        </div>
      </v-card-text>
    </v-card>
  </v-dialog>
</template>

<style lang="scss" scoped>
.nest-chat-settings {
  background: var(--nest-surface);
}
.nest-hint {
  font-size: 11.5px;
  color: var(--nest-text-muted);
  line-height: 1.4;
}
.nest-state {
  display: flex;
  justify-content: center;
}

.nest-tag-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  min-height: 24px;
}
.nest-tag-sugg {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 2px;
}

.nest-stats-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}
.nest-stat {
  padding: 10px 12px;
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius);
  background: var(--nest-bg-elevated);
}
.nest-stat-wide { grid-column: 1 / -1; }
.nest-stat-label {
  font-size: 11px;
  color: var(--nest-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.06em;
}
.nest-stat-value {
  font-size: 24px;
  font-weight: 600;
  margin-top: 2px;
}
.nest-stat-sub {
  font-size: 11.5px;
  color: var(--nest-text-secondary);
  margin-top: 2px;
}

.nest-summary {
  padding: 10px 12px;
  margin-bottom: 8px;
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius);
  background: var(--nest-bg-elevated);

  &--auto {
    border-left: 3px solid var(--nest-accent);
  }
  &--pinned {
    border-left: 3px solid rgb(var(--v-theme-warning, 201 136 42));
  }
}
.nest-summary-head {
  display: flex;
  align-items: center;
  font-size: 11.5px;
  color: var(--nest-text-muted);
  margin-bottom: 6px;
}
.nest-summary-model {
  font-size: 10.5px;
  opacity: 0.75;
}
.nest-summary-head-label {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  font-weight: 600;
  color: var(--nest-text-secondary);
  margin: 12px 0 6px;
}
.nest-add-summary {
  padding: 10px 12px;
  border: 1px dashed var(--nest-border-subtle);
  border-radius: var(--nest-radius);
}
</style>
