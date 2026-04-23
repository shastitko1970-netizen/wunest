<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
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
  type OpenAIBundleData,
} from '@/api/presets'
import PromptManagerPanel from '@/components/PromptManagerPanel.vue'
import RegexScriptsPanel from '@/components/RegexScriptsPanel.vue'
import AdvancedBundlePanel from '@/components/AdvancedBundlePanel.vue'

/**
 * PresetEditorForm — the typed editor surface for a single preset. Used
 * inline inside PresetsPanel row-expand (primary) and optionally wrappable
 * in a dialog. Replaces the 597-line PresetEditorDialog with something
 * less overwhelming:
 *
 *  P1. Progressive disclosure — core fields always visible, advanced /
 *      penalties / reasoning behind v-expansion-panels so first-time
 *      users aren't drowned in 11 sliders.
 *  P2. Tooltips + recommended-range hints on every field so new users
 *      understand what each knob does ("temperature 0 = deterministic,
 *      1.2 is the roleplay sweet-spot").
 *  P3. Inline-expand (lives inside the row, not a modal) so users see
 *      the preset they're editing IN the list context.
 *  P4. Raw-JSON tab — power users can edit ST-specific fields (mirostat,
 *      tfs, dry_*) that our form doesn't surface. Unknown fields on the
 *      imported preset are preserved either way; this makes them
 *      tunable instead of just round-trippable.
 *  P5. Starter templates — creative / balanced / deterministic /
 *      reasoning one-click presets to fill sensible defaults when
 *      building a sampler from scratch.
 */

const { t } = useI18n()
const presets = usePresetsStore()

const props = defineProps<{
  /** Existing preset = edit mode. null/undefined = create. */
  preset?: Preset | null
  /** Initial type when creating. Ignored in edit mode. */
  initialType?: PresetType
}>()

const emit = defineEmits<{
  /** User clicked Save and the preset persisted. `activated` is true when
   *  the store auto-activated a newly-created preset. */
  (e: 'saved', preset: Preset, activated: boolean): void
  (e: 'cancelled'): void
}>()

const isEdit = computed(() => !!props.preset)
const type = ref<PresetType>(props.initialType ?? 'sampler')
const name = ref('')
// For sampler/openai types we edit the FULL bundle (sampler knobs +
// Prompt Manager + regex + per-provider flags). OpenAIBundleData extends
// SamplerData, so every existing sampler-form binding continues to work
// against bundle.value without per-field renaming.
const bundle = ref<OpenAIBundleData>(defaultBundle())
const instruct = ref<InstructData>(defaultInstruct())
const context = ref<ContextData>(defaultContext())
const sysprompt = ref<SyspromptData>(defaultSysprompt())
const reasoning = ref<ReasoningData>(defaultReasoning())
const stopText = ref('')   // comma-separated textbox for sampler.stop
const saving = ref(false)
const apiError = ref<string | null>(null)

// Active tab. For sampler/openai: sampler | prompts | regex | advanced | raw.
// For other preset types (instruct/context/sysprompt/reasoning): form | raw.
// Flat one-level tab bar — the previous two-level nesting (Form → Sampler)
// hid the Prompt Manager so well that users thought the UI didn't exist.
type TabKey = 'sampler' | 'prompts' | 'regex' | 'advanced' | 'raw' | 'form'
const activeTab = ref<TabKey>('sampler')

// Counts for the tab chips so users see "3 regex scripts" at a glance
// instead of needing to click through.
const promptCount = computed(() => {
  const groups = bundle.value.prompt_order ?? []
  const wildcard = groups.find(g => g.character_id === 100001) ?? groups[0]
  return wildcard?.order?.length ?? 0
})
const regexCount = computed(() => bundle.value.extensions?.regex_scripts?.length ?? 0)

// Raw-JSON mode state. Raw lets the user edit the JSONB payload
// directly — useful for ST fields we don't surface (mirostat_*, tfs,
// xtc_*, dry_*, etc.) which round-trip but are otherwise invisible.
const rawText = ref('')
const rawError = ref<string | null>(null)

// Initial default tab depends on the preset type. For sampler/openai
// we start on the sampler knobs; for template types on the single form.
function defaultTabFor(tp: PresetType): TabKey {
  return (tp === 'sampler' || tp === 'openai') ? 'sampler' : 'form'
}

// Hydrate on mount AND whenever the source preset changes (e.g. parent
// swaps between different rows without unmounting the form).
watch(
  () => [props.preset, props.initialType] as const,
  ([p, initType]) => {
    apiError.value = null
    saving.value = false
    rawError.value = null
    if (p) {
      type.value = p.type
      name.value = p.name
      hydrateFromPreset(p)
    } else {
      type.value = initType ?? 'sampler'
      name.value = ''
      bundle.value = defaultBundle()
      instruct.value = defaultInstruct()
      context.value = defaultContext()
      sysprompt.value = defaultSysprompt()
      reasoning.value = defaultReasoning()
      stopText.value = ''
    }
    activeTab.value = defaultTabFor(type.value)
    rawText.value = JSON.stringify(buildData(), null, 2)
  },
  { immediate: true },
)

// Keep the raw-JSON preview in sync with the form as the user edits —
// so flipping the tab doesn't show stale data.
watch(
  [bundle, instruct, context, sysprompt, reasoning, stopText, type],
  () => {
    if (activeTab.value !== 'raw') {
      rawText.value = JSON.stringify(buildData(), null, 2)
    }
  },
  { deep: true },
)

function hydrateFromPreset(p: Preset) {
  const data = p.data as Record<string, any>
  switch (p.type) {
    case 'sampler':
    case 'openai': {
      // Hydrate FULL bundle — sampler knobs, prompts, prompt_order, regex
      // scripts, per-provider flags all come from the same JSONB blob.
      const b: OpenAIBundleData = { ...defaultBundle(), ...(data as OpenAIBundleData) }
      // ST's openai preset stores max_tokens as `openai_max_tokens`.
      if (b.max_tokens == null && typeof data.openai_max_tokens === 'number') {
        b.max_tokens = data.openai_max_tokens
      }
      bundle.value = b
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
      if (!s.post_history && typeof data.post_history_instructions === 'string') {
        s.post_history = data.post_history_instructions
      }
      sysprompt.value = s
      break
    }
    case 'reasoning':
      reasoning.value = { ...defaultReasoning(), ...(data as ReasoningData) }
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

// defaultBundle extends defaultSampler with empty arrays for the Prompt
// Manager / regex / extensions. All fields beyond the sampler knobs are
// zero-value (no prompts, no order, no regex); users populate them via
// import or the Prompts / Regex tabs.
function defaultBundle(): OpenAIBundleData {
  return {
    ...defaultSampler(),
    prompts: [],
    prompt_order: [],
    extensions: { regex_scripts: [] },
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
    story_string: "{{#if system}}{{system}}\n{{/if}}{{#if description}}{{char}}'s description:\n{{description}}\n{{/if}}{{#if scenario}}Scenario:\n{{scenario}}\n{{/if}}",
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
  PRESET_TYPES.map(tp => ({
    value: tp,
    title: t(`presets.type.${tp}`),
  })),
)

// Slider-safe views for nullable fields (v-slider won't accept null).
const temperatureView = computed(() => bundle.value.temperature ?? 1)
const topPView = computed(() => bundle.value.top_p ?? 1)

// ── Starter templates (P5) ─────────────────────────────────────
//
// One-click fills for someone making their first sampler preset. All
// values are rough community defaults for OpenAI-compat chat
// completion; nothing set that we can't pass to an upstream API.
interface StarterTemplate {
  id: 'creative' | 'balanced' | 'deterministic' | 'reasoning'
  label: string
  data: SamplerData
}
const starterTemplates = computed<StarterTemplate[]>(() => [
  {
    id: 'creative',
    label: t('presets.starter.creative'),
    data: {
      temperature: 1.2, top_p: 0.95,
      frequency_penalty: 0.3, presence_penalty: 0.3,
      repetition_penalty: 1.05, max_tokens: 1024,
      system_prompt: null,
    },
  },
  {
    id: 'balanced',
    label: t('presets.starter.balanced'),
    data: {
      temperature: 0.85, top_p: 0.9,
      frequency_penalty: 0.1, presence_penalty: 0.1,
      repetition_penalty: 1.02, max_tokens: 1024,
      system_prompt: null,
    },
  },
  {
    id: 'deterministic',
    label: t('presets.starter.deterministic'),
    data: {
      temperature: 0.3, top_p: 0.7,
      frequency_penalty: 0, presence_penalty: 0,
      repetition_penalty: 1, max_tokens: 1024,
      system_prompt: null,
    },
  },
  {
    id: 'reasoning',
    label: t('presets.starter.reasoning'),
    data: {
      temperature: 0.5, top_p: 0.8,
      max_tokens: 4096,
      reasoning_enabled: true,
      system_prompt: null,
    },
  },
])

function applyStarter(tpl: StarterTemplate) {
  // Starter template only fills sampler knobs — preserve any prompts /
  // prompt_order / regex the user (or their imported preset) already had.
  bundle.value = { ...bundle.value, ...tpl.data }
  if (!name.value) name.value = tpl.label
}

// ── Raw JSON mode (P4) ────────────────────────────────────────
//
// Switching to raw mode dumps the current typed state as JSON; switching
// back parses the user's edited JSON and pushes field values back into
// the typed refs so the form stays consistent. On parse failure we leave
// the typed state alone and show rawError; Save is disabled until the
// user either fixes the JSON or switches back.
function syncRawToForm(): boolean {
  rawError.value = null
  try {
    const parsed = JSON.parse(rawText.value || '{}') as Record<string, any>
    switch (type.value) {
      case 'sampler':
      case 'openai': {
        // Full bundle resync — pulls any prompts/regex/flags the user
        // edited in raw mode back into the typed refs.
        const b: OpenAIBundleData = { ...defaultBundle(), ...(parsed as OpenAIBundleData) }
        bundle.value = b
        stopText.value = Array.isArray(parsed.stop) ? parsed.stop.join(', ') : ''
        break
      }
      case 'instruct':
        instruct.value = { ...defaultInstruct(), ...(parsed as InstructData) }
        break
      case 'context':
        context.value = { ...defaultContext(), ...(parsed as ContextData) }
        break
      case 'sysprompt':
        sysprompt.value = { ...defaultSysprompt(), ...(parsed as SyspromptData) }
        break
      case 'reasoning':
        reasoning.value = { ...defaultReasoning(), ...(parsed as ReasoningData) }
        break
    }
    return true
  } catch (e) {
    rawError.value = (e as Error).message
    return false
  }
}

function onTabChange(to: TabKey) {
  // Leaving Raw tab — parse JSON back into the typed refs. If the
  // JSON is broken, block the tab switch so the user can't lose
  // edits by navigating away from a malformed raw payload.
  if (activeTab.value === 'raw' && to !== 'raw') {
    if (!syncRawToForm()) return
  }
  // Entering Raw tab — refresh the text from whatever the form now
  // holds so the user sees live state, not stale JSON.
  if (to === 'raw') {
    rawText.value = JSON.stringify(buildData(), null, 2)
    rawError.value = null
  }
  activeTab.value = to
}

// Preserve unknown-to-our-schema fields from the source preset so round-
// trip is lossless even if we only edit one slider.
function buildData(): unknown {
  // When the user is on the raw tab, that's the source of truth.
  if (activeTab.value === 'raw') {
    try { return JSON.parse(rawText.value || '{}') }
    catch { /* fall through to typed build below */ }
  }
  const base = (props.preset?.data ?? {}) as Record<string, unknown>
  switch (type.value) {
    case 'sampler':
    case 'openai': {
      const b: OpenAIBundleData = { ...bundle.value }
      const stops = stopText.value.split(',').map(x => x.trim()).filter(Boolean)
      b.stop = stops.length ? stops : null
      return { ...base, ...b }
    }
    case 'instruct':   return { ...base, ...instruct.value }
    case 'context':    return { ...base, ...context.value }
    case 'sysprompt':  return { ...base, ...sysprompt.value }
    case 'reasoning':  return { ...base, ...reasoning.value }
  }
}

async function save() {
  // Raw mode: parse once more before save so any last edits are picked up.
  if (activeTab.value === 'raw' && !syncRawToForm()) return
  if (!name.value.trim()) {
    apiError.value = t('presets.editor.nameRequired')
    return
  }
  saving.value = true
  apiError.value = null
  try {
    const data = buildData()
    if (props.preset) {
      const saved = await presets.update(props.preset.id, {
        name: name.value.trim(),
        data,
      })
      emit('saved', saved, false)
    } else {
      const { preset, activated } = await presets.create(
        type.value,
        name.value.trim(),
        data,
      )
      emit('saved', preset, activated)
    }
  } catch (e) {
    apiError.value = (e as Error).message
  } finally {
    saving.value = false
  }
}

function cancel() {
  emit('cancelled')
}
</script>

<template>
  <div class="nest-preset-form">
    <!-- Always-visible header: type + name + (starter chips for new sampler).
         Preset identity goes OUTSIDE the tab bar — it's metadata every
         tab needs, and flattening it means users don't lose sight of
         which preset they're editing when they switch tabs. -->
    <div class="nest-form-header">
      <div class="nest-field-row">
        <div class="nest-field nest-field-half">
          <label class="nest-field-label">{{ t('presets.editor.typeLabel') }}</label>
          <v-select
            v-model="type"
            :items="typeOptions"
            :disabled="isEdit"
            item-title="title"
            item-value="value"
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

      <!-- Starter templates (new sampler/openai only). -->
      <div
        v-if="!isEdit && (type === 'sampler' || type === 'openai')"
        class="nest-starter-row"
      >
        <span class="nest-starter-label">
          <v-icon size="14" class="mr-1">mdi-lightbulb-outline</v-icon>
          {{ t('presets.starter.useHint') }}
        </span>
        <v-chip
          v-for="tpl in starterTemplates"
          :key="tpl.id"
          size="small"
          variant="outlined"
          @click="applyStarter(tpl)"
        >
          {{ tpl.label }}
        </v-chip>
      </div>
    </div>

    <!-- Single flat tab bar — no more Form/Raw wrapping an inner
         Sampler/Prompts/… row. For sampler/openai: 5 tabs in one row.
         For template types: just Form + Raw. Counts surface on Prompts
         and Regex so "111 prompts" is visible at first glance. -->
    <v-tabs
      :model-value="activeTab"
      density="compact"
      show-arrows
      class="nest-form-tabs mb-3"
      @update:model-value="v => onTabChange(v as TabKey)"
    >
      <template v-if="type === 'sampler' || type === 'openai'">
        <v-tab value="sampler" class="nest-mono">
          <v-icon size="14" class="mr-1">mdi-tune-variant</v-icon>
          {{ t('presets.subtab.sampler') }}
        </v-tab>
        <v-tab value="prompts" class="nest-mono">
          <v-icon size="14" class="mr-1">mdi-format-list-bulleted-square</v-icon>
          {{ t('presets.subtab.prompts') }}
          <span v-if="promptCount > 0" class="nest-subtab-count">{{ promptCount }}</span>
        </v-tab>
        <v-tab value="regex" class="nest-mono">
          <v-icon size="14" class="mr-1">mdi-regex</v-icon>
          {{ t('presets.subtab.regex') }}
          <span v-if="regexCount > 0" class="nest-subtab-count">{{ regexCount }}</span>
        </v-tab>
        <v-tab value="advanced" class="nest-mono">
          <v-icon size="14" class="mr-1">mdi-cog-outline</v-icon>
          {{ t('presets.subtab.advanced') }}
        </v-tab>
      </template>
      <v-tab v-else value="form" class="nest-mono">
        <v-icon size="14" class="mr-1">mdi-form-select</v-icon>
        {{ t('presets.editor.tabForm') }}
      </v-tab>
      <v-tab value="raw" class="nest-mono">
        <v-icon size="14" class="mr-1">mdi-code-json</v-icon>
        {{ t('presets.editor.tabRaw') }}
      </v-tab>
    </v-tabs>

    <!-- ──────────────────── Tab bodies ──────────────────── -->
    <div class="nest-form-body">
      <!-- Prompt Manager — 111-style prompt list with toggles. -->
      <div v-if="activeTab === 'prompts'">
        <PromptManagerPanel v-model="bundle" />
      </div>

      <!-- Regex scripts editor. -->
      <div v-else-if="activeTab === 'regex'">
        <RegexScriptsPanel v-model="bundle" />
      </div>

      <!-- Advanced bundle flags. -->
      <div v-else-if="activeTab === 'advanced'">
        <AdvancedBundlePanel v-model="bundle" />
      </div>

      <!-- Sampler knobs (for sampler/openai) or typed form (for
           instruct/context/sysprompt/reasoning). Both share this
           branch — only one renders at a time based on `type`. -->
      <template v-else-if="activeTab === 'sampler' || activeTab === 'form'">
        <!-- ── Sampler / OpenAI — knobs only ─────────────────── -->
        <template v-if="type === 'sampler' || type === 'openai'">
        <!-- CORE: always visible. Most-edited fields live here. -->
        <div class="nest-form-section">
          <div class="nest-field-row">
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">
                {{ t('chat.sampler.temperature') }}
                <v-tooltip location="top" :text="t('presets.hint.temperature')">
                  <template #activator="{ props: p }">
                    <v-icon v-bind="p" size="12" class="nest-hint-icon">mdi-information-outline</v-icon>
                  </template>
                </v-tooltip>
                <span class="nest-mono nest-field-value">{{ temperatureView.toFixed(2) }}</span>
              </label>
              <v-slider
                :model-value="temperatureView"
                :min="0" :max="2" :step="0.05"
                hide-details color="primary"
                @update:model-value="v => (bundle.temperature = v)"
              />
            </div>
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">
                {{ t('chat.sampler.topP') }}
                <v-tooltip location="top" :text="t('presets.hint.topP')">
                  <template #activator="{ props: p }">
                    <v-icon v-bind="p" size="12" class="nest-hint-icon">mdi-information-outline</v-icon>
                  </template>
                </v-tooltip>
                <span class="nest-mono nest-field-value">{{ topPView.toFixed(2) }}</span>
              </label>
              <v-slider
                :model-value="topPView"
                :min="0" :max="1" :step="0.05"
                hide-details color="primary"
                @update:model-value="v => (bundle.top_p = v)"
              />
            </div>
          </div>

          <div class="nest-field">
            <label class="nest-field-label">
              {{ t('chat.sampler.maxTokens') }}
              <v-tooltip location="top" :text="t('presets.hint.maxTokens')">
                <template #activator="{ props: p }">
                  <v-icon v-bind="p" size="12" class="nest-hint-icon">mdi-information-outline</v-icon>
                </template>
              </v-tooltip>
            </label>
            <v-text-field
              v-model.number="bundle.max_tokens"
              type="number" :min="0"
              :placeholder="t('chat.sampler.maxTokensPlaceholder')"
              density="compact" hide-details clearable
            />
          </div>

          <div class="nest-field">
            <label class="nest-field-label">
              {{ t('chat.sampler.systemPrompt') }}
              <v-tooltip location="top" :text="t('presets.hint.systemPrompt')">
                <template #activator="{ props: p }">
                  <v-icon v-bind="p" size="12" class="nest-hint-icon">mdi-information-outline</v-icon>
                </template>
              </v-tooltip>
            </label>
            <v-textarea
              v-model="bundle.system_prompt"
              :placeholder="t('chat.sampler.systemPromptPlaceholder')"
              rows="3" auto-grow density="compact" hide-details
            />
          </div>
        </div>

        <!-- Progressive-disclosure: the long tail is collapsed by default.
             Users who just want temp+top_p+max_tokens never see this. -->
        <v-expansion-panels variant="accordion" class="nest-adv-panels">
          <v-expansion-panel>
            <v-expansion-panel-title>
              <v-icon size="16" class="mr-2">mdi-tune</v-icon>
              {{ t('presets.section.samplingAdv') }}
            </v-expansion-panel-title>
            <v-expansion-panel-text>
              <div class="nest-field-row">
                <div class="nest-field nest-field-half">
                  <label class="nest-field-label">
                    {{ t('chat.sampler.topK') }}
                    <v-tooltip location="top" :text="t('presets.hint.topK')">
                      <template #activator="{ props: p }">
                        <v-icon v-bind="p" size="12" class="nest-hint-icon">mdi-information-outline</v-icon>
                      </template>
                    </v-tooltip>
                  </label>
                  <v-text-field
                    v-model.number="bundle.top_k" type="number" :min="0"
                    :placeholder="t('chat.sampler.unset')"
                    density="compact" hide-details clearable
                  />
                </div>
                <div class="nest-field nest-field-half">
                  <label class="nest-field-label">
                    {{ t('chat.sampler.minP') }}
                    <v-tooltip location="top" :text="t('presets.hint.minP')">
                      <template #activator="{ props: p }">
                        <v-icon v-bind="p" size="12" class="nest-hint-icon">mdi-information-outline</v-icon>
                      </template>
                    </v-tooltip>
                  </label>
                  <v-text-field
                    v-model.number="bundle.min_p" type="number"
                    :min="0" :max="1" :step="0.01"
                    :placeholder="t('chat.sampler.unset')"
                    density="compact" hide-details clearable
                  />
                </div>
              </div>
              <div class="nest-field-row">
                <div class="nest-field nest-field-half">
                  <label class="nest-field-label">
                    {{ t('chat.sampler.seed') }}
                    <v-tooltip location="top" :text="t('presets.hint.seed')">
                      <template #activator="{ props: p }">
                        <v-icon v-bind="p" size="12" class="nest-hint-icon">mdi-information-outline</v-icon>
                      </template>
                    </v-tooltip>
                  </label>
                  <v-text-field
                    v-model.number="bundle.seed" type="number"
                    :placeholder="t('chat.sampler.seedPlaceholder')"
                    density="compact" hide-details clearable
                  />
                </div>
                <div class="nest-field nest-field-half">
                  <label class="nest-field-label">
                    {{ t('chat.sampler.stop') }}
                    <v-tooltip location="top" :text="t('presets.hint.stop')">
                      <template #activator="{ props: p }">
                        <v-icon v-bind="p" size="12" class="nest-hint-icon">mdi-information-outline</v-icon>
                      </template>
                    </v-tooltip>
                  </label>
                  <v-text-field
                    v-model="stopText"
                    :placeholder="t('chat.sampler.stopPlaceholder')"
                    density="compact" hide-details
                  />
                </div>
              </div>
            </v-expansion-panel-text>
          </v-expansion-panel>

          <v-expansion-panel>
            <v-expansion-panel-title>
              <v-icon size="16" class="mr-2">mdi-flash-outline</v-icon>
              {{ t('presets.section.penalties') }}
            </v-expansion-panel-title>
            <v-expansion-panel-text>
              <div class="nest-field-row">
                <div class="nest-field nest-field-half">
                  <label class="nest-field-label">
                    {{ t('chat.sampler.freqPenalty') }}
                    <v-tooltip location="top" :text="t('presets.hint.freqPenalty')">
                      <template #activator="{ props: p }">
                        <v-icon v-bind="p" size="12" class="nest-hint-icon">mdi-information-outline</v-icon>
                      </template>
                    </v-tooltip>
                  </label>
                  <v-text-field
                    v-model.number="bundle.frequency_penalty" type="number"
                    :min="-2" :max="2" :step="0.1"
                    density="compact" hide-details clearable
                  />
                </div>
                <div class="nest-field nest-field-half">
                  <label class="nest-field-label">
                    {{ t('chat.sampler.presPenalty') }}
                    <v-tooltip location="top" :text="t('presets.hint.presPenalty')">
                      <template #activator="{ props: p }">
                        <v-icon v-bind="p" size="12" class="nest-hint-icon">mdi-information-outline</v-icon>
                      </template>
                    </v-tooltip>
                  </label>
                  <v-text-field
                    v-model.number="bundle.presence_penalty" type="number"
                    :min="-2" :max="2" :step="0.1"
                    density="compact" hide-details clearable
                  />
                </div>
              </div>
              <div class="nest-field">
                <label class="nest-field-label">
                  {{ t('chat.sampler.repPenalty') }}
                  <v-tooltip location="top" :text="t('presets.hint.repPenalty')">
                    <template #activator="{ props: p }">
                      <v-icon v-bind="p" size="12" class="nest-hint-icon">mdi-information-outline</v-icon>
                    </template>
                  </v-tooltip>
                </label>
                <v-text-field
                  v-model.number="bundle.repetition_penalty" type="number"
                  :min="0.5" :max="2" :step="0.05"
                  :placeholder="t('chat.sampler.unset')"
                  density="compact" hide-details clearable
                />
              </div>
            </v-expansion-panel-text>
          </v-expansion-panel>

          <v-expansion-panel>
            <v-expansion-panel-title>
              <v-icon size="16" class="mr-2">mdi-brain</v-icon>
              {{ t('presets.section.reasoning') }}
            </v-expansion-panel-title>
            <v-expansion-panel-text>
              <div class="nest-field">
                <label class="nest-field-label">
                  {{ t('chat.sampler.reasoning.label') }}
                  <v-tooltip location="top" :text="t('presets.hint.reasoning')">
                    <template #activator="{ props: p }">
                      <v-icon v-bind="p" size="12" class="nest-hint-icon">mdi-information-outline</v-icon>
                    </template>
                  </v-tooltip>
                </label>
                <v-btn-toggle
                  v-model="bundle.reasoning_enabled"
                  color="primary"
                  density="compact"
                  variant="outlined"
                  mandatory="force"
                >
                  <v-btn :value="null">{{ t('chat.sampler.reasoning.default') }}</v-btn>
                  <v-btn :value="true">{{ t('chat.sampler.reasoning.on') }}</v-btn>
                  <v-btn :value="false">{{ t('chat.sampler.reasoning.off') }}</v-btn>
                </v-btn-toggle>
              </div>
            </v-expansion-panel-text>
          </v-expansion-panel>
        </v-expansion-panels>
        </template>
        <!-- ── End sampler/openai knobs ──────────────────── -->

      <!-- ── Instruct ─────────────────────────────────────── -->
      <template v-else-if="type === 'instruct'">
        <div class="nest-form-section">
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.editor.instruct.inputSeq') }}</label>
            <v-text-field v-model="instruct.input_sequence" density="compact" hide-details />
          </div>
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.editor.instruct.outputSeq') }}</label>
            <v-text-field v-model="instruct.output_sequence" density="compact" hide-details />
          </div>
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.editor.instruct.systemSeq') }}</label>
            <v-text-field v-model="instruct.system_sequence" density="compact" hide-details />
          </div>
        </div>
        <v-expansion-panels variant="accordion" class="nest-adv-panels">
          <v-expansion-panel>
            <v-expansion-panel-title>
              <v-icon size="16" class="mr-2">mdi-tune</v-icon>
              {{ t('presets.section.advanced') }}
            </v-expansion-panel-title>
            <v-expansion-panel-text>
              <div class="nest-field">
                <label class="nest-field-label">{{ t('presets.editor.instruct.stopSeq') }}</label>
                <v-text-field v-model="instruct.stop_sequence" density="compact" hide-details />
              </div>
              <div class="nest-field">
                <label class="nest-field-label">{{ t('presets.editor.instruct.activationRegex') }}</label>
                <v-text-field v-model="instruct.activation_regex" density="compact" hide-details placeholder="/gpt-/i" />
              </div>
              <div class="nest-field">
                <v-switch
                  v-model="instruct.wrap"
                  :label="t('presets.editor.instruct.wrap')"
                  color="primary" hide-details density="compact"
                />
              </div>
            </v-expansion-panel-text>
          </v-expansion-panel>
        </v-expansion-panels>
      </template>

      <!-- ── Context ──────────────────────────────────────── -->
      <template v-else-if="type === 'context'">
        <div class="nest-form-section">
          <div class="nest-field">
            <label class="nest-field-label">
              {{ t('presets.editor.context.storyString') }}
            </label>
            <v-textarea
              v-model="context.story_string"
              :placeholder="t('presets.editor.context.storyHint')"
              rows="5" auto-grow density="compact" hide-details
            />
          </div>
        </div>
        <v-expansion-panels variant="accordion" class="nest-adv-panels">
          <v-expansion-panel>
            <v-expansion-panel-title>
              <v-icon size="16" class="mr-2">mdi-tune</v-icon>
              {{ t('presets.section.advanced') }}
            </v-expansion-panel-title>
            <v-expansion-panel-text>
              <div class="nest-field-row">
                <div class="nest-field nest-field-half">
                  <label class="nest-field-label">{{ t('presets.editor.context.chatStart') }}</label>
                  <v-text-field v-model="context.chat_start" density="compact" hide-details />
                </div>
                <div class="nest-field nest-field-half">
                  <label class="nest-field-label">{{ t('presets.editor.context.exampleSep') }}</label>
                  <v-text-field v-model="context.example_separator" density="compact" hide-details />
                </div>
              </div>
              <div class="nest-field">
                <v-switch
                  v-model="context.trim_sentences"
                  :label="t('presets.editor.context.trimSentences')"
                  color="primary" hide-details density="compact"
                />
              </div>
              <div class="nest-field">
                <v-switch
                  v-model="context.single_line"
                  :label="t('presets.editor.context.singleLine')"
                  color="primary" hide-details density="compact"
                />
              </div>
            </v-expansion-panel-text>
          </v-expansion-panel>
        </v-expansion-panels>
      </template>

      <!-- ── Sysprompt ────────────────────────────────────── -->
      <template v-else-if="type === 'sysprompt'">
        <div class="nest-form-section">
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.editor.sysprompt.content') }}</label>
            <v-textarea
              v-model="sysprompt.content"
              :placeholder="t('presets.editor.sysprompt.contentPlaceholder')"
              rows="6" auto-grow density="compact" hide-details
            />
          </div>
        </div>
        <v-expansion-panels variant="accordion" class="nest-adv-panels">
          <v-expansion-panel>
            <v-expansion-panel-title>
              <v-icon size="16" class="mr-2">mdi-format-paragraph</v-icon>
              {{ t('presets.editor.sysprompt.postHistory') }}
            </v-expansion-panel-title>
            <v-expansion-panel-text>
              <div class="nest-field">
                <v-textarea
                  v-model="sysprompt.post_history"
                  :placeholder="t('presets.editor.sysprompt.postHistoryPlaceholder')"
                  :hint="t('presets.editor.sysprompt.postHistoryHint')"
                  rows="4" auto-grow density="compact"
                  persistent-hint
                />
              </div>
            </v-expansion-panel-text>
          </v-expansion-panel>
        </v-expansion-panels>
      </template>

      <!-- ── Reasoning ────────────────────────────────────── -->
      <template v-else-if="type === 'reasoning'">
        <div class="nest-form-section">
          <div class="nest-field-row">
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">{{ t('presets.editor.reasoning.prefix') }}</label>
              <v-text-field v-model="reasoning.prefix" density="compact" hide-details />
            </div>
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">{{ t('presets.editor.reasoning.suffix') }}</label>
              <v-text-field v-model="reasoning.suffix" density="compact" hide-details />
            </div>
          </div>
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.editor.reasoning.separator') }}</label>
            <v-text-field v-model="reasoning.separator" density="compact" hide-details />
          </div>
        </div>
      </template>
      </template>
      <!-- End sampler/form branch -->

      <!-- ── Raw JSON tab ──────────────────────────────────── -->
      <div v-else-if="activeTab === 'raw'" class="nest-raw-mode">
        <div class="nest-raw-hint">
          <v-icon size="14" class="mr-1">mdi-information-outline</v-icon>
          {{ t('presets.editor.rawHint') }}
        </div>
        <v-textarea
          v-model="rawText"
          rows="20"
          density="compact"
          hide-details
          variant="outlined"
          class="nest-raw-textarea"
          :placeholder="t('presets.editor.rawPlaceholder')"
        />
        <v-alert
          v-if="rawError"
          type="error"
          variant="tonal"
          density="compact"
          class="mt-2"
        >
          {{ rawError }}
        </v-alert>
      </div>
    </div>
    <!-- End nest-form-body -->

    <!-- API error banner (for Save failures). -->
    <v-alert
      v-if="apiError"
      type="error"
      variant="tonal"
      density="compact"
      class="mt-2"
    >
      {{ apiError }}
    </v-alert>

    <!-- Actions row. -->
    <div class="nest-form-actions">
      <v-btn variant="text" :disabled="saving" @click="cancel">
        {{ t('common.cancel') }}
      </v-btn>
      <v-btn
        color="primary"
        variant="flat"
        :loading="saving"
        @click="save"
      >
        {{ isEdit ? t('common.save') : t('presets.actions.create') }}
      </v-btn>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.nest-preset-form {
  padding: 8px 0 4px;
}

.nest-form-tabs {
  border-bottom: 1px solid var(--nest-border-subtle);
}

// Sampler sub-tabs — one level deeper than the main Form/Raw tabs.
// Slightly smaller, no underline, so they don't compete visually with
// the outer tab bar.
.nest-subtabs {
  :deep(.v-tab) {
    min-height: 36px !important;
    font-size: 11.5px;
    letter-spacing: 0.02em;
  }
}
.nest-subtab-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  margin-left: 6px;
  padding: 0 6px;
  min-width: 18px;
  height: 16px;
  font-size: 10px;
  font-family: var(--nest-font-mono);
  background: var(--nest-accent);
  color: #fff;
  border-radius: 9px;
}

.nest-form-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.nest-form-section {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 4px 2px;
}

.nest-field-row {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}
.nest-field { min-width: 0; }
.nest-field-half {
  flex: 1 1 220px;
}

.nest-field-label {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--nest-text-secondary);
  margin-bottom: 4px;
}
.nest-field-value {
  margin-left: auto;
  font-size: 11.5px;
  color: var(--nest-text-muted);
}

.nest-hint-icon {
  color: var(--nest-text-muted);
  cursor: help;
  opacity: 0.7;

  &:hover { opacity: 1; color: var(--nest-accent); }
}

.nest-starter-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding: 10px 12px;
  background: var(--nest-bg-elevated);
  border-radius: var(--nest-radius-sm);
  border: 1px dashed var(--nest-border-subtle);
}
.nest-starter-label {
  font-size: 12px;
  color: var(--nest-text-muted);
  display: inline-flex;
  align-items: center;
}

.nest-adv-panels {
  // Tighter visual weight than Vuetify's default — these are secondary.
  :deep(.v-expansion-panel-title) {
    min-height: 40px !important;
    padding: 8px 14px !important;
    font-size: 13px;
    font-weight: 500;
  }
  :deep(.v-expansion-panel-text__wrapper) {
    padding: 10px 14px 14px !important;
  }
  :deep(.v-expansion-panel) {
    background: var(--nest-surface) !important;
    border: 1px solid var(--nest-border-subtle) !important;
    border-radius: var(--nest-radius-sm) !important;
    margin-top: 6px;
  }
}

.nest-raw-mode {
  padding-top: 4px;
}
.nest-raw-hint {
  display: flex;
  align-items: center;
  color: var(--nest-text-muted);
  font-size: 12px;
  margin-bottom: 8px;
}
.nest-raw-textarea {
  :deep(textarea) {
    font-family: var(--nest-font-mono) !important;
    font-size: 12px !important;
    line-height: 1.5 !important;
  }
}

.nest-form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 14px;
  padding-top: 12px;
  border-top: 1px solid var(--nest-border-subtle);
}
</style>
