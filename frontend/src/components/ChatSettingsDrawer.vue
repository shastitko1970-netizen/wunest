<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useDisplay } from 'vuetify'
import { chatsApi, type AuthorsNote, type AutoSummariseConfig, type Chat, type ChatStats, type Summary } from '@/api/chats'
import { useModelsStore } from '@/stores/models'
import { byokApi, type BYOKKey } from '@/api/byok'
// M51 Sprint 3 wave 2 — per-chat theme override picker.
import { useThemeStore, THEME_PRESETS } from '@/stores/theme'

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

// ─── Per-chat theme override (M51 Sprint 3 wave 2) ─────────────────
//
// Picker with the 5 built-in presets + a "use default" sentinel value.
// Saving is fire-and-forget — the watcher in Chat.vue applies the new
// preset reactively when chat_metadata.theme_preset updates.
//
// Empty-string sentinel for "use default" because `<v-select>` v-model
// doesn't tolerate `null` cleanly across all densities.
const themeStore = useThemeStore()
const themePresetItems = computed(() => [
  { value: '', title: t('chat.settings.theme.useDefault') },
  ...THEME_PRESETS.map(p => ({ value: p.id, title: p.label })),
])
const themePresetDraft = computed({
  get: (): string => props.chat?.chat_metadata?.theme_preset ?? '',
  set: (v: string) => { void saveThemePreset(v) },
})
async function saveThemePreset(preset: string) {
  if (!props.chat) return
  try {
    // Server expects null to clear, string to set.
    await chatsApi.setThemePreset(props.chat.id, preset || null)
    // Optimistically mutate the local chat metadata so the watcher in
    // Chat.vue applies the preset without waiting for a refetch.
    if (props.chat.chat_metadata) {
      const meta = { ...(props.chat.chat_metadata as Record<string, unknown>) }
      if (preset) meta.theme_preset = preset
      else delete meta.theme_preset
      props.chat.chat_metadata = meta as Chat['chat_metadata']
    }
  } catch (e) {
    console.warn('set chat theme preset:', e)
  }
}

// Apply the active preset reactively for a small swatch shown next to
// the picker — uses the same data-driven swatches the gallery does.
const themePresetSwatch = computed(() => {
  const id = themePresetDraft.value || themeStore.currentId
  return THEME_PRESETS.find(p => p.id === id)?.swatches ?? null
})

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

// M53 — Author's Note in Memory tab. Previously only in
// GenerationSettings drawer; users associate AN with chat memory, not
// sampler config, so duplicating the editor here improves
// discoverability. State source-of-truth остаётся `chat.chat_metadata.
// authors_note` — мы локальная draft + save через chatsApi.setAuthorsNote.
const noteDraft = ref<AuthorsNote>({ content: '', depth: 4, role: 'system' })
const noteSaving = ref(false)
const noteSavedHint = ref(false)
function hydrateNoteFromChat(c: Chat | null) {
  const an = c?.chat_metadata?.authors_note
  noteDraft.value = an
    ? { content: an.content || '', depth: an.depth ?? 4, role: an.role ?? 'system' }
    : { content: '', depth: 4, role: 'system' }
}
async function saveNote() {
  if (!props.chat) return
  noteSaving.value = true
  try {
    const payload = noteDraft.value.content.trim()
      ? noteDraft.value
      : null
    await chatsApi.setAuthorsNote(props.chat.id, payload)
    // Mirror locally so other components (GenerationSettings, prompt
    // builder) see the change without a refetch.
    if (props.chat.chat_metadata) {
      const meta = { ...(props.chat.chat_metadata as Record<string, unknown>) }
      if (payload) meta.authors_note = payload
      else delete meta.authors_note
      props.chat.chat_metadata = meta as Chat['chat_metadata']
    }
    noteSavedHint.value = true
    setTimeout(() => (noteSavedHint.value = false), 1500)
  } catch (e) {
    console.warn('save authors note:', e)
  } finally {
    noteSaving.value = false
  }
}
async function clearNote() {
  noteDraft.value = { content: '', depth: 4, role: 'system' }
  await saveNote()
}

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
      // Backend returns English literal strings for "no-op" responses
      // (e.g. "nothing new to summarise" when the chat is too short).
      // Map known strings to localised UX copy so the tester doesn't see
      // mixed-language error alerts.
      if (/nothing new/i.test(res.message)) {
        summarizeError.value = t('chat.settings.memory.tooShortToSummarize')
      } else {
        summarizeError.value = res.message
      }
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

// ─── M44 Auto-summary ──────────────────────────────────────────────
// Opt-in per-chat. Hydrates from chat.chat_metadata.auto_summarise.
// Saves on each toggle/change via PUT /auto-summarise; backend stores
// under jsonb_set to preserve sibling metadata. Debounced light because
// slider drag + numeric input both mutate threshold_tokens.

const AUTO_DEFAULT_THRESHOLD = 30_000
const AUTO_THRESHOLD_MAX = 2_000_000

// Local state — mirror of server config; watcher resyncs when chat swaps.
const autoEnabled = ref(false)
const autoThreshold = ref(AUTO_DEFAULT_THRESHOLD)
const autoModel = ref<string>('')
// Source picker: 'wuapi' OR 'byok:<uuid>'.
const autoSource = ref<'wuapi' | string>('wuapi')

const autoSaveError = ref<string | null>(null)
const autoBYOKList = ref<BYOKKey[]>([])
const autoBYOKLoading = ref(false)
const models = useModelsStore()
// Two snapshots of the model list — one for each source — so switching
// source doesn't immediately blast the chat's main model picker. We use
// a dedicated cache by fetching into a local copy.
const autoModelItems = ref<{ id: string }[]>([])
const autoModelsLoading = ref(false)

function hydrateAutoFromChat(c: Chat | null) {
  const cfg = c?.chat_metadata?.auto_summarise
  if (cfg) {
    autoEnabled.value = !!cfg.enabled
    autoThreshold.value = typeof cfg.threshold_tokens === 'number' ? cfg.threshold_tokens : AUTO_DEFAULT_THRESHOLD
    autoModel.value = cfg.model ?? ''
    autoSource.value = cfg.byok_id ? `byok:${cfg.byok_id}` : 'wuapi'
  } else {
    autoEnabled.value = false
    autoThreshold.value = AUTO_DEFAULT_THRESHOLD
    autoModel.value = ''
    autoSource.value = 'wuapi'
  }
}

watch(() => props.chat?.id, () => {
  hydrateAutoFromChat(props.chat)
  hydrateNoteFromChat(props.chat)
}, { immediate: true })
watch(() => props.modelValue, (open) => {
  if (open && props.chat) hydrateAutoFromChat(props.chat)
})

onMounted(async () => {
  // BYOK list — needed for the source picker. Non-fatal if fails
  // (feature still works with just WuApi).
  try {
    autoBYOKLoading.value = true
    const res = await byokApi.list()
    autoBYOKList.value = res.items
  } catch { /* non-fatal */ }
  finally { autoBYOKLoading.value = false }
})

// Fetch model catalogue for current source. Reuses the models store —
// which caches per source — so switching back and forth is free.
//
// Try/catch wraps store call so `models.setActiveSource` rejections
// don't escape as unhandled promise rejections (source of
// «SES_UNCAUGHT_EXCEPTION: null» reported by browser extensions that
// hook unhandled rejections).
async function refreshAutoModels() {
  if (!props.chat) return
  autoModelsLoading.value = true
  try {
    const src = autoSource.value
    if (src === 'wuapi') {
      await models.setActiveSource('wuapi')
    } else {
      const id = src.slice(5)
      await models.setActiveSource({ byokID: id })
    }
    autoModelItems.value = models.items.map(m => ({ id: m.id }))
  } catch (e) {
    console.warn('auto-summarise: refresh model list failed', e)
    autoModelItems.value = []
  } finally {
    autoModelsLoading.value = false
  }
}

// Trigger model fetch when the user expands Memory tab OR changes source.
watch([() => props.modelValue, tab, autoSource], ([open, t]) => {
  if (open && t === 'memory') refreshAutoModels()
}, { immediate: false })

// Debounced save — slider drag emits many changes; batch into one PUT.
let autoSaveDebounce: ReturnType<typeof setTimeout> | null = null
function scheduleAutoSave() {
  if (autoSaveDebounce) clearTimeout(autoSaveDebounce)
  // `setTimeout(asyncFn)` sets up an unhandled-promise dance if the fn
  // rejects. Wrap in .catch() so failures go to console, not to
  // window's unhandled-rejection handler (which SES hooks).
  autoSaveDebounce = setTimeout(() => {
    saveAutoConfig().catch(err => console.warn('auto-summarise save:', err))
  }, 300)
}

async function saveAutoConfig() {
  if (!props.chat) return
  autoSaveError.value = null
  const cfg: AutoSummariseConfig = {
    enabled: autoEnabled.value,
    threshold_tokens: Math.max(0, Math.min(AUTO_THRESHOLD_MAX, Math.round(autoThreshold.value))),
    model: autoModel.value || '',
    byok_id: autoSource.value.startsWith('byok:') ? autoSource.value.slice(5) : null,
  }
  try {
    await chatsApi.setAutoSummarise(props.chat.id, cfg)
    // Client-side chat object mirror — server now has the fresh config,
    // but `props.chat.chat_metadata.auto_summarise` still holds the prior
    // value (parent didn't refetch). Without this mutation the drawer's
    // `hydrateAutoFromChat` (called on drawer-open or chat-id change)
    // reads stale data and overwrites our just-toggled form. Tester:
    // «настройки автосаммари не сохраняются» — exactly this race.
    //
    // Same pattern Chat.vue uses for persona auto-pin: mutate
    // `chat.chat_metadata` in place via spread so reactivity picks up
    // the change. Vue allows it because props.chat is a reactive object
    // (not a primitive); this is explicitly sanctioned for parent-owned
    // deep structures when the child has authoritative knowledge.
    if (props.chat) {
      props.chat.chat_metadata = {
        ...(props.chat.chat_metadata ?? {}),
        auto_summarise: cfg,
      }
    }
  } catch (e: any) {
    autoSaveError.value = e?.message || String(e)
  }
}

// Handler: toggle the enable switch. Saves immediately (not debounced —
// it's a single binary event).
//
// Event handlers below swallow their own rejections explicitly. Vue
// template `@update:model-value` bindings receive the returned Promise
// but don't await it — any rejection becomes «uncaught» and bubbles
// to window, where third-party injectors (SES/MetaMask) surface it
// as an «SES_UNCAUGHT_EXCEPTION». Each handler now has a local .catch().
function onAutoEnabledChange(v: boolean | null) {
  autoEnabled.value = !!v
  saveAutoConfig().catch(err => console.warn('auto-summarise save:', err))
}

async function onAutoSourceChange(v: string | null) {
  if (!v) return
  try {
    autoSource.value = v
    await refreshAutoModels()
    autoModel.value = models.selected || autoModelItems.value[0]?.id || ''
    await saveAutoConfig()
  } catch (err) {
    console.warn('auto-summarise source change:', err)
  }
}

function onAutoModelChange(v: string | null) {
  autoModel.value = v ?? ''
  saveAutoConfig().catch(err => console.warn('auto-summarise save:', err))
}

function onAutoThresholdChange(v: number) {
  autoThreshold.value = v
  scheduleAutoSave()
}

function onAutoThresholdInput(v: string | number) {
  const n = typeof v === 'number' ? v : parseInt(v, 10)
  if (!Number.isFinite(n) || n < 0) return
  onAutoThresholdChange(n)
}

// Pretty label for the current source, used as secondary text next to
// the picker.
const autoSourceLabel = computed(() => {
  if (autoSource.value === 'wuapi') return 'WuApi pool'
  const id = autoSource.value.slice(5)
  const k = autoBYOKList.value.find(x => x.id === id)
  return k ? `${k.label || k.provider} · ${k.masked}` : '—'
})

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

          <!-- Per-chat theme override (M51 Sprint 3 wave 2) -->
          <v-divider class="my-4" />
          <div class="nest-eyebrow mb-1">{{ t('chat.settings.theme.title') }}</div>
          <div class="nest-hint mb-2">{{ t('chat.settings.theme.hint') }}</div>
          <div class="d-flex align-center ga-2">
            <div
              v-if="themePresetSwatch"
              class="nest-chat-theme-swatch"
              :style="{
                background: themePresetSwatch.bg,
                borderColor: themePresetSwatch.border,
              }"
            >
              <span :style="{ background: themePresetSwatch.accent }" class="nest-chat-theme-swatch-dot" />
            </div>
            <v-select
              v-model="themePresetDraft"
              :items="themePresetItems"
              item-title="title"
              item-value="value"
              density="compact"
              variant="outlined"
              hide-details
              style="flex: 1"
            />
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
          <!-- Author's Note (M53) — было только в GenerationSettings,
               пользователи искали в Memory. Дублируем editor сюда; state
               sync через chat.chat_metadata.authors_note + setAuthorsNote
               API. Свернуто в `<details>` чтобы не конкурировать с
               summary controls. -->
          <details class="nest-an-block mb-3" open>
            <summary class="nest-eyebrow nest-an-summary">
              <v-icon size="14" class="mr-1">mdi-note-edit-outline</v-icon>
              {{ t('chat.authorsNote.label') }}
              <span v-if="noteSavedHint" class="nest-mono nest-saving-hint ml-2">
                {{ t('chat.authorsNote.saved') }}
              </span>
            </summary>
            <div class="nest-an-body mt-2">
              <v-textarea
                v-model="noteDraft.content"
                :placeholder="t('chat.authorsNote.placeholder')"
                rows="3"
                auto-grow
                hide-details
                density="compact"
                variant="outlined"
              />
              <div class="nest-an-fields mt-2">
                <v-text-field
                  v-model.number="noteDraft.depth"
                  :label="t('chat.authorsNote.depth')"
                  type="number" :min="0" :max="20"
                  density="compact" variant="outlined" hide-details
                  style="max-width: 110px"
                />
                <v-select
                  v-model="noteDraft.role"
                  :label="t('chat.authorsNote.role')"
                  :items="[
                    { value: 'system', title: t('chat.authorsNote.roleSystem') },
                    { value: 'user', title: t('chat.authorsNote.roleUser') },
                    { value: 'assistant', title: t('chat.authorsNote.roleAssistant') },
                  ]"
                  density="compact" variant="outlined" hide-details
                  style="max-width: 200px"
                />
              </div>
              <div class="nest-an-hint nest-hint mt-1">{{ t('chat.authorsNote.hint') }}</div>
              <div class="d-flex ga-2 justify-end mt-2">
                <v-btn size="x-small" variant="text" @click="clearNote">
                  {{ t('chat.authorsNote.clear') }}
                </v-btn>
                <v-btn
                  size="x-small" color="primary" variant="flat"
                  :loading="noteSaving"
                  prepend-icon="mdi-content-save"
                  @click="saveNote"
                >
                  {{ t('common.save') }}
                </v-btn>
              </div>
            </div>
          </details>

          <v-divider class="mb-3" />

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

          <!-- ── M44: Auto-summary config ─────────────────────── -->
          <!-- Opt-in per-chat. Toggle is same v-switch style as the
               app's other toggles (`inset`, `color="primary"`). When
               enabled, user picks provider + model + threshold; backend
               fires SummariseChat async after each assistant turn whose
               prompt hits threshold. Users pay their own tokens. -->
          <div class="nest-auto-summary mb-4">
            <v-switch
              :model-value="autoEnabled"
              :label="t('chat.settings.memory.auto.toggle')"
              color="primary"
              inset
              hide-details
              density="compact"
              @update:model-value="onAutoEnabledChange"
            />
            <p class="nest-hint nest-hint--sm mt-1">
              {{ t('chat.settings.memory.auto.tagline') }}
            </p>

            <v-expand-transition>
              <div v-if="autoEnabled" class="nest-auto-summary-body mt-3">
                <!-- Provider source picker -->
                <div class="nest-auto-label nest-eyebrow mb-1">
                  {{ t('chat.settings.memory.auto.source') }}
                </div>
                <v-radio-group
                  :model-value="autoSource"
                  hide-details
                  density="compact"
                  @update:model-value="onAutoSourceChange"
                >
                  <v-radio value="wuapi">
                    <template #label>
                      <span class="nest-mono">wuapi</span>
                      <span class="nest-auto-source-hint">
                        · {{ t('chat.settings.memory.auto.wuapiHint') }}
                      </span>
                    </template>
                  </v-radio>
                  <v-radio
                    v-for="k in autoBYOKList"
                    :key="k.id"
                    :value="'byok:' + k.id"
                  >
                    <template #label>
                      <span class="nest-mono">byok</span>
                      <span class="nest-auto-source-hint">
                        · {{ k.label || k.provider }} · {{ k.masked }}
                      </span>
                    </template>
                  </v-radio>
                </v-radio-group>

                <!-- Model picker — dependent on source -->
                <v-select
                  :model-value="autoModel"
                  :items="autoModelItems.map(m => m.id)"
                  :loading="autoModelsLoading"
                  :label="t('chat.settings.memory.auto.model')"
                  density="compact"
                  hide-details
                  class="mt-3 nest-auto-model"
                  @update:model-value="onAutoModelChange"
                />

                <!-- Threshold: slider + numeric input (synced). Range
                     0..2M input tokens. Triggers when prompt reaches
                     this size — see system prompt for details. -->
                <div class="mt-4">
                  <div class="nest-auto-label nest-eyebrow mb-1">
                    {{ t('chat.settings.memory.auto.threshold') }}
                  </div>
                  <p class="nest-hint nest-hint--sm mb-2">
                    {{ t('chat.settings.memory.auto.thresholdHint') }}
                  </p>
                  <div class="d-flex align-center ga-3">
                    <v-slider
                      :model-value="autoThreshold"
                      :min="0"
                      :max="AUTO_THRESHOLD_MAX"
                      :step="1000"
                      hide-details
                      color="primary"
                      density="compact"
                      class="flex-grow-1"
                      @update:model-value="onAutoThresholdChange"
                    />
                    <v-text-field
                      :model-value="autoThreshold"
                      type="number"
                      :min="0"
                      :max="AUTO_THRESHOLD_MAX"
                      :step="1000"
                      density="compact"
                      variant="outlined"
                      hide-details
                      class="nest-auto-threshold-input nest-mono"
                      suffix="tok"
                      @update:model-value="onAutoThresholdInput"
                    />
                  </div>
                </div>

                <!-- Current-cost reminder — same UX pattern as Converter -->
                <p class="nest-hint nest-hint--sm mt-3 nest-auto-cost">
                  {{ t('chat.settings.memory.auto.costHint', { source: autoSourceLabel }) }}
                </p>

                <v-alert
                  v-if="autoSaveError"
                  type="error"
                  variant="tonal"
                  density="compact"
                  class="mt-2 nest-hint"
                  closable
                  @click:close="autoSaveError = null"
                >
                  {{ autoSaveError }}
                </v-alert>
              </div>
            </v-expand-transition>
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
                {{ t('chat.settings.memory.autoLabel') }}
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

// M53 — Author's Note in Memory tab.
.nest-an-block {
  background: var(--nest-bg-elevated);
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
  padding: 10px 12px;

  & > .nest-an-summary {
    cursor: pointer;
    display: flex;
    align-items: center;
    list-style: none;
    &::-webkit-details-marker { display: none; }
  }
  &[open] > .nest-an-summary { color: var(--nest-text); }
}
.nest-an-fields {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}
.nest-saving-hint {
  font-size: 10px;
  color: var(--nest-text-muted);
}

// M51 Sprint 3 wave 2 — tiny preview swatch next to the theme picker.
// Reuses the same swatch data the gallery does (manifest.swatches), so
// the small chip in the drawer stays in sync without a second source
// of truth.
.nest-chat-theme-swatch {
  position: relative;
  flex: 0 0 32px;
  height: 32px;
  border-radius: var(--nest-radius-sm);
  border: 1px solid var(--nest-border);
}
.nest-chat-theme-swatch-dot {
  position: absolute;
  bottom: 4px;
  right: 4px;
  width: 10px;
  height: 10px;
  border-radius: 50%;
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

// ── M44: Auto-summary config panel ──────────────────────────
.nest-auto-summary {
  padding: 12px;
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius);
  background: var(--nest-bg-elevated);
}
.nest-hint--sm {
  font-size: 11px;
  line-height: 1.4;
}
.nest-auto-label {
  // Re-use .nest-eyebrow tone for the small section labels, but
  // without the aggressive letter-spacing.
  color: var(--nest-text-muted);
  letter-spacing: 0.05em;
}
.nest-auto-source-hint {
  color: var(--nest-text-secondary);
  font-size: 13px;
  margin-left: 6px;
}
.nest-auto-model {
  max-width: 100%;
}
.nest-auto-threshold-input {
  max-width: 120px;
}
.nest-auto-threshold-input :deep(input) {
  text-align: right;
}
.nest-auto-cost {
  color: var(--nest-text-secondary);
  font-style: italic;
}
</style>
