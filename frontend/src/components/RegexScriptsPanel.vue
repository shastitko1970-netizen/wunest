<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { OpenAIBundleData, RegexScript } from '@/api/presets'

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

// Placement choices — we only surface the ones our server-side executor
// handles. ST has more (3=slash, 4=WI, 5=reasoning, 6=display) but they
// round-trip through the raw blob without a UI control.
const placementOptions = [
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
  expandedIdx.value = scripts.value.length  // new row is last
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
  // Silent pass-through for 3-6 so the chip stays short.
  return labels.join(', ') || '—'
}
</script>

<template>
  <div class="nest-regex-panel">
    <div class="nest-rx-head">
      <div class="nest-rx-stats nest-mono">
        {{ t('presets.regex.stats', { enabled: enabledCount, total: totalCount }) }}
      </div>
      <v-btn
        size="small"
        variant="outlined"
        prepend-icon="mdi-plus"
        @click="addScript"
      >
        {{ t('presets.regex.addBtn') }}
      </v-btn>
    </div>

    <div class="nest-rx-hint">
      <v-icon size="14" class="mr-1">mdi-information-outline</v-icon>
      {{ t('presets.regex.hint') }}
    </div>

    <div v-if="totalCount === 0" class="nest-rx-empty">
      {{ t('presets.regex.empty') }}
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
            <v-text-field
              :model-value="s.findRegex"
              class="nest-rx-code"
              placeholder="/pattern/gi"
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
            <v-text-field
              :model-value="s.replaceString"
              class="nest-rx-code"
              density="compact" hide-details
              @update:model-value="v => update(idx, { replaceString: v })"
            />
          </div>
        </div>
      </div>
    </div>
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
  padding: 20px;
  text-align: center;
  color: var(--nest-text-muted);
  font-size: 13px;
  border: 1px dashed var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
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
