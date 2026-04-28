<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { usePresetsStore } from '@/stores/presets'
import { PRESET_TYPES, type Preset, type PresetType } from '@/api/presets'
import ImportPresetDialog from '@/components/ImportPresetDialog.vue'
import PresetEditorForm from '@/components/PresetEditorForm.vue'
import UsageHintChip from '@/components/UsageHintChip.vue'

/**
 * PresetsPanel — flat list of every preset the user has, with filter chips
 * per type and an "Active" toggle that immediately applies the preset to
 * all chats (M30 "variant-1" semantics: one active per type, global).
 *
 * Replaces the earlier grouped layout with five per-type empty states
 * ("Нет сохранённых шаблонов типа…") which was mostly whitespace on a
 * fresh account. Now empty = one message + two CTAs.
 */

const { t } = useI18n()

const presets = usePresetsStore()
const { items, loading, error } = storeToRefs(presets)

const importOpen = ref(false)
const confirmDeleteId = ref<string | null>(null)
// `expandedId` tracks which row is showing its inline editor. Unique
// sentinel 'new' means "the new-preset draft row is open".
const expandedId = ref<string | null>(null)
// When creating a new preset, which type is pre-selected in the draft row.
const newDraftType = ref<PresetType>('sampler')
// For the "view raw JSON" read-only fold (the {} button). Separate from
// the editor because they're different intents (inspect vs tune).
const jsonViewId = ref<string | null>(null)

// Filter: "all" shows every preset, or one of PRESET_TYPES narrows to just
// that type. Chip bar mirrors this.
const filter = ref<PresetType | 'all'>('all')

// Snackbar feedback — visible confirmation that "Apply" / "Imported" /
// "Saved" actually did something. Users kept missing the subtle
// border-highlight on the active row, so a corner toast tells them
// explicitly what just happened.
const snack = ref<{ open: boolean; text: string; color: string }>({
  open: false,
  text: '',
  color: 'success',
})
function toast(text: string, color: string = 'success') {
  snack.value = { open: true, text, color }
}

onMounted(() => presets.fetchAll())

function openCreate(type: PresetType) {
  newDraftType.value = type
  expandedId.value = 'new'
  // Close any open "view JSON" fold so the row isn't double-expanded.
  jsonViewId.value = null
}

function openEdit(p: Preset) {
  expandedId.value = expandedId.value === p.id ? null : p.id
  jsonViewId.value = null
}

function onEditorSaved(preset: Preset, activated: boolean) {
  if (expandedId.value === 'new') {
    // New preset just created — close the draft row and confirm.
    expandedId.value = null
    toast(
      activated
        ? t('presets.snack.createdAndApplied', { name: preset.name })
        : t('presets.snack.created', { name: preset.name }),
      activated ? 'success' : 'info',
    )
  } else {
    // Existing preset updated.
    expandedId.value = null
    toast(t('presets.snack.saved', { name: preset.name }))
  }
}

function onEditorCancelled() {
  expandedId.value = null
}

// Export preset as ST-compatible JSON download.
function exportPreset(p: Preset) {
  const payload = { name: p.name, ...(p.data as Record<string, unknown>) }
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `${p.name.replace(/[^a-z0-9-_]+/gi, '_')}.json`
  a.click()
  URL.revokeObjectURL(url)
}

const filteredItems = computed<Preset[]>(() => {
  if (filter.value === 'all') return items.value
  return items.value.filter(p => p.type === filter.value)
})

// Typed counts for the filter bar chips. Computed once per preset list
// change rather than per-chip render.
const typeCounts = computed<Record<string, number>>(() => {
  const c: Record<string, number> = { all: items.value.length }
  for (const tp of PRESET_TYPES) c[tp] = 0
  for (const p of items.value) c[p.type] = (c[p.type] ?? 0) + 1
  return c
})

function typeLabel(tp: PresetType): string { return t(`presets.type.${tp}`) }

// Visual accent per preset type — makes the chip glanceable at a distance
// (what kind of preset this row represents) without reading the label.
function typeColor(tp: PresetType): string {
  switch (tp) {
    case 'sampler':   return 'primary'
    case 'instruct':  return 'purple'
    case 'context':   return 'orange'
    case 'sysprompt': return 'success'
    case 'reasoning': return 'teal'
    case 'openai':    return 'secondary'
    default:          return ''
  }
}

function isActive(p: Preset): boolean { return presets.isActive(p) }

async function apply(p: Preset) {
  // Exclusive per type: setActive auto-clears the previous active one.
  await presets.setActive(p.type, p.id)
  toast(t('presets.snack.applied', { name: p.name }))
}

async function unapply(p: Preset) {
  // User explicitly deactivated — no active preset for this type after.
  await presets.setActive(p.type, null)
  toast(t('presets.snack.unapplied', { name: p.name }), 'info')
}

// Called by the import dialog. `activated` is true when the store
// auto-activated the new preset (no prior active preset of that type).
function onImported(_id: string, activated: boolean) {
  if (activated) toast(t('presets.snack.importedAndApplied'))
  else toast(t('presets.snack.imported'), 'info')
}

function toggleJsonView(id: string) {
  jsonViewId.value = jsonViewId.value === id ? null : id
  // Opening the read-only JSON view closes any open editor on the same row.
  if (jsonViewId.value && expandedId.value === id) expandedId.value = null
}

function rawJson(p: Preset): string {
  try { return JSON.stringify(p.data, null, 2) } catch { return String(p.data) }
}

async function doDelete() {
  if (!confirmDeleteId.value) return
  await presets.remove(confirmDeleteId.value)
  confirmDeleteId.value = null
}

function formatDate(iso: string): string { return new Date(iso).toLocaleDateString() }

/**
 * Summarize the content of a sampler/openai preset in one short label —
 * "111 prompts · 4 regex · prefill" — so the row shows at a glance that
 * an imported ST preset carries rich internals the user can tune. Non-
 * sampler types return empty (their internals are already just a handful
 * of fields; no point advertising "3 fields").
 */
function summarizeBundle(p: Preset): string {
  if (p.type !== 'sampler' && p.type !== 'openai') return ''
  const data = (p.data ?? {}) as Record<string, any>
  const parts: string[] = []
  const promptOrder = Array.isArray(data.prompt_order) ? data.prompt_order : []
  if (promptOrder.length > 0) {
    const wildcard = promptOrder.find((g: any) => g?.character_id === 100001) ?? promptOrder[0]
    const order = Array.isArray(wildcard?.order) ? wildcard.order : []
    if (order.length > 0) {
      parts.push(t('presets.rowSummary.prompts', { n: order.length }))
    }
  }
  const regex = data.extensions?.regex_scripts
  if (Array.isArray(regex) && regex.length > 0) {
    parts.push(t('presets.rowSummary.regex', { n: regex.length }))
  }
  if (typeof data.assistant_prefill === 'string' && data.assistant_prefill.trim()) {
    parts.push(t('presets.rowSummary.prefill'))
  }
  return parts.join(' · ')
}

// When the user clicks "New" with a specific type filter active, pre-pick
// that type in the editor. "All" filter falls through to sampler (the most
// common type users create first).
function createForCurrentFilter() {
  if (filter.value === 'all') openCreate('sampler')
  else openCreate(filter.value)
}
</script>

<template>
  <div class="nest-presets-panel">
    <!-- Header -->
    <div class="nest-panel-head">
      <div class="nest-panel-head-text">
        <h3 class="nest-h3">{{ t('presets.headline') }}</h3>
        <p class="nest-subtitle">{{ t('presets.tagline') }}</p>
      </div>
      <div class="nest-panel-head-actions">
        <v-btn
          variant="outlined"
          prepend-icon="mdi-upload"
          size="small"
          @click="importOpen = true"
        >
          {{ t('presets.actions.import') }}
        </v-btn>
        <v-menu offset="4">
          <template #activator="{ props: menuProps }">
            <v-btn
              v-bind="menuProps"
              color="primary"
              variant="flat"
              prepend-icon="mdi-plus"
              append-icon="mdi-menu-down"
              size="small"
            >
              {{ t('presets.actions.new') }}
            </v-btn>
          </template>
          <v-list density="compact" min-width="200">
            <v-list-item
              v-for="tp in PRESET_TYPES"
              :key="tp"
              @click="openCreate(tp)"
            >
              <v-list-item-title>{{ typeLabel(tp) }}</v-list-item-title>
            </v-list-item>
          </v-list>
        </v-menu>
      </div>
    </div>

    <!-- M54.3 — slot usage hint. Total across all preset types. -->
    <UsageHintChip :used="items.length" class="mb-2" />

    <div v-if="loading && !items.length" class="nest-state">
      <v-progress-circular indeterminate color="primary" size="28" />
    </div>
    <v-alert v-else-if="error" type="error" variant="tonal" class="mt-4">{{ error }}</v-alert>

    <!-- Filter bar — only relevant when the user has at least one preset. -->
    <div v-if="items.length > 0" class="nest-filter-bar">
      <v-chip
        size="small"
        :variant="filter === 'all' ? 'flat' : 'outlined'"
        :color="filter === 'all' ? 'primary' : ''"
        @click="filter = 'all'"
      >
        {{ t('presets.filter.all') }}
        <span class="nest-mono ml-1">{{ typeCounts.all }}</span>
      </v-chip>
      <v-chip
        v-for="tp in PRESET_TYPES"
        :key="tp"
        size="small"
        :variant="filter === tp ? 'flat' : 'outlined'"
        :color="filter === tp ? typeColor(tp) : ''"
        :disabled="typeCounts[tp] === 0"
        @click="filter = tp"
      >
        {{ typeLabel(tp) }}
        <span class="nest-mono ml-1">{{ typeCounts[tp] ?? 0 }}</span>
      </v-chip>
    </div>

    <!-- Flat list of presets. -->
    <div v-if="filteredItems.length > 0 || expandedId === 'new'" class="nest-preset-list">
      <!-- "Draft" row for a brand-new preset. Appears at the top when
           user clicked a New menu entry. Contains just the inline
           editor — no row metadata since nothing exists yet. -->
      <div
        v-if="expandedId === 'new'"
        class="nest-preset-row is-draft"
      >
        <PresetEditorForm
          :preset="null"
          :initial-type="newDraftType"
          @saved="onEditorSaved"
          @cancelled="onEditorCancelled"
        />
      </div>
      <div
        v-for="p in filteredItems"
        :key="p.id"
        class="nest-preset-row"
        :class="{
          'is-active': isActive(p),
          editing: expandedId === p.id,
          'json-open': jsonViewId === p.id,
        }"
      >
        <!-- Main click target — whole row (minus action buttons) toggles
             the inline editor. Having click-only-pencil surfaces was
             hiding the editor behind a tiny tap-target most users never
             noticed. -->
        <div class="nest-preset-main" @click="openEdit(p)">
          <div class="nest-preset-title">
            <v-chip
              size="x-small"
              :color="typeColor(p.type)"
              variant="tonal"
              class="nest-preset-typechip nest-mono"
            >
              {{ typeLabel(p.type) }}
            </v-chip>
            <span class="nest-preset-name">{{ p.name }}</span>
            <v-chip
              v-if="isActive(p)"
              size="x-small"
              color="success"
              variant="flat"
              prepend-icon="mdi-check"
              class="nest-mono ml-1"
            >
              {{ t('presets.activeBadge') }}
            </v-chip>
          </div>
          <!-- Bundle summary + date. Sampler/openai rows carrying a
               Prompt Manager or regex scripts surface that in plain
               text so users never miss "111 prompts" is in there. -->
          <div class="nest-preset-meta nest-mono">
            <span v-if="summarizeBundle(p)" class="nest-preset-summary">
              {{ summarizeBundle(p) }}
            </span>
            <span v-if="summarizeBundle(p)" class="nest-preset-sep">·</span>
            <span>{{ formatDate(p.updated_at) }}</span>
          </div>
        </div>

        <!-- Action buttons. @click.stop on each so clicking a button
             doesn't also fire the row-open handler on nest-preset-main. -->
        <div class="nest-preset-actions" @click.stop>
          <v-btn
            v-if="!isActive(p)"
            size="small"
            variant="outlined"
            color="primary"
            prepend-icon="mdi-play-circle-outline"
            @click.stop="apply(p)"
          >
            {{ t('presets.actions.apply') }}
          </v-btn>
          <v-btn
            v-else
            size="small"
            variant="flat"
            color="success"
            prepend-icon="mdi-check-circle"
            :title="t('presets.actions.clickToDeactivate')"
            @click.stop="unapply(p)"
          >
            {{ t('presets.actions.applied') }}
          </v-btn>
          <v-btn
            size="x-small" variant="text"
            :title="expandedId === p.id ? t('common.close') : t('presets.actions.edit')"
            :icon="expandedId === p.id ? 'mdi-close' : 'mdi-pencil-outline'"
            :color="expandedId === p.id ? 'primary' : undefined"
            @click.stop="openEdit(p)"
          />
          <v-btn
            size="x-small" variant="text"
            :title="t('presets.actions.view')"
            icon="mdi-code-braces"
            :color="jsonViewId === p.id ? 'primary' : undefined"
            @click.stop="toggleJsonView(p.id)"
          />
          <v-btn
            size="x-small" variant="text"
            :title="t('presets.actions.export')"
            icon="mdi-download-outline"
            @click.stop="exportPreset(p)"
          />
          <v-btn
            size="x-small" variant="text" color="error"
            :title="t('common.delete')"
            icon="mdi-delete-outline"
            @click.stop="confirmDeleteId = p.id"
          />
        </div>

        <!-- Read-only JSON inspector. Kept for users who want to SEE
             what's under the hood without opening the editor. -->
        <pre
          v-if="jsonViewId === p.id"
          class="nest-preset-json"
        >{{ rawJson(p) }}</pre>

        <!-- Inline editor (P3) — lives inside the row so the rest of
             the list stays visible. No modal, no scroll trap. -->
        <div v-if="expandedId === p.id" class="nest-preset-edit">
          <PresetEditorForm
            :preset="p"
            @saved="onEditorSaved"
            @cancelled="onEditorCancelled"
          />
        </div>
      </div>
    </div>

    <!-- Single unified empty state — replaces five per-group
         "Нет сохранённых шаблонов типа Х" messages. -->
    <div
      v-else-if="!loading"
      class="nest-preset-empty"
    >
      <v-icon size="40" color="surface-variant">mdi-tune-vertical-variant</v-icon>
      <h4 class="nest-h4 mt-3">
        {{ filter === 'all'
          ? t('presets.emptyAll.title')
          : t('presets.emptyType.title', { type: typeLabel(filter as PresetType) }) }}
      </h4>
      <p class="nest-subtitle mt-1">
        {{ filter === 'all' ? t('presets.emptyAll.body') : t('presets.emptyType.body') }}
      </p>
      <div class="nest-empty-cta mt-4">
        <v-btn
          variant="outlined"
          prepend-icon="mdi-upload"
          @click="importOpen = true"
        >
          {{ t('presets.actions.import') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="flat"
          prepend-icon="mdi-plus"
          @click="createForCurrentFilter"
        >
          {{ t('presets.actions.newForType') }}
        </v-btn>
      </div>
    </div>

    <ImportPresetDialog v-model="importOpen" @imported="onImported" />

    <!-- Confirmation toast — fires on apply / unapply / import. Bottom-
         right position keeps it out of the way of the Action buttons. -->
    <v-snackbar
      v-model="snack.open"
      :color="snack.color"
      :timeout="2400"
      location="bottom right"
    >
      {{ snack.text }}
    </v-snackbar>

    <v-dialog
      :model-value="confirmDeleteId !== null"
      max-width="400"
      @update:model-value="v => !v && (confirmDeleteId = null)"
    >
      <v-card class="nest-confirm">
        <v-card-title>{{ t('presets.delete.title') }}</v-card-title>
        <v-card-text>{{ t('presets.delete.body') }}</v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="confirmDeleteId = null">{{ t('common.cancel') }}</v-btn>
          <v-btn color="error" variant="flat" @click="doDelete">{{ t('common.delete') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<style lang="scss" scoped>
.nest-presets-panel {
  max-width: 900px;
  padding-bottom: 40px;
}

.nest-panel-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}
.nest-panel-head-text .nest-subtitle {
  font-size: 13px;
  margin: 4px 0 0;
}
.nest-panel-head-actions {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
}

.nest-state {
  padding: 40px;
  display: grid;
  place-items: center;
}

// Filter chip bar — horizontal, wraps on narrow viewports.
.nest-filter-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin: 8px 0 16px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--nest-border-subtle);
}

// Flat list.
.nest-preset-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.nest-preset-row {
  display: grid;
  grid-template-columns: 1fr auto;
  grid-template-areas: "main actions";
  gap: 10px;
  padding: 12px 14px;
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
  background: var(--nest-surface);
  align-items: center;
  transition: border-color var(--nest-transition-fast), background var(--nest-transition-fast);

  &:hover {
    border-color: var(--nest-border);
  }
  &.is-active {
    border-color: var(--nest-accent);
    background: color-mix(in srgb, var(--nest-accent) 6%, var(--nest-surface));
  }

  // States where the row expands to show content below the
  // name/actions line — editor or read-only JSON.
  &.json-open   { grid-template-areas: "main actions" "json json"; }
  &.editing {
    grid-template-areas: "main actions" "edit edit";
    border-color: var(--nest-accent);
  }

  // Draft row (for a new preset) is all-editor, no name/actions line.
  &.is-draft {
    display: block;
    border-color: var(--nest-accent);
    border-style: dashed;
    padding: 14px 16px;
  }
}

.nest-preset-main {
  grid-area: main;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
  cursor: pointer;
  // A whisper of feedback on row click — the border on the parent row
  // already lights up when `editing` class is active, so no extra hover
  // effect is needed here; just advertise it's clickable.
}

.nest-preset-title {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  font-size: 14px;
  color: var(--nest-text);
}

.nest-preset-typechip {
  text-transform: lowercase;
}

.nest-preset-name {
  font-weight: 500;
}

.nest-preset-meta {
  font-size: 11px;
  color: var(--nest-text-muted);
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

// Bundle summary ("111 prompts · 4 regex · prefill") — subtly highlighted
// so users see it's the "what's in this preset" indicator.
.nest-preset-summary {
  color: var(--nest-text-secondary);
  font-weight: 500;
}
.nest-preset-sep {
  color: var(--nest-text-muted);
  opacity: 0.5;
}

.nest-preset-actions {
  grid-area: actions;
  display: flex;
  gap: 2px;
  align-items: center;

  // Tighter switch so it sits nicely next to icon buttons.
  :deep(.v-switch__track) {
    min-width: 32px !important;
  }
}

.nest-preset-json {
  grid-area: json;
  margin: 8px 0 0;
  padding: 10px 12px;
  background: var(--nest-bg);
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
  font-family: var(--nest-font-mono);
  font-size: 11.5px;
  line-height: 1.55;
  color: var(--nest-text-secondary);
  overflow-x: auto;
  white-space: pre;
}

// Inline editor container — sits below the row's name/actions line
// when .editing is active. Separator line on top to visually detach
// it from the row metadata.
.nest-preset-edit {
  grid-area: edit;
  margin-top: 10px;
  padding-top: 12px;
  border-top: 1px solid var(--nest-border-subtle);
}

// Unified empty state — one central plate, two CTAs.
.nest-preset-empty {
  text-align: center;
  padding: 56px 24px;
  border: 1px dashed var(--nest-border-subtle);
  border-radius: var(--nest-radius);
  background: var(--nest-surface);
  margin-top: 8px;
}
.nest-empty-cta {
  display: inline-flex;
  gap: 10px;
  flex-wrap: wrap;
  justify-content: center;
}

.nest-confirm {
  background: var(--nest-surface) !important;
  border: 1px solid var(--nest-border);
}

// Mobile — let the actions row wrap under the main text instead of
// overlapping it on narrow screens.
@media (max-width: 560px) {
  .nest-preset-row {
    grid-template-columns: 1fr;
    grid-template-areas: "main" "actions";
    &.expanded { grid-template-areas: "main" "actions" "json"; }
  }
  .nest-preset-actions {
    justify-content: flex-end;
    flex-wrap: wrap;
  }
}
</style>
