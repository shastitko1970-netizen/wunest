<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useDisplay } from 'vuetify'
import { useChatsStore } from '@/stores/chats'
import { usePresetsStore } from '@/stores/presets'
import { usePreferencesStore } from '@/stores/preferences'
import type { AuthorsNote } from '@/api/chats'
import type { Preset, SamplerData } from '@/api/presets'

// Drawer that edits the user's ACTIVE sampler preset (M30 variant-1: one
// active per type, applies globally). Non-modal so users can see the chat
// while tweaking.
//
// What changed vs. the pre-M30 drawer:
//   • Hydration reads presets.activePreset('sampler'), NOT chat_metadata.
//   • Save writes back to the active preset, so edits propagate to every
//     chat immediately (no per-chat sampler copies anymore).
//   • The preset dropdown switches which preset is active (exclusive —
//     flipping one on clears the previous); it no longer "applies to this
//     chat only".
//   • Save As creates a new preset and makes it active.
//
// Layout: core generation knobs stay visible; the long tail of power-user
// fields (top_k, min_p, rep penalty, seed, stop strings, reasoning toggle)
// is collapsed into an "Advanced" section — matches SillyTavern's
// progressive disclosure and keeps casual users from drowning.

const { t } = useI18n()
const { smAndDown } = useDisplay()

const props = defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: boolean): void }>()

const chats = useChatsStore()
const presets = usePresetsStore()
// Global streaming preference (persists to localStorage, syncs across
// tabs). Exposing it here next to the sampler knobs because that's the
// "how my reply comes out" drawer — and users asked to have the toggle
// somewhere they actually see while chatting, not buried in Settings.
const prefs = usePreferencesStore()
const { disableStreaming } = storeToRefs(prefs)

// Drawer width adapts to viewport. On mobile we go full-width so the
// two-column field rows have room to breathe; on desktop 420px stays
// out of the way of the chat.
const drawerWidth = computed(() => (smAndDown.value ? undefined : 420))

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

// Author's Note is sibling to the sampler — same drawer because users
// tweak both at the same moment. State is its own since it doesn't
// round-trip through a preset.
const note = ref<AuthorsNote>({ content: '', depth: 4, role: 'system' })
const noteSaving = ref(false)
const noteSavedHint = ref(false)

// Save-as dialog state
const saveAsOpen = ref(false)
const saveAsName = ref('')
const saveAsBusy = ref(false)
const saveAsError = ref<string | null>(null)

watch(
  () => [props.modelValue, chats.currentChat?.id] as const,
  async ([open]) => {
    if (!open) return
    await presets.fetchAll()
    hydrateFromActive()
  },
  { immediate: true },
)

// Re-hydrate the form whenever the active sampler preset changes under us
// (e.g. the user switched active via the PresetsPanel toggle while the
// drawer was already open).
watch(
  () => presets.activeID('sampler'),
  () => { if (props.modelValue) hydrateFromActive() },
)

function hydrateFromActive() {
  const active = presets.activePreset('sampler')
  if (!active) {
    form.value = defaultForm()
    selectedPresetId.value = null
  } else {
    const d = active.data as SamplerData
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
    selectedPresetId.value = active.id
  }
  // Author's Note is still per-chat (not part of M30 rework).
  const meta = chats.currentChat?.chat_metadata
  const an = meta?.authors_note
  note.value = an
    ? { content: an.content, depth: an.depth ?? 4, role: an.role ?? 'system' }
    : { content: '', depth: 4, role: 'system' }
}

/**
 * Switch active sampler preset. Writes through to the server so every
 * chat picks up the new active one on the next message. Then reloads the
 * form so the drawer shows the new values.
 */
async function applyPreset(id: string | null) {
  selectedPresetId.value = id
  await presets.setActive('sampler', id)
  hydrateFromActive()
}

function toSamplerData(): SamplerData {
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
  }
}

/**
 * Save = write form changes back to the active preset. If there is no
 * active preset yet, fall through to Save-As so the user can name it
 * first (otherwise we'd silently create an "Untitled" preset).
 */
async function save() {
  const activeId = presets.activeID('sampler')
  if (!activeId) {
    openSaveAs()
    return
  }
  saving.value = true
  savedHint.value = false
  try {
    await presets.update(activeId, { data: toSamplerData() })
    savedHint.value = true
    setTimeout(() => (savedHint.value = false), 1500)
  } finally {
    saving.value = false
  }
}

async function saveNote() {
  if (!chats.currentChat) return
  noteSaving.value = true
  noteSavedHint.value = false
  try {
    const content = note.value.content.trim()
    await chats.setAuthorsNote(content ? { ...note.value, content } : null)
    noteSavedHint.value = true
    setTimeout(() => (noteSavedHint.value = false), 1500)
  } finally {
    noteSaving.value = false
  }
}

async function clearNote() {
  if (!chats.currentChat) return
  note.value = { content: '', depth: 4, role: 'system' }
  await chats.setAuthorsNote(null)
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
    const created = await presets.createSampler(name, toSamplerData())
    // New preset auto-becomes active — matches user expectation of
    // "saved + now using it".
    await presets.setActive('sampler', created.id)
    selectedPresetId.value = created.id
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
  if (selectedPresetId.value === id) {
    selectedPresetId.value = null
    // presets.remove also clears active; reload the form to reflect that.
    hydrateFromActive()
  }
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

// Slider-friendly proxies for `number | null` fields. The slider always
// needs a concrete number, but the field's "unset" state (placeholder
// text) must round-trip null. Strategy:
//   - getter returns 0 when null so the slider shows the "neutral" pos
//   - setter clears to null when value === 0 (only for max_tokens — for
//     penalties 0 is a valid explicit value, so the slider always writes
//     a concrete number; clearing to null still works via the input's
//     clearable button)
const maxTokensSlider = computed<number>({
  get: () => form.value.max_tokens ?? 0,
  set: v => { form.value.max_tokens = v > 0 ? v : null },
})
const freqPenaltySlider = computed<number>({
  get: () => form.value.frequency_penalty ?? 0,
  set: v => { form.value.frequency_penalty = v },
})
const presPenaltySlider = computed<number>({
  get: () => form.value.presence_penalty ?? 0,
  set: v => { form.value.presence_penalty = v },
})

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
    :width="drawerWidth"
    :class="['nest-sampler-drawer', { 'is-mobile': smAndDown }]"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div class="nest-sampler-head">
      <div class="nest-eyebrow">{{ t('chat.sampler.title') }}</div>
      <v-btn icon="mdi-close" variant="text" size="small" @click="close" />
    </div>

    <div class="nest-sampler-body">
      <!-- Streaming toggle — lives here so it's one tap from the chat.
           Flipping it takes effect on the next turn. Preference is global
           and persists; no per-chat override today. -->
      <div class="nest-field nest-stream-toggle">
        <v-switch
          v-model="disableStreaming"
          :label="t('settings.streaming.disableLabel')"
          color="primary"
          inset
          hide-details
          density="compact"
        />
        <div class="nest-field-hint">{{ t('settings.streaming.disableHintShort') }}</div>
      </div>

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

      <!-- Core knobs.
           Each numeric field exposes BOTH a slider (intuition for the
           scale) AND a numeric input (precision when the user knows the
           value). Both bind to the same source — drag the slider, type
           in the box, edits round-trip in either direction. -->
      <div class="nest-field">
        <label class="nest-field-label">{{ t('chat.sampler.temperature') }}</label>
        <div class="nest-slider-row">
          <v-slider
            v-model="form.temperature"
            :min="0" :max="2" :step="0.05"
            hide-details density="compact" color="primary"
          />
          <v-text-field
            v-model.number="form.temperature"
            type="number" :min="0" :max="2" :step="0.05"
            hide-details density="compact"
            class="nest-slider-num"
          />
        </div>
      </div>

      <div class="nest-field">
        <label class="nest-field-label">{{ t('chat.sampler.topP') }}</label>
        <div class="nest-slider-row">
          <v-slider
            v-model="form.top_p"
            :min="0" :max="1" :step="0.05"
            hide-details density="compact" color="primary"
          />
          <v-text-field
            v-model.number="form.top_p"
            type="number" :min="0" :max="1" :step="0.05"
            hide-details density="compact"
            class="nest-slider-num"
          />
        </div>
      </div>

      <div class="nest-field">
        <label class="nest-field-label">{{ t('chat.sampler.maxTokens') }}</label>
        <div class="nest-slider-row">
          <!-- Slider 0..8000. 0 = "не ограничивать" (round-trips null). -->
          <v-slider
            v-model="maxTokensSlider"
            :min="0" :max="8000" :step="100"
            hide-details density="compact" color="primary"
          />
          <v-text-field
            v-model.number="form.max_tokens"
            type="number" :min="0"
            :placeholder="t('chat.sampler.maxTokensPlaceholder')"
            hide-details density="compact" clearable
            class="nest-slider-num nest-slider-num--wide"
          />
        </div>
      </div>

      <div class="nest-field-row">
        <div class="nest-field nest-field-half">
          <label class="nest-field-label">{{ t('chat.sampler.freqPenalty') }}</label>
          <div class="nest-slider-row">
            <v-slider
              v-model="freqPenaltySlider"
              :min="-2" :max="2" :step="0.1"
              hide-details density="compact" color="primary"
            />
            <v-text-field
              v-model.number="form.frequency_penalty"
              type="number" :min="-2" :max="2" :step="0.1"
              hide-details density="compact" clearable
              class="nest-slider-num"
            />
          </div>
        </div>
        <div class="nest-field nest-field-half">
          <label class="nest-field-label">{{ t('chat.sampler.presPenalty') }}</label>
          <div class="nest-slider-row">
            <v-slider
              v-model="presPenaltySlider"
              :min="-2" :max="2" :step="0.1"
              hide-details density="compact" color="primary"
            />
            <v-text-field
              v-model.number="form.presence_penalty"
              type="number" :min="-2" :max="2" :step="0.1"
              hide-details density="compact" clearable
              class="nest-slider-num"
            />
          </div>
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

      <!-- Author's Note — mid-history injection -->
      <div class="nest-authors-note">
        <div class="nest-field">
          <label class="nest-field-label">
            {{ t('chat.authorsNote.label') }}
            <span v-if="noteSavedHint" class="nest-mono nest-saving-hint">
              {{ t('chat.authorsNote.saved') }}
            </span>
          </label>
          <v-textarea
            v-model="note.content"
            :placeholder="t('chat.authorsNote.placeholder')"
            rows="3" auto-grow hide-details density="compact"
          />
          <div class="nest-field-hint">{{ t('chat.authorsNote.hint') }}</div>
        </div>
        <div class="nest-field-row">
          <div class="nest-field nest-field-half">
            <label class="nest-field-label">
              {{ t('chat.authorsNote.depth') }}
              <span class="nest-mono nest-field-hint-inline">
                {{ t('chat.authorsNote.depthHint') }}
              </span>
            </label>
            <v-text-field
              v-model.number="note.depth"
              type="number" :min="0" :max="20"
              hide-details density="compact"
            />
          </div>
          <div class="nest-field nest-field-half">
            <label class="nest-field-label">{{ t('chat.authorsNote.role') }}</label>
            <v-select
              v-model="note.role"
              :items="[
                { value: 'system', title: t('chat.authorsNote.roleSystem') },
                { value: 'user', title: t('chat.authorsNote.roleUser') },
                { value: 'assistant', title: t('chat.authorsNote.roleAssistant') },
              ]"
              density="compact" hide-details
            />
          </div>
        </div>
        <div class="d-flex ga-2 justify-end">
          <v-btn size="small" variant="text" @click="clearNote">
            {{ t('chat.authorsNote.clear') }}
          </v-btn>
          <v-btn
            size="small" color="primary" variant="flat"
            :loading="noteSaving"
            prepend-icon="mdi-content-save"
            @click="saveNote"
          >
            {{ t('common.save') }}
          </v-btn>
        </div>
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

// Slider + numeric input on one line.
// Slider takes the remaining width, input is fixed-width on the right.
// The user can drag for intuition, type for precision — both bind to
// the same source so edits round-trip in either direction.
.nest-slider-row {
  display: flex;
  align-items: center;
  gap: 10px;
}
.nest-slider-row :deep(.v-slider) {
  flex: 1 1 auto;
  min-width: 0;
}
.nest-slider-num {
  flex: 0 0 76px;
  max-width: 76px;
}
.nest-slider-num--wide {
  flex: 0 0 96px;
  max-width: 96px;
}
// In a half-row (penalty pair) the input must shrink so the slider
// still has room. 60px fits "-2.0" / "1.20" comfortably.
.nest-field-half .nest-slider-num {
  flex: 0 0 60px;
  max-width: 60px;
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

.nest-authors-note {
  padding: 12px;
  border: 1px dashed var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
  background: var(--nest-bg);
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.nest-saving-hint {
  font-size: 10.5px;
  color: var(--nest-green);
  letter-spacing: 0.05em;
}

.nest-stream-toggle {
  padding: 8px 10px;
  border: 1px dashed var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
  background: var(--nest-bg);
}

// Phones. Drawer goes full-width, and the half-half field rows stack
// into one column since at ~375px each half would be 150px — too
// cramped for any of the penalty / seed / stop / depth / role fields.
// DS-canonical 640px breakpoint; was 600.
@media (max-width: 640px) {
  .nest-sampler-body {
    padding: 12px;
    gap: 14px;
  }
  .nest-field-row { flex-direction: column; gap: 14px; }
  .nest-field-half { flex: 1 1 100%; }
  .nest-sampler-head { padding: 12px 14px; }
  .nest-sampler-foot { padding: 8px 10px; flex-wrap: wrap; gap: 4px; }
  .nest-authors-note { padding: 10px; gap: 10px; }

  // Slider + input stack vertically on phones — both take full width so
  // the slider has room to drag accurately and the input shows numbers
  // without truncation. On desktop they stay side-by-side.
  .nest-slider-row {
    flex-direction: column;
    align-items: stretch;
    gap: 6px;
  }
  .nest-slider-num,
  .nest-slider-num--wide,
  .nest-field-half .nest-slider-num {
    flex: 1 1 auto;
    max-width: none;
  }
}
</style>
