<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { usePresetsStore } from '@/stores/presets'
import { PRESET_TYPES, type Preset, type PresetType } from '@/api/presets'
import ImportPresetDialog from '@/components/ImportPresetDialog.vue'
import PresetEditorDialog from '@/components/PresetEditorDialog.vue'

// PresetsPanel — the whole "templates" experience, embedded inside a
// Library tab. Previously lived at /presets as its own page; pulled into
// Library so users have one place for everything they create/import.

const { t } = useI18n()

const presets = usePresetsStore()
const { items, loading, error, defaults } = storeToRefs(presets)

const importOpen = ref(false)
const confirmDeleteId = ref<string | null>(null)
const expandedId = ref<string | null>(null)

const editorOpen = ref(false)
const editingPreset = ref<Preset | null>(null)
const editorType = ref<PresetType>('sampler')

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

const groups = computed<Array<{ type: PresetType; items: Preset[] }>>(() =>
  PRESET_TYPES.map(type => ({ type, items: presets.byType(type) })),
)

function typeLabel(tp: PresetType): string { return t(`presets.type.${tp}`) }
function isDefault(p: Preset): boolean { return defaults.value[p.type] === p.id }

async function toggleDefault(p: Preset) {
  if (isDefault(p)) await presets.setDefault(p.type, null)
  else await presets.setDefault(p.type, p.id)
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
</script>

<template>
  <div class="nest-presets-panel">
    <!-- Inline header: small, tab-internal framing (no page hero). -->
    <div class="nest-panel-head">
      <div class="nest-panel-head-text">
        <h3 class="nest-h3">{{ t('presets.headline') }}</h3>
        <p class="nest-subtitle">{{ t('presets.tagline') }}</p>
      </div>
      <v-btn
        color="primary"
        variant="flat"
        prepend-icon="mdi-upload"
        size="small"
        @click="importOpen = true"
      >
        {{ t('presets.actions.import') }}
      </v-btn>
    </div>

    <div v-if="loading && !items.length" class="nest-state">
      <v-progress-circular indeterminate color="primary" size="28" />
    </div>
    <v-alert v-else-if="error" type="error" variant="tonal" class="mt-4">{{ error }}</v-alert>

    <section
      v-for="group in groups"
      :key="group.type"
      class="nest-preset-group"
    >
      <div class="nest-group-header">
        <h4 class="nest-h4 mb-0">{{ typeLabel(group.type) }}</h4>
        <span class="nest-mono nest-group-count">{{ group.items.length }}</span>
        <v-spacer />
        <v-btn
          size="small"
          variant="outlined"
          prepend-icon="mdi-plus"
          @click="openCreate(group.type)"
        >
          {{ t('presets.actions.newForType') }}
        </v-btn>
      </div>

      <div v-if="group.items.length === 0" class="nest-group-empty">
        {{ t('presets.empty', { type: typeLabel(group.type) }) }}
      </div>

      <div v-else class="nest-preset-list">
        <div
          v-for="p in group.items"
          :key="p.id"
          class="nest-preset-row"
          :class="{ expanded: expandedId === p.id }"
        >
          <div class="nest-preset-main">
            <div class="nest-preset-title">
              <span class="nest-preset-name">{{ p.name }}</span>
              <v-chip
                v-if="isDefault(p)"
                size="x-small"
                color="secondary"
                variant="tonal"
                class="nest-mono ml-2"
              >
                {{ t('presets.defaultBadge') }}
              </v-chip>
            </div>
            <div class="nest-mono nest-preset-meta">
              {{ formatDate(p.updated_at) }}
            </div>
          </div>

          <div class="nest-preset-actions">
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
              size="x-small"
              :variant="isDefault(p) ? 'tonal' : 'text'"
              :color="isDefault(p) ? 'secondary' : undefined"
              :title="isDefault(p) ? t('presets.actions.unsetDefault') : t('presets.actions.setDefault')"
              :icon="isDefault(p) ? 'mdi-star' : 'mdi-star-outline'"
              @click="toggleDefault(p)"
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
    </section>

    <ImportPresetDialog v-model="importOpen" />
    <PresetEditorDialog
      v-model="editorOpen"
      :preset="editingPreset"
      :initial-type="editorType"
    />

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
  margin-bottom: 8px;
  flex-wrap: wrap;
}
.nest-panel-head-text .nest-subtitle {
  font-size: 13px;
  margin: 4px 0 0;
}

.nest-state {
  padding: 40px;
  display: grid;
  place-items: center;
}

.nest-preset-group {
  margin-top: 24px;
  padding-top: 16px;
  border-top: 1px solid var(--nest-border-subtle);
}
.nest-preset-group:first-of-type {
  border-top: none;
  padding-top: 0;
  margin-top: 12px;
}

.nest-group-header {
  display: flex;
  align-items: baseline;
  gap: 10px;
  margin-bottom: 6px;
}
.nest-group-count {
  font-size: 11.5px;
  color: var(--nest-text-muted);
}

.nest-group-empty {
  margin-top: 6px;
  color: var(--nest-text-muted);
  font-size: 13px;
}

.nest-preset-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 8px;
}

.nest-preset-row {
  display: grid;
  grid-template-columns: 1fr auto;
  grid-template-areas: "main actions";
  gap: 10px;
  padding: 10px 12px;
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
  background: var(--nest-surface);
  align-items: center;
  transition: border-color var(--nest-transition-fast);

  &:hover { border-color: var(--nest-border); }
  &.expanded { grid-template-areas: "main actions" "json json"; }
}

.nest-preset-main { grid-area: main; min-width: 0; }

.nest-preset-title {
  display: flex;
  align-items: center;
  font-size: 14px;
  color: var(--nest-text);
}

.nest-preset-name { font-weight: 500; }

.nest-preset-meta {
  font-size: 11px;
  color: var(--nest-text-muted);
  margin-top: 2px;
}

.nest-preset-actions {
  grid-area: actions;
  display: flex;
  gap: 2px;
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

.nest-confirm {
  background: var(--nest-surface) !important;
  border: 1px solid var(--nest-border);
}
</style>
