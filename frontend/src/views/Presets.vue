<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { usePresetsStore } from '@/stores/presets'
import { PRESET_TYPES, type Preset, type PresetType } from '@/api/presets'
import ImportPresetDialog from '@/components/ImportPresetDialog.vue'

const { t } = useI18n()

const presets = usePresetsStore()
const { items, loading, error, defaults } = storeToRefs(presets)

const importOpen = ref(false)
const confirmDeleteId = ref<string | null>(null)
const expandedId = ref<string | null>(null)

onMounted(() => presets.fetchAll())

// Group presets by type, preserving the canonical PRESET_TYPES order so
// the UI layout is stable across reloads.
const groups = computed<Array<{ type: PresetType; items: Preset[] }>>(() =>
  PRESET_TYPES.map(type => ({ type, items: presets.byType(type) })),
)

function typeLabel(t2: PresetType): string {
  return t(`presets.type.${t2}`)
}

function isDefault(p: Preset): boolean {
  return defaults.value[p.type] === p.id
}

async function toggleDefault(p: Preset) {
  if (isDefault(p)) {
    await presets.setDefault(p.type, null)
  } else {
    await presets.setDefault(p.type, p.id)
  }
}

function toggleExpand(id: string) {
  expandedId.value = expandedId.value === id ? null : id
}

function rawJson(p: Preset): string {
  try {
    return JSON.stringify(p.data, null, 2)
  } catch {
    return String(p.data)
  }
}

async function doDelete() {
  if (!confirmDeleteId.value) return
  await presets.remove(confirmDeleteId.value)
  confirmDeleteId.value = null
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString()
}
</script>

<template>
  <v-container class="nest-presets" fluid>
    <!-- Header -->
    <div class="nest-page-head">
      <div>
        <div class="nest-eyebrow">{{ t('presets.title') }}</div>
        <h1 class="nest-h1 mt-1">{{ t('presets.headline') }}</h1>
        <p class="nest-subtitle mt-1">{{ t('presets.tagline') }}</p>
      </div>
      <v-btn
        color="primary"
        variant="flat"
        prepend-icon="mdi-upload"
        @click="importOpen = true"
      >
        {{ t('presets.actions.import') }}
      </v-btn>
    </div>

    <div v-if="loading && !items.length" class="nest-state">
      <v-progress-circular indeterminate color="primary" size="28" />
    </div>
    <v-alert v-else-if="error" type="error" variant="tonal" class="mt-4">{{ error }}</v-alert>

    <!-- Type-grouped lists -->
    <section
      v-for="group in groups"
      :key="group.type"
      class="nest-preset-group"
    >
      <div class="nest-group-header">
        <h2 class="nest-h2">{{ typeLabel(group.type) }}</h2>
        <span class="nest-mono nest-group-count">{{ group.items.length }}</span>
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
              size="x-small"
              variant="text"
              :title="t('presets.actions.view')"
              icon="mdi-code-braces"
              @click="toggleExpand(p.id)"
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
              size="x-small"
              variant="text"
              color="error"
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

    <!-- Import dialog -->
    <ImportPresetDialog v-model="importOpen" />

    <!-- Delete confirmation -->
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
  </v-container>
</template>

<style lang="scss" scoped>
.nest-presets {
  max-width: 900px;
  padding: 32px 24px 60px;
}

.nest-page-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  flex-wrap: wrap;
  gap: 16px;
  margin-bottom: 20px;
}

.nest-state {
  padding: 40px;
  display: grid;
  place-items: center;
}

.nest-preset-group {
  margin-top: 32px;
  padding-top: 20px;
  border-top: 1px solid var(--nest-border);
}
.nest-preset-group:first-of-type {
  border-top: none;
  padding-top: 0;
  margin-top: 20px;
}

.nest-group-header {
  display: flex;
  align-items: baseline;
  gap: 10px;
}
.nest-group-count {
  font-size: 11.5px;
  color: var(--nest-text-muted);
}

.nest-group-empty {
  margin-top: 8px;
  color: var(--nest-text-muted);
  font-size: 13px;
}

.nest-preset-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 10px;
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
