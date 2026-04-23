<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useDisplay } from 'vuetify'
import { usePresetsStore } from '@/stores/presets'
import {
  PRESET_TYPES,
  type Preset,
  type PresetType,
  type SamplerData,
  type InstructData,
  type ContextData,
  type SyspromptData,
  type ReasoningData,
} from '@/api/presets'

// Typed preset editor. One dialog for all five types — the form below the
// type picker switches based on type. Field names match SillyTavern's
// preset JSON schema so export/import round-trips with ST users.
//
// Modes:
//   - create: type + name editable, data pre-filled with sane defaults
//   - edit: type locked, name + data mutable
const { t } = useI18n()
const { smAndDown } = useDisplay()
const presets = usePresetsStore()

const props = defineProps<{
  modelValue: boolean
  // When provided → edit. When nil → create.
  preset?: Preset | null
  // Initial type when creating. Ignored in edit mode.
  initialType?: PresetType
}>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'saved', p: Preset): void
}>()

const isEdit = computed(() => !!props.preset)
const type = ref<PresetType>(props.initialType ?? 'sampler')
const name = ref('')
const sampler = ref<SamplerData>({})
const instruct = ref<InstructData>({})
const context = ref<ContextData>({})
const sysprompt = ref<SyspromptData>({})
const reasoning = ref<ReasoningData>({})
const stopText = ref('')   // comma-separated textbox for sampler.stop
const saving = ref(false)
const apiError = ref<string | null>(null)

watch(() => props.modelValue, (open) => {
  if (!open) return
  apiError.value = null
  saving.value = false
  if (props.preset) {
    type.value = props.preset.type
    name.value = props.preset.name
    hydrateFromPreset(props.preset)
  } else {
    type.value = props.initialType ?? 'sampler'
    name.value = ''
    sampler.value = defaultSampler()
    instruct.value = defaultInstruct()
    context.value = defaultContext()
    sysprompt.value = defaultSysprompt()
    reasoning.value = defaultReasoning()
    stopText.value = ''
  }
})

function hydrateFromPreset(p: Preset) {
  const data = p.data as Record<string, any>
  switch (p.type) {
    case 'sampler': {
      const s: SamplerData = { ...defaultSampler(), ...(data as SamplerData) }
      // ST's openai preset stores max_tokens as `openai_max_tokens`. Promote
      // it to `max_tokens` so the slider reflects the imported value.
      if (s.max_tokens == null && typeof data.openai_max_tokens === 'number') {
        s.max_tokens = data.openai_max_tokens
      }
      sampler.value = s
      stopText.value = Array.isArray(data?.stop)
        ? data.stop.join(', ')
        : (Array.isArray(data?.stop_sequence) ? data.stop_sequence.join(', ') : '')
      break
    }
    case 'instruct':
      instruct.value = { ...defaultInstruct(), ...(data as InstructData) }
      break
    case 'context':
      context.value = { ...defaultContext(), ...(data as ContextData) }
      break
    case 'sysprompt': {
      const s = { ...defaultSysprompt(), ...(data as SyspromptData) }
      // Older ST and V3-card fields sometimes use `post_history_instructions`
      // instead of the sysprompt-preset's `post_history`. Surface the first
      // one that has content so edits don't silently lose the value.
      if (!s.post_history && typeof data.post_history_instructions === 'string') {
        s.post_history = data.post_history_instructions
      }
      sysprompt.value = s
      break
    }
    case 'reasoning':
      reasoning.value = { ...defaultReasoning(), ...(data as ReasoningData) }
      break
    case 'openai':
      // Legacy — treat as sampler for editing.
      sampler.value = { ...defaultSampler(), ...(data as SamplerData) }
      break
  }
}

function defaultSampler(): SamplerData {
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
    stop: null,
    reasoning_enabled: null,
    system_prompt: null,
  }
}
function defaultInstruct(): InstructData {
  return {
    input_sequence: '### Instruction:\n',
    output_sequence: '\n### Response:\n',
    system_sequence: '',
    stop_sequence: '',
    activation_regex: '',
    wrap: false,
  }
}
function defaultContext(): ContextData {
  return {
    story_string: '{{#if system}}{{system}}\n{{/if}}{{#if description}}{{char}}\'s description:\n{{description}}\n{{/if}}{{#if scenario}}Scenario:\n{{scenario}}\n{{/if}}',
    chat_start: '***',
    example_separator: '***',
    trim_sentences: false,
    single_line: false,
  }
}
function defaultSysprompt(): SyspromptData {
  return { content: '', post_history: '' }
}
function defaultReasoning(): ReasoningData {
  return { prefix: '<think>', suffix: '</think>', separator: '\n\n' }
}

const typeOptions = computed(() =>
  PRESET_TYPES.map(type => ({
    value: type,
    title: t(`presets.type.${type}`),
  })),
)

// v-slider doesn't accept null — wrap the nullable form fields with a non-null
// view-only computed and write back via explicit handler.
const temperatureView = computed(() => sampler.value.temperature ?? 1)
const topPView = computed(() => sampler.value.top_p ?? 1)

// Preserve any fields the editor doesn't surface (ST-specific knobs etc.)
// by merging our typed state on top of the original data blob. Exports
// then round-trip untouched, even if the user only tweaked one field.
function buildData(): unknown {
  const base = (props.preset?.data ?? {}) as Record<string, unknown>
  switch (type.value) {
    case 'sampler':
    case 'openai': {
      const s: SamplerData = { ...sampler.value }
      const stops = stopText.value.split(',').map(x => x.trim()).filter(Boolean)
      s.stop = stops.length ? stops : null
      return { ...base, ...s }
    }
    case 'instruct':   return { ...base, ...instruct.value }
    case 'context':    return { ...base, ...context.value }
    case 'sysprompt':  return { ...base, ...sysprompt.value }
    case 'reasoning':  return { ...base, ...reasoning.value }
  }
}

async function save() {
  if (!name.value.trim()) {
    apiError.value = t('presets.editor.nameRequired')
    return
  }
  saving.value = true
  apiError.value = null
  try {
    const data = buildData()
    let saved: Preset
    if (props.preset) {
      saved = await presets.update(props.preset.id, { name: name.value.trim(), data })
    } else {
      saved = await presets.create(type.value, name.value.trim(), data)
    }
    emit('saved', saved)
    emit('update:modelValue', false)
  } catch (e) {
    apiError.value = (e as Error).message
  } finally {
    saving.value = false
  }
}

function close() { emit('update:modelValue', false) }
</script>

<template>
  <v-dialog
    :model-value="modelValue"
    :max-width="smAndDown ? undefined : 640"
    :fullscreen="smAndDown"
    scrollable
    @update:model-value="emit('update:modelValue', $event)"
  >
    <v-card class="nest-preset-editor">
      <v-card-title class="nest-pe-title">
        <div>
          <div class="nest-eyebrow">
            {{ isEdit ? t('presets.editor.editTitle') : t('presets.editor.newTitle') }}
          </div>
          <span class="nest-h3 mt-1">
            {{ t(`presets.type.${type}`) }}
          </span>
        </div>
        <v-btn icon="mdi-close" variant="text" size="small" @click="close" />
      </v-card-title>

      <v-card-text class="nest-pe-body">
        <v-alert
          v-if="apiError"
          type="error" variant="tonal" density="compact" class="mb-3"
        >
          {{ apiError }}
        </v-alert>

        <div class="nest-field-row">
          <div class="nest-field nest-field-half">
            <label class="nest-field-label">{{ t('presets.editor.typeLabel') }}</label>
            <v-select
              v-model="type"
              :items="typeOptions"
              item-title="title"
              item-value="value"
              :disabled="isEdit"
              density="compact"
              hide-details
            />
          </div>
          <div class="nest-field nest-field-half">
            <label class="nest-field-label">{{ t('presets.editor.nameLabel') }}</label>
            <v-text-field
              v-model="name"
              :placeholder="t('presets.editor.namePlaceholder')"
              density="compact"
              hide-details
            />
          </div>
        </div>

        <!-- ── Sampler / OpenAI ──────────────────────────────── -->
        <template v-if="type === 'sampler' || type === 'openai'">
          <div class="nest-field-row">
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">
                {{ t('chat.sampler.temperature') }}
                <span class="nest-mono">{{ temperatureView.toFixed(2) }}</span>
              </label>
              <v-slider
                :model-value="temperatureView"
                :min="0" :max="2" :step="0.05"
                hide-details color="primary"
                @update:model-value="v => (sampler.temperature = v)"
              />
            </div>
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">
                {{ t('chat.sampler.topP') }}
                <span class="nest-mono">{{ topPView.toFixed(2) }}</span>
              </label>
              <v-slider
                :model-value="topPView"
                :min="0" :max="1" :step="0.05"
                hide-details color="primary"
                @update:model-value="v => (sampler.top_p = v)"
              />
            </div>
          </div>

          <div class="nest-field-row">
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">{{ t('chat.sampler.topK') }}</label>
              <v-text-field
                v-model.number="sampler.top_k" type="number" :min="0"
                :placeholder="t('chat.sampler.unset')"
                density="compact" hide-details clearable
              />
            </div>
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">{{ t('chat.sampler.minP') }}</label>
              <v-text-field
                v-model.number="sampler.min_p" type="number" :min="0" :max="1" :step="0.01"
                :placeholder="t('chat.sampler.unset')"
                density="compact" hide-details clearable
              />
            </div>
          </div>

          <div class="nest-field-row">
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">{{ t('chat.sampler.maxTokens') }}</label>
              <v-text-field
                v-model.number="sampler.max_tokens" type="number" :min="0"
                :placeholder="t('chat.sampler.maxTokensPlaceholder')"
                density="compact" hide-details clearable
              />
            </div>
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">{{ t('chat.sampler.seed') }}</label>
              <v-text-field
                v-model.number="sampler.seed" type="number"
                :placeholder="t('chat.sampler.seedPlaceholder')"
                density="compact" hide-details clearable
              />
            </div>
          </div>

          <div class="nest-field-row">
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">{{ t('chat.sampler.freqPenalty') }}</label>
              <v-text-field
                v-model.number="sampler.frequency_penalty" type="number"
                :min="-2" :max="2" :step="0.1"
                density="compact" hide-details clearable
              />
            </div>
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">{{ t('chat.sampler.presPenalty') }}</label>
              <v-text-field
                v-model.number="sampler.presence_penalty" type="number"
                :min="-2" :max="2" :step="0.1"
                density="compact" hide-details clearable
              />
            </div>
          </div>

          <div class="nest-field-row">
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">{{ t('chat.sampler.repPenalty') }}</label>
              <v-text-field
                v-model.number="sampler.repetition_penalty" type="number"
                :min="0.5" :max="2" :step="0.05"
                :placeholder="t('chat.sampler.unset')"
                density="compact" hide-details clearable
              />
            </div>
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">{{ t('chat.sampler.stop') }}</label>
              <v-text-field
                v-model="stopText"
                :placeholder="t('chat.sampler.stopPlaceholder')"
                density="compact" hide-details
              />
            </div>
          </div>

          <div class="nest-field">
            <label class="nest-field-label">{{ t('chat.sampler.systemPrompt') }}</label>
            <v-textarea
              v-model="sampler.system_prompt"
              :placeholder="t('chat.sampler.systemPromptPlaceholder')"
              rows="4" auto-grow density="compact" hide-details
            />
          </div>
        </template>

        <!-- ── Instruct ─────────────────────────────────────── -->
        <template v-else-if="type === 'instruct'">
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.editor.instruct.inputSeq') }}</label>
            <v-textarea
              v-model="instruct.input_sequence"
              :placeholder="'[INST] '" rows="2" auto-grow
              density="compact" hide-details variant="outlined"
              class="nest-mono-textarea"
            />
          </div>
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.editor.instruct.outputSeq') }}</label>
            <v-textarea
              v-model="instruct.output_sequence"
              :placeholder="'[/INST] '" rows="2" auto-grow
              density="compact" hide-details variant="outlined"
              class="nest-mono-textarea"
            />
          </div>
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.editor.instruct.systemSeq') }}</label>
            <v-textarea
              v-model="instruct.system_sequence"
              :placeholder="'<<SYS>>\\n'" rows="2" auto-grow
              density="compact" hide-details variant="outlined"
              class="nest-mono-textarea"
            />
          </div>
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.editor.instruct.stopSeq') }}</label>
            <v-text-field
              v-model="instruct.stop_sequence"
              :placeholder="'</s>'"
              density="compact" hide-details
            />
          </div>
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.editor.instruct.activationRegex') }}</label>
            <v-text-field
              v-model="instruct.activation_regex"
              :placeholder="'llama|mistral'"
              density="compact" hide-details
            />
          </div>
          <v-switch
            v-model="instruct.wrap"
            :label="t('presets.editor.instruct.wrap')"
            hide-details color="primary" inset
          />
        </template>

        <!-- ── Context ──────────────────────────────────────── -->
        <template v-else-if="type === 'context'">
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.editor.context.storyString') }}</label>
            <v-textarea
              v-model="context.story_string"
              rows="8" auto-grow
              density="compact" hide-details variant="outlined"
              class="nest-mono-textarea"
            />
            <div class="nest-field-hint">{{ t('presets.editor.context.storyHint') }}</div>
          </div>
          <div class="nest-field-row">
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">{{ t('presets.editor.context.chatStart') }}</label>
              <v-text-field
                v-model="context.chat_start" :placeholder="'***'"
                density="compact" hide-details
              />
            </div>
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">{{ t('presets.editor.context.exampleSep') }}</label>
              <v-text-field
                v-model="context.example_separator" :placeholder="'***'"
                density="compact" hide-details
              />
            </div>
          </div>
          <div class="nest-field-row">
            <v-switch
              v-model="context.trim_sentences"
              :label="t('presets.editor.context.trimSentences')"
              hide-details color="primary" inset
            />
            <v-switch
              v-model="context.single_line"
              :label="t('presets.editor.context.singleLine')"
              hide-details color="primary" inset
            />
          </div>
        </template>

        <!-- ── System prompt ────────────────────────────────── -->
        <template v-else-if="type === 'sysprompt'">
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.editor.sysprompt.content') }}</label>
            <v-textarea
              v-model="sysprompt.content"
              :placeholder="t('presets.editor.sysprompt.contentPlaceholder')"
              rows="10" auto-grow
              density="compact" hide-details variant="outlined"
            />
          </div>
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.editor.sysprompt.postHistory') }}</label>
            <v-textarea
              v-model="sysprompt.post_history"
              :placeholder="t('presets.editor.sysprompt.postHistoryPlaceholder')"
              rows="4" auto-grow
              density="compact" hide-details variant="outlined"
            />
            <div class="nest-field-hint">{{ t('presets.editor.sysprompt.postHistoryHint') }}</div>
          </div>
        </template>

        <!-- ── Reasoning ────────────────────────────────────── -->
        <template v-else-if="type === 'reasoning'">
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.editor.reasoning.prefix') }}</label>
            <v-text-field
              v-model="reasoning.prefix" :placeholder="'<think>'"
              density="compact" hide-details
              class="nest-mono-textarea"
            />
          </div>
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.editor.reasoning.suffix') }}</label>
            <v-text-field
              v-model="reasoning.suffix" :placeholder="'</think>'"
              density="compact" hide-details
              class="nest-mono-textarea"
            />
          </div>
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.editor.reasoning.separator') }}</label>
            <v-text-field
              v-model="reasoning.separator" :placeholder="'\\n\\n'"
              density="compact" hide-details
              class="nest-mono-textarea"
            />
            <div class="nest-field-hint">{{ t('presets.editor.reasoning.separatorHint') }}</div>
          </div>
        </template>
      </v-card-text>

      <v-card-actions class="px-6 pb-4">
        <v-spacer />
        <v-btn variant="text" :disabled="saving" @click="close">
          {{ t('common.cancel') }}
        </v-btn>
        <v-btn
          color="primary" variant="flat"
          :loading="saving"
          :disabled="!name.trim()"
          @click="save"
        >
          {{ isEdit ? t('common.save') : t('presets.actions.create') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<style lang="scss" scoped>
.nest-preset-editor {
  background: var(--nest-surface) !important;
  border: 1px solid var(--nest-border);
  border-radius: var(--nest-radius) !important;
}
.nest-pe-title {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  padding: 20px 20px 8px;
}
.nest-pe-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-height: 70vh;
  overflow-y: auto;
}
.nest-field { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
.nest-field-row { display: flex; gap: 12px; flex-wrap: wrap; }
.nest-field-half { flex: 1 1 180px; min-width: 0; }

// Dialog + its rows squeeze onto iPhone SE cleanly.
@media (max-width: 480px) {
  .nest-field-half { flex: 1 1 100%; }
  .nest-pe-body { padding-left: 16px; padding-right: 16px; }
}
.nest-field-label {
  display: flex;
  justify-content: space-between;
  font-family: var(--nest-font-mono);
  font-size: 10.5px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--nest-text-muted);
}
.nest-field-hint {
  font-size: 11px;
  color: var(--nest-text-muted);
  line-height: 1.4;
}
.nest-mono-textarea :deep(textarea),
.nest-mono-textarea :deep(input) {
  font-family: var(--nest-font-mono);
  font-size: 12.5px;
}
</style>
