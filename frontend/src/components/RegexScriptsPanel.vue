<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { OpenAIBundleData, RegexScript } from '@/api/presets'
import {
  importRegexScriptsFromFile,
  mergeRegexScripts,
  RegexImportError,
} from '@/lib/regexZipImport'

/**
 * RegexScriptsPanel — editor for bundle.extensions.regex_scripts. ST's
 * regex system lets users define find/replace pairs that run at numbered
 * pipeline stages (1 = user input, 2 = AI output, …). Common uses:
 * jailbreak-style invisible-char insertion (placement 1) and HTML
 * stripping from output (placement 2).
 *
 * v-model carries the bundle; we mutate bundle.extensions.regex_scripts.
 */

const { t } = useI18n()

const props = defineProps<{
  modelValue: OpenAIBundleData
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: OpenAIBundleData): void
}>()

const expandedIdx = ref<number | null>(null)

const scripts = computed<RegexScript[]>(() =>
  props.modelValue.extensions?.regex_scripts ?? [],
)

const enabledCount = computed(() =>
  scripts.value.filter(s => !s.disabled).length,
)
const totalCount = computed(() => scripts.value.length)

const placementOptions = [
  { value: 0, title: 'MD display (legacy)' },
  { value: 1, title: 'user input' },
  { value: 2, title: 'AI output' },
]

function setScripts(next: RegexScript[]) {
  emit('update:modelValue', {
    ...props.modelValue,
    extensions: {
      ...(props.modelValue.extensions ?? {}),
      regex_scripts: next,
    },
  })
}

function update(idx: number, patch: Partial<RegexScript>) {
  const next = [...scripts.value]
  next[idx] = { ...next[idx], ...patch }
  setScripts(next)
}

function toggleDisabled(idx: number) {
  update(idx, { disabled: !scripts.value[idx].disabled })
}

function removeScript(idx: number) {
  setScripts(scripts.value.filter((_, i) => i !== idx))
  if (expandedIdx.value === idx) expandedIdx.value = null
}

function moveUp(idx: number) {
  if (idx <= 0) return
  const next = [...scripts.value]
  ;[next[idx - 1], next[idx]] = [next[idx], next[idx - 1]]
  setScripts(next)
}
function moveDown(idx: number) {
  if (idx >= scripts.value.length - 1) return
  const next = [...scripts.value]
  ;[next[idx], next[idx + 1]] = [next[idx + 1], next[idx]]
  setScripts(next)
}

function addScript() {
  const next: RegexScript = {
    id: cryptoID(),
    scriptName: t('presets.regex.newName'),
    findRegex: '/.*/g',
    replaceString: '',
    placement: [2],
    disabled: false,
  }
  setScripts([...scripts.value, next])
  expandedIdx.value = scripts.value.length
}

function cryptoID(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return crypto.randomUUID()
  }
  return 'r-' + Math.random().toString(36).slice(2, 12)
}

function placementSummary(s: RegexScript): string {
  if (!s.placement || s.placement.length === 0) return '—'
  const labels: string[] = []
  if (s.placement.includes(1)) labels.push(t('presets.regex.placeUser'))
  if (s.placement.includes(2)) labels.push(t('presets.regex.placeAI'))
  return labels.join(', ') || '—'
}

const fileInput = ref<HTMLInputElement | null>(null)
const importing = ref(false)
const importNotice = ref<string | null>(null)
const importError = ref<string | null>(null)
const mergeDialog = ref(false)
const pendingImport = ref<RegexScript[]>([])

function openImportPicker() {
  importError.value = null
  fileInput.value?.click()
}

async function onImportFile(ev: Event) {
  const input = ev.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return

  importing.value = true
  importError.value = null
  importNotice.value = null
  try {
    const imported = await importRegexScriptsFromFile(file)
    if (scripts.value.length > 0) {
      pendingImport.value = imported
      mergeDialog.value = true
    } else {
      applyImport('replace', imported)
    }
  } catch (e) {
    const msg = e instanceof RegexImportError ? e.message : (e as Error).message
    importError.value = t('presets.regex.importError', { msg })
  } finally {
    importing.value = false
  }
}

function applyImport(mode: 'replace' | 'append', imported: RegexScript[]) {
  const next = mergeRegexScripts(scripts.value, imported, mode)
  setScripts(next)
  mergeDialog.value = false
  pendingImport.value = []
  importNotice.value = t('presets.regex.importDone', { n: imported.length })
}

function confirmMergeReplace() {
  applyImport('replace', pendingImport.value)
}

function confirmMergeAppend() {
  applyImport('append', pendingImport.value)
}

function cancelMerge() {
  mergeDialog.value = false
  pendingImport.value = []
}
</script>

<template>
  <div class="nest-regex-panel">
    <div class="nest-rx-head">
      <div class="nest-rx-stats nest-mono">
        {{ t('presets.regex.stats', { enabled: enabledCount, total: totalCount }) }}
      </div>
      <div class="nest-rx-head-actions">
        <v-btn
          size="small"
          variant="outlined"
          prepend-icon="mdi-folder-zip-outline"
          :loading="importing"
          @click="openImportPicker"
        >
          {{ t('presets.regex.importZipBtn') }}
        </v-btn>
        <v-btn
          size="small"
          variant="outlined"
          prepend-icon="mdi-plus"
          @click="addScript"
        >
          {{ t('presets.regex.addBtn') }}
        </v-btn>
      </div>
      <input
        ref="fileInput"
        type="file"
        accept=".zip,.json,application/zip,application/json"
        class="nest-rx-file-input"
        @change="onImportFile"
      >
    </div>

    <v-alert
      v-if="importNotice"
      type="success"
      density="compact"
      variant="tonal"
      class="nest-rx-alert"
      closable
      @click:close="importNotice = null"
    >
      {{ importNotice }}
    </v-alert>
    <v-alert
      v-if="importError"
      type="error"
      density="compact"
      variant="tonal"
      class="nest-rx-alert"
      closable
      @click:close="importError = null"
    >
      {{ importError }}
    </v-alert>

    <div class="nest-rx-hint">
      <v-icon size="14" class="mr-1">mdi-information-outline</v-icon>
      {{ t('presets.regex.hint') }}
    </div>
    <p class="nest-rx-import-hint">{{ t('presets.regex.importZipHint') }}</p>

    <div v-if="totalCount === 0" class="nest-rx-empty">
      <v-icon size="36" color="surface-variant" class="mb-2">mdi-folder-zip-outline</v-icon>
      <p class="nest-rx-empty-text">{{ t('presets.regex.empty') }}</p>
      <v-btn
        color="primary"
        variant="flat"
        prepend-icon="mdi-folder-zip-outline"
        :loading="importing"
        class="mt-3"
        @click="openImportPicker"
      >
        {{ t('presets.regex.importZipBtn') }}
      </v-btn>
    </div>

    <div v-else class="nest-rx-list">
      <div
        v-for="(s, idx) in scripts"
        :key="(s.id ?? '') + idx"
        class="nest-rx-row"
        :class="{ disabled: s.disabled, expanded: expandedIdx === idx }"
      >
        <div class="nest-rx-row-main" @click="expandedIdx = expandedIdx === idx ? null : idx">
          <v-checkbox-btn
            :model-value="!s.disabled"
            color="success"
            density="compact"
            hide-details
            @click.stop
            @update:model-value="toggleDisabled(idx)"
          />
          <div class="nest-rx-name">{{ s.scriptName || t('presets.regex.unnamed') }}</div>
          <v-chip size="x-small" variant="tonal" class="nest-mono">{{ placementSummary(s) }}</v-chip>
          <code class="nest-rx-preview">{{ s.findRegex }}</code>
          <div class="nest-rx-row-actions" @click.stop>
            <v-btn
              size="x-small" variant="text" icon="mdi-chevron-up"
              :disabled="idx === 0" @click="moveUp(idx)"
            />
            <v-btn
              size="x-small" variant="text" icon="mdi-chevron-down"
              :disabled="idx === totalCount - 1" @click="moveDown(idx)"
            />
            <v-btn
              size="x-small" variant="text" color="error"
              icon="mdi-delete-outline" @click="removeScript(idx)"
            />
          </div>
        </div>

        <div v-if="expandedIdx === idx" class="nest-rx-row-edit">
          <div class="nest-field-row">
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">{{ t('presets.regex.nameLabel') }}</label>
              <v-text-field
                :model-value="s.scriptName"
                density="compact" hide-details
                @update:model-value="v => update(idx, { scriptName: v })"
              />
            </div>
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">{{ t('presets.regex.placementLabel') }}</label>
              <v-select
                :model-value="s.placement ?? []"
                :items="placementOptions"
                multiple chips closable-chips
                density="compact" hide-details
                @update:model-value="v => update(idx, { placement: v as number[] })"
              />
            </div>
          </div>
          <div class="nest-field">
            <label class="nest-field-label">
              {{ t('presets.regex.findLabel') }}
              <v-tooltip location="top" :text="t('presets.regex.findHint')">
                <template #activator="{ props: p }">
                  <v-icon v-bind="p" size="12" class="nest-hint-icon">mdi-information-outline</v-icon>
                </template>
              </v-tooltip>
            </label>
            <v-textarea
              :model-value="s.findRegex"
              class="nest-rx-code"
              placeholder="/pattern/gims"
              rows="3"
              auto-grow
              density="compact" hide-details
              @update:model-value="v => update(idx, { findRegex: v })"
            />
          </div>
          <div class="nest-field">
            <label class="nest-field-label">
              {{ t('presets.regex.replaceLabel') }}
              <v-tooltip location="top" :text="t('presets.regex.replaceHint')">
                <template #activator="{ props: p }">
                  <v-icon v-bind="p" size="12" class="nest-hint-icon">mdi-information-outline</v-icon>
                </template>
              </v-tooltip>
            </label>
            <v-textarea
              :model-value="s.replaceString"
              class="nest-rx-code"
              rows="6"
              auto-grow
              density="compact" hide-details
              @update:model-value="v => update(idx, { replaceString: v })"
            />
          </div>
        </div>
      </div>
    </div>

    <v-dialog v-model="mergeDialog" max-width="420">
      <v-card>
        <v-card-title>{{ t('presets.regex.importMergeTitle') }}</v-card-title>
        <v-card-text>
          {{ t('presets.regex.importMergeBody', { n: pendingImport.length, cur: totalCount }) }}
        </v-card-text>
        <v-card-actions>
          <v-btn variant="text" @click="cancelMerge">{{ t('common.cancel') }}</v-btn>
          <v-spacer />
          <v-btn variant="outlined" @click="confirmMergeAppend">{{ t('presets.regex.importMergeAppend') }}</v-btn>
          <v-btn color="primary" variant="flat" @click="confirmMergeReplace">{{ t('presets.regex.importMergeReplace') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<style lang="scss" scoped>
.nest-regex-panel {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.nest-rx-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 2px 4px;
  flex-wrap: wrap;
  gap: 8px;
}
.nest-rx-head-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.nest-rx-file-input {
  display: none;
}
.nest-rx-alert {
  margin: 0 4px;
}
.nest-rx-import-hint {
  font-size: 11px;
  color: var(--nest-text-muted);
  padding: 0 4px 6px;
  margin: 0;
}
.nest-rx-stats {
  font-size: 11.5px;
  color: var(--nest-text-muted);
}

.nest-rx-hint {
  display: flex;
  align-items: center;
  color: var(--nest-text-muted);
  font-size: 11.5px;
  padding: 2px 4px 4px;
}

.nest-rx-empty {
  padding: 24px 16px;
  text-align: center;
  color: var(--nest-text-muted);
  font-size: 13px;
  border: 1px dashed var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
  display: flex;
  flex-direction: column;
  align-items: center;
}
.nest-rx-empty-text {
  margin: 0;
  max-width: 320px;
  line-height: 1.45;
}

.nest-rx-list { display: flex; flex-direction: column; gap: 4px; }
.nest-rx-row {
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
  background: var(--nest-surface);
  transition: border-color var(--nest-transition-fast);

  &:hover:not(.expanded) { border-color: var(--nest-border); }
  &.disabled { opacity: 0.55; }
  &.expanded { border-color: var(--nest-accent); }
}
.nest-rx-row-main {
  display: grid;
  grid-template-columns: auto 1fr auto auto auto;
  gap: 8px;
  align-items: center;
  padding: 6px 10px;
  cursor: pointer;
}
.nest-rx-name {
  font-size: 13px;
  color: var(--nest-text);
  font-weight: 500;
  white-space: nowrap;
}
.nest-rx-preview {
  font-family: var(--nest-font-mono);
  font-size: 11px;
  color: var(--nest-text-muted);
  background: var(--nest-bg-elevated);
  padding: 1px 6px;
  border-radius: 4px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.nest-rx-row-actions { display: flex; gap: 0; }

.nest-rx-row-edit {
  padding: 10px 14px 14px;
  border-top: 1px solid var(--nest-border-subtle);
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.nest-rx-code {
  :deep(input) {
    font-family: var(--nest-font-mono) !important;
    font-size: 12px !important;
  }
}

.nest-field-row { display: flex; gap: 12px; flex-wrap: wrap; }
.nest-field { min-width: 0; }
.nest-field-half { flex: 1 1 220px; }
.nest-field-label {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--nest-text-secondary);
  margin-bottom: 4px;
}
.nest-hint-icon {
  color: var(--nest-text-muted);
  cursor: help;
  opacity: 0.7;
  &:hover { opacity: 1; color: var(--nest-accent); }
}

@media (max-width: 560px) {
  .nest-rx-row-main {
    grid-template-columns: auto 1fr auto;
    grid-template-areas:
      "check name actions"
      "check preview actions"
      "check placement actions";
    row-gap: 2px;
  }
  .nest-rx-preview { grid-area: preview; white-space: normal; }
}
</style>
