<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useChatsStore } from '@/stores/chats'
import { usePresetsStore } from '@/stores/presets'
import type { ChatSamplerMetadata } from '@/api/chats'
import type { Preset, SamplerData } from '@/api/presets'

// Drawer that edits chat_metadata.sampler (per-chat) with preset
// load/save shortcuts. Non-modal so users can see the chat while tweaking.
//
// Layout: core generation knobs stay visible; the long tail of power-user
// fields (top_k, min_p, rep penalty, seed, stop strings, reasoning toggle)
// is collapsed into an "Advanced" section — matches SillyTavern's
// progressive disclosure and keeps casual users from drowning.

const { t } = useI18n()

const props = defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: boolean): void }>()

const chats = useChatsStore()
const presets = usePresetsStore()

// Form mirrors ChatSamplerMetadata. Sliders need concrete numbers (Vuetify
// doesn't accept null), so temperature/top_p are always numeric here.
// Numeric text fields can be cleared — those use `number | null`.
interface Form {
  temperature: number
  top_p: number
  top_k: number | null
  min_p: number | null
  max_tokens: number | null
  frequency_penalty: number | null
  presence_penalty: number | null
  repetition_penalty: number | null
  seed: number | null
  stop: string[]
  reasoning_enabled: boolean | null
  system_prompt: string
}

function defaultForm(): Form {
  return {
    temperature: 1.0,
    top_p: 1.0,
    top_k: null,
    min_p: null,
    max_tokens: null,
    frequency_penalty: null,
    presence_penalty: null,
    repetition_penalty: null,
    seed: null,
    stop: [],
    reasoning_enabled: null,
    system_prompt: '',
  }
}

const form = ref<Form>(defaultForm())
const selectedPresetId = ref<string | null>(null)
const saving = ref(false)
const savedHint = ref(false)
const showAdvanced = ref(false)

// Save-as dialog state
const saveAsOpen = ref(false)
const saveAsName = ref('')
const saveAsBusy = ref(false)
const saveAsError = ref<string | null>(null)

watch(
  () => [props.modelValue, chats.currentChat?.id] as const,
  ([open]) => {
    if (!open) return
    void presets.fetchAll()
    hydrateFromChat()
  },
  { immediate: true },
)

function hydrateFromChat() {
  const s = chats.currentChat?.chat_metadata?.sampler
  if (!s) {
    form.value = defaultForm()
    selectedPresetId.value = null
    return
  }
  form.value = {
    temperature: s.temperature ?? 1.0,
    top_p: s.top_p ?? 1.0,
    top_k: s.top_k ?? null,
    min_p: s.min_p ?? null,
    max_tokens: s.max_tokens ?? null,
    frequency_penalty: s.frequency_penalty ?? null,
    presence_penalty: s.presence_penalty ?? null,
    repetition_penalty: s.repetition_penalty ?? null,
    seed: s.seed ?? null,
    stop: s.stop ? [...s.stop] : [],
    reasoning_enabled: s.reasoning_enabled ?? null,
    system_prompt: s.system_prompt ?? '',
  }
  selectedPresetId.value = s.preset_id ?? null
}

function applyPreset(id: string | null) {
  selectedPresetId.value = id
  if (!id) return
  const p = presets.samplers.find((x: Preset) => x.id === id)
  if (!p) return
  const d = p.data as SamplerData
  form.value = {
    temperature: d.temperature ?? 1.0,
    top_p: d.top_p ?? 1.0,
    top_k: d.top_k ?? null,
    min_p: d.min_p ?? null,
    max_tokens: d.max_tokens ?? null,
    frequency_penalty: d.frequency_penalty ?? null,
    presence_penalty: d.presence_penalty ?? null,
    repetition_penalty: d.repetition_penalty ?? null,
    seed: d.seed ?? null,
    stop: d.stop ? [...d.stop] : [],
    reasoning_enabled: d.reasoning_enabled ?? null,
    system_prompt: d.system_prompt ?? '',
  }
}

function toWire(): ChatSamplerMetadata {
  return {
    temperature: form.value.temperature,
    top_p: form.value.top_p,
    top_k: form.value.top_k,
    min_p: form.value.min_p,
    max_tokens: form.value.max_tokens,
    frequency_penalty: form.value.frequency_penalty,
    presence_penalty: form.value.presence_penalty,
    repetition_penalty: form.value.repetition_penalty,
    seed: form.value.seed,
    stop: form.value.stop.length ? form.value.stop : null,
    reasoning_enabled: form.value.reasoning_enabled,
    system_prompt: form.value.system_prompt.trim() || null,
    preset_id: selectedPresetId.value,
  }
}

async function save() {
  if (!chats.currentChat) return
  saving.value = true
  savedHint.value = false
  try {
    await chats.setSampler(toWire())
    savedHint.value = true
    setTimeout(() => (savedHint.value = false), 1500)
  } finally {
    saving.value = false
  }
}

function reset() {
  form.value = defaultForm()
  selectedPresetId.value = null
}

function openSaveAs() {
  saveAsName.value = ''
  saveAsError.value = null
  saveAsOpen.value = true
}

async function saveAsPreset() {
  const name = saveAsName.value.trim()
  if (!name) {
    saveAsError.value = t('chat.sampler.preset.nameRequired')
    return
  }
  saveAsBusy.value = true
  saveAsError.value = null
  try {
    const data: SamplerData = {
      temperature: form.value.temperature,
      top_p: form.value.top_p,
      top_k: form.value.top_k,
      min_p: form.value.min_p,
      max_tokens: form.value.max_tokens,
      frequency_penalty: form.value.frequency_penalty,
      presence_penalty: form.value.presence_penalty,
      repetition_penalty: form.value.repetition_penalty,
      seed: form.value.seed,
      stop: form.value.stop.length ? form.value.stop : null,
      reasoning_enabled: form.value.reasoning_enabled,
      system_prompt: form.value.system_prompt.trim() || null,
    }
    const created = await presets.createSampler(name, data)
    selectedPresetId.value = created.id
    await chats.setSampler(toWire())
    saveAsOpen.value = false
  } catch (e) {
    saveAsError.value = (e as Error).message
  } finally {
    saveAsBusy.value = false
  }
}

async function deletePreset() {
  if (!selectedPresetId.value) return
  const id = selectedPresetId.value
  await presets.remove(id)
  if (selectedPresetId.value === id) selectedPresetId.value = null
}

const presetOptions = computed(() => [
  { id: null as string | null, name: t('chat.sampler.preset.none') },
  ...presets.samplers.map((p: Preset) => ({ id: p.id, name: p.name })),
])

// Reasoning toggle: tri-state. null = provider default, true = force on,
// false = force off. Displayed as a three-button group.
const reasoningOptions = computed(() => [
  { value: null, label: t('chat.sampler.reasoning.default') },
  { value: true, label: t('chat.sampler.reasoning.on') },
  { value: false, label: t('chat.sampler.reasoning.off') },
])

// Comma-separated text ↔ string[] for the stop-strings input.
const stopText = computed<string>({
  get: () => form.value.stop.join(', '),
  set: v => {
    form.value.stop = v
      .split(',')
      .map(s => s.trim())
      .filter(Boolean)
  },
})

function close() {
  emit('update:modelValue', false)
}
</script>

<template>
  <v-navigation-drawer
    :model-value="modelValue"
    location="right"
    temporary
    width="420"
    class="nest-sampler-drawer"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div class="nest-sampler-head">
      <div class="nest-eyebrow">{{ t('chat.sampler.title') }}</div>
      <v-btn icon="mdi-close" variant="text" size="small" @click="close" />
    </div>

    <div class="nest-sampler-body">
      <!-- Preset picker -->
      <div class="nest-field">
        <label class="nest-field-label">{{ t('chat.sampler.preset.label') }}</label>
        <div class="d-flex ga-2 align-center">
          <v-select
            :model-value="selectedPresetId"
            :items="presetOptions"
            item-title="name"
            item-value="id"
            hide-details
            density="compact"
            :loading="presets.loading"
            style="flex: 1"
            @update:model-value="applyPreset"
          />
          <v-btn
            v-if="selectedPresetId"
            size="small"
            variant="text"
            :title="t('common.delete')"
            icon="mdi-delete-outline"
            @click="deletePreset"
          />
        </div>
      </div>

      <!-- Core knobs -->
      <div class="nest-field">
        <div class="nest-slider-header">
          <label class="nest-field-label">{{ t('chat.sampler.temperature') }}</label>
          <span class="nest-mono nest-field-value">{{ form.temperature.toFixed(2) }}</span>
        </div>
        <v-slider
          v-model="form.temperature"
          :min="0" :max="2" :step="0.05"
          hide-details density="compact" color="primary"
        />
      </div>

      <div class="nest-field">
        <div class="nest-slider-header">
          <label class="nest-field-label">{{ t('chat.sampler.topP') }}</label>
          <span class="nest-mono nest-field-value">{{ form.top_p.toFixed(2) }}</span>
        </div>
        <v-slider
          v-model="form.top_p"
          :min="0" :max="1" :step="0.05"
          hide-details density="compact" color="primary"
        />
      </div>

      <div class="nest-field">
        <label class="nest-field-label">{{ t('chat.sampler.maxTokens') }}</label>
        <v-text-field
          v-model.number="form.max_tokens"
          type="number" :min="0"
          :placeholder="t('chat.sampler.maxTokensPlaceholder')"
          hide-details density="compact" clearable
        />
      </div>

      <div class="nest-field-row">
        <div class="nest-field nest-field-half">
          <label class="nest-field-label">{{ t('chat.sampler.freqPenalty') }}</label>
          <v-text-field
            v-model.number="form.frequency_penalty"
            type="number" :min="-2" :max="2" :step="0.1"
            hide-details density="compact" clearable
          />
        </div>
        <div class="nest-field nest-field-half">
          <label class="nest-field-label">{{ t('chat.sampler.presPenalty') }}</label>
          <v-text-field
            v-model.number="form.presence_penalty"
            type="number" :min="-2" :max="2" :step="0.1"
            hide-details density="compact" clearable
          />
        </div>
      </div>

      <!-- System prompt -->
      <div class="nest-field">
        <label class="nest-field-label">{{ t('chat.sampler.systemPrompt') }}</label>
        <v-textarea
          v-model="form.system_prompt"
          :placeholder="t('chat.sampler.systemPromptPlaceholder')"
          rows="4" auto-grow hide-details density="compact"
        />
        <div class="nest-field-hint">{{ t('chat.sampler.systemPromptHint') }}</div>
      </div>

      <!-- Advanced — collapsed by default -->
      <div class="nest-advanced">
        <button class="nest-advanced-toggle" @click="showAdvanced = !showAdvanced">
          <v-icon size="16" class="mr-1">
            {{ showAdvanced ? 'mdi-chevron-down' : 'mdi-chevron-right' }}
          </v-icon>
          {{ t('chat.sampler.advanced') }}
        </button>

        <div v-show="showAdvanced" class="nest-advanced-body">
          <div class="nest-field-row">
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">
                {{ t('chat.sampler.topK') }}
                <span class="nest-field-hint-inline">{{ t('chat.sampler.topKHint') }}</span>
              </label>
              <v-text-field
                v-model.number="form.top_k"
                type="number" :min="0"
                :placeholder="t('chat.sampler.unset')"
                hide-details density="compact" clearable
              />
            </div>
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">
                {{ t('chat.sampler.minP') }}
                <span class="nest-field-hint-inline">{{ t('chat.sampler.minPHint') }}</span>
              </label>
              <v-text-field
                v-model.number="form.min_p"
                type="number" :min="0" :max="1" :step="0.01"
                :placeholder="t('chat.sampler.unset')"
                hide-details density="compact" clearable
              />
            </div>
          </div>

          <div class="nest-field-row">
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">
                {{ t('chat.sampler.repPenalty') }}
                <span class="nest-field-hint-inline">{{ t('chat.sampler.repPenaltyHint') }}</span>
              </label>
              <v-text-field
                v-model.number="form.repetition_penalty"
                type="number" :min="0.5" :max="2" :step="0.05"
                :placeholder="t('chat.sampler.unset')"
                hide-details density="compact" clearable
              />
            </div>
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">
                {{ t('chat.sampler.seed') }}
                <span class="nest-field-hint-inline">{{ t('chat.sampler.seedHint') }}</span>
              </label>
              <v-text-field
                v-model.number="form.seed"
                type="number"
                :placeholder="t('chat.sampler.seedPlaceholder')"
                hide-details density="compact" clearable
              />
            </div>
          </div>

          <div class="nest-field">
            <label class="nest-field-label">
              {{ t('chat.sampler.stop') }}
              <span class="nest-field-hint-inline">{{ t('chat.sampler.stopHint') }}</span>
            </label>
            <v-text-field
              v-model="stopText"
              :placeholder="t('chat.sampler.stopPlaceholder')"
              hide-details density="compact" clearable
            />
          </div>

          <div class="nest-field">
            <label class="nest-field-label">{{ t('chat.sampler.reasoning.label') }}</label>
            <v-btn-toggle
              v-model="form.reasoning_enabled"
              color="primary"
              mandatory="force"
              density="compact"
              variant="outlined"
            >
              <v-btn
                v-for="opt in reasoningOptions"
                :key="String(opt.value)"
                :value="opt.value"
              >
                {{ opt.label }}
              </v-btn>
            </v-btn-toggle>
            <div class="nest-field-hint">{{ t('chat.sampler.reasoning.hint') }}</div>
          </div>
        </div>
      </div>
    </div>

    <div class="nest-sampler-foot">
      <v-btn variant="text" size="small" @click="reset">
        {{ t('chat.sampler.reset') }}
      </v-btn>
      <v-spacer />
      <v-btn
        variant="outlined" size="small"
        prepend-icon="mdi-content-save-plus-outline"
        @click="openSaveAs"
      >
        {{ t('chat.sampler.saveAs') }}
      </v-btn>
      <v-btn
        color="primary" variant="flat" size="small"
        :loading="saving" @click="save"
      >
        {{ savedHint ? t('chat.sampler.saved') : t('common.save') }}
      </v-btn>
    </div>

    <v-dialog v-model="saveAsOpen" max-width="400">
      <v-card class="nest-saveas">
        <v-card-title>{{ t('chat.sampler.preset.saveAsTitle') }}</v-card-title>
        <v-card-text>
          <v-text-field
            v-model="saveAsName"
            :label="t('chat.sampler.preset.nameLabel')"
            :placeholder="t('chat.sampler.preset.namePlaceholder')"
            autofocus
          />
          <v-alert
            v-if="saveAsError"
            type="error" variant="tonal" density="compact" class="mt-2"
          >
            {{ saveAsError }}
          </v-alert>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" :disabled="saveAsBusy" @click="saveAsOpen = false">
            {{ t('common.cancel') }}
          </v-btn>
          <v-btn
            color="primary" variant="flat"
            :loading="saveAsBusy" @click="saveAsPreset"
          >
            {{ t('chat.sampler.preset.saveBtn') }}
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </v-navigation-drawer>
</template>

<style lang="scss" scoped>
.nest-sampler-drawer {
  background: var(--nest-bg-elevated) !important;
  border-left: 1px solid var(--nest-border) !important;
}

.nest-sampler-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  border-bottom: 1px solid var(--nest-border);
}

.nest-sampler-body {
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 18px;
  overflow-y: auto;
  height: calc(100% - 56px - 56px);
}

.nest-sampler-foot {
  position: sticky;
  bottom: 0;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 10px 12px;
  border-top: 1px solid var(--nest-border);
  background: var(--nest-bg-elevated);
}

.nest-field { display: flex; flex-direction: column; gap: 6px; }
.nest-field-row { display: flex; gap: 12px; }
.nest-field-half { flex: 1; }

.nest-field-label {
  font-family: var(--nest-font-mono);
  font-size: 10.5px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--nest-text-muted);
  display: flex;
  align-items: baseline;
  gap: 6px;
  flex-wrap: wrap;
}
.nest-field-hint-inline {
  text-transform: none;
  letter-spacing: 0;
  font-size: 10px;
  color: var(--nest-text-muted);
  opacity: 0.8;
}
.nest-field-value { font-size: 12px; color: var(--nest-text); }
.nest-field-hint {
  font-size: 11px;
  color: var(--nest-text-muted);
  line-height: 1.4;
}
.nest-slider-header {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
}

.nest-advanced {
  margin-top: 4px;
  border-top: 1px dashed var(--nest-border-subtle);
  padding-top: 14px;
}
.nest-advanced-toggle {
  all: unset;
  display: inline-flex;
  align-items: center;
  font-family: var(--nest-font-mono);
  font-size: 11px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--nest-text-secondary);
  cursor: pointer;
  padding: 4px 0;

  &:hover { color: var(--nest-text); }
}
.nest-advanced-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
  margin-top: 12px;
}

.nest-saveas {
  background: var(--nest-surface) !important;
  border: 1px solid var(--nest-border);
}
</style>
