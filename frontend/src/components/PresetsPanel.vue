<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { usePresetsStore } from '@/stores/presets'
import { PRESET_TYPES, type Preset, type PresetType } from '@/api/presets'
import ImportPresetDialog from '@/components/ImportPresetDialog.vue'
import PresetEditorDialog from '@/components/PresetEditorDialog.vue'

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
const expandedId = ref<string | null>(null)

const editorOpen = ref(false)
const editingPreset = ref<Preset | null>(null)
const editorType = ref<PresetType>('sampler')

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
  editingPreset.value = null
  editorType.value = type
  editorOpen.value = true
}

function openEdit(p: Preset) {
  editingPreset.value = p
  editorOpen.value = true
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

function toggleExpand(id: string) {
  expandedId.value = expandedId.value === id ? null : id
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
    <div v-if="filteredItems.length > 0" class="nest-preset-list">
      <div
        v-for="p in filteredItems"
        :key="p.id"
        class="nest-preset-row"
        :class="{ 'is-active': isActive(p), expanded: expandedId === p.id }"
      >
        <div class="nest-preset-main">
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
          <div class="nest-mono nest-preset-meta">
            {{ formatDate(p.updated_at) }}
          </div>
        </div>

        <div class="nest-preset-actions">
          <!-- The big "Apply" / "Applied ✓" action. Explicit verb
               instead of an abstract v-switch — users kept treating
               the switch as a feature-flag. Clicking when already
               applied deactivates (no active preset of this type). -->
          <v-btn
            v-if="!isActive(p)"
            size="small"
            variant="outlined"
            color="primary"
            prepend-icon="mdi-play-circle-outline"
            @click="apply(p)"
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
            @click="unapply(p)"
          >
            {{ t('presets.actions.applied') }}
          </v-btn>
          <v-btn
            size="x-small" variant="text"
            :title="t('presets.actions.edit')"
            icon="mdi-pencil-outline"
            @click="openEdit(p)"
          />
          <v-btn
            size="x-small" variant="text"
            :title="t('presets.actions.view')"
            icon="mdi-code-braces"
            @click="toggleExpand(p.id)"
          />
          <v-btn
            size="x-small" variant="text"
            :title="t('presets.actions.export')"
            icon="mdi-download-outline"
            @click="exportPreset(p)"
          />
          <v-btn
            size="x-small" variant="text" color="error"
            :title="t('common.delete')"
            icon="mdi-delete-outline"
            @click="confirmDeleteId = p.id"
          />
        </div>

        <pre
          v-if="expandedId === p.id"
          class="nest-preset-json"
        >{{ rawJson(p) }}</pre>
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
    <PresetEditorDialog
      v-model="editorOpen"
      :preset="editingPreset"
      :initial-type="editorType"
    />

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
  &.expanded { grid-template-areas: "main actions" "json json"; }
}

.nest-preset-main {
  grid-area: main;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
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
