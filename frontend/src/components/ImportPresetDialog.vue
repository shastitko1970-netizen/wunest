<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useDisplay } from 'vuetify'
import { usePresetsStore } from '@/stores/presets'
import { PRESET_TYPES, type PresetType } from '@/api/presets'
import { detectPresetType } from '@/lib/presetDetect'

// Import SillyTavern-style preset JSON. We auto-detect the preset type
// from the JSON shape (see lib/presetDetect.ts) so the user doesn't have
// to know whether their file is a sampler / instruct / context / sysprompt
// / reasoning preset — it "just imports". The type picker is still there
// as a fallback for files whose shape we can't classify, or for the
// occasional user who explicitly wants to misfile one.
const { t } = useI18n()
const { smAndDown } = useDisplay()

const props = defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  /** `activated` is true when auto-activation kicked in (no prior active preset of this type). */
  (e: 'imported', id: string, activated: boolean): void
}>()

const presets = usePresetsStore()

const file = ref<File | null>(null)
const rawJson = ref<string>('')
const parsed = ref<unknown>(null)
const detectedType = ref<PresetType | null>(null)
const overrideType = ref<PresetType | null>(null)
const name = ref('')
const parseError = ref<string | null>(null)
const busy = ref(false)
const apiError = ref<string | null>(null)
const isDragging = ref(false)
const inputEl = ref<HTMLInputElement | null>(null)

// The user's final pick — override if they picked one, otherwise the
// detected type. Button stays disabled while this is null.
const effectiveType = computed<PresetType | null>(() =>
  overrideType.value ?? detectedType.value,
)

const typeOptions = computed(() =>
  PRESET_TYPES.map(type => ({
    value: type,
    label: t(`presets.type.${type}`),
  })),
)

watch(() => props.modelValue, (open) => {
  if (open) {
    file.value = null
    rawJson.value = ''
    parsed.value = null
    detectedType.value = null
    overrideType.value = null
    name.value = ''
    parseError.value = null
    apiError.value = null
    busy.value = false
    isDragging.value = false
  }
})

function close() {
  emit('update:modelValue', false)
}

function pickFile() {
  inputEl.value?.click()
}

async function onFileInput(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0] ?? null
  await ingestFile(f)
}

async function onDrop(e: DragEvent) {
  e.preventDefault()
  isDragging.value = false
  const f = e.dataTransfer?.files?.[0] ?? null
  await ingestFile(f)
}

async function ingestFile(f: File | null) {
  file.value = f
  parseError.value = null
  apiError.value = null
  parsed.value = null
  detectedType.value = null
  overrideType.value = null
  if (!f) {
    rawJson.value = ''
    return
  }
  try {
    rawJson.value = await f.text()
    const obj = JSON.parse(rawJson.value)
    parsed.value = obj
    detectedType.value = detectPresetType(obj)
    // Derive a default name from the file name if the user hasn't typed one.
    if (!name.value) {
      name.value = f.name.replace(/\.[^.]+$/, '').slice(0, 60)
    }
  } catch (err) {
    parseError.value = t('presets.import.invalidJson')
  }
}

async function doImport() {
  if (!effectiveType.value || parsed.value === null) {
    parseError.value = t('presets.import.pickFile')
    return
  }
  if (!name.value.trim()) {
    apiError.value = t('presets.import.nameRequired')
    return
  }
  busy.value = true
  apiError.value = null
  try {
    // Auto-activate if no preset of this type is currently active. Most
    // users imported because they want to USE this preset — don't make
    // them hunt for an Apply toggle afterwards.
    const { preset, activated } = await presets.create(
      effectiveType.value,
      name.value.trim(),
      parsed.value,
    )
    emit('imported', preset.id, activated)
    close()
  } catch (e) {
    apiError.value = (e as Error).message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <v-dialog
    :model-value="modelValue"
    :max-width="smAndDown ? undefined : 560"
    :fullscreen="smAndDown"
    scrollable
    @update:model-value="emit('update:modelValue', $event)"
  >
    <v-card class="nest-import-preset">
      <v-card-title class="nest-import-title">
        <span>{{ t('presets.import.title') }}</span>
        <v-btn icon="mdi-close" variant="text" size="small" @click="close" />
      </v-card-title>

      <v-card-text>
        <!-- Drop zone: the whole thing is clickable; also handles drag/drop -->
        <div
          class="nest-preset-dz"
          :class="{ dragging: isDragging, hasfile: !!file }"
          @click="pickFile"
          @dragover.prevent="isDragging = true"
          @dragleave.prevent="isDragging = false"
          @drop="onDrop"
        >
          <input
            ref="inputEl"
            type="file"
            accept="application/json,.json"
            hidden
            @change="onFileInput"
          />
          <template v-if="!file">
            <v-icon size="28" color="primary">mdi-code-json</v-icon>
            <div class="nest-dz-title mt-2">{{ t('presets.import.pickFile') }}</div>
            <div class="nest-dz-sub">{{ t('presets.import.dropHint') }}</div>
            <v-btn class="mt-3" size="small" variant="outlined" @click.stop="pickFile">
              {{ t('presets.import.choose') }}
            </v-btn>
          </template>
          <template v-else>
            <v-icon size="24" :color="detectedType ? 'success' : 'warning'">
              {{ detectedType ? 'mdi-check-circle' : 'mdi-help-circle-outline' }}
            </v-icon>
            <div class="nest-dz-title mt-2">{{ file.name }}</div>
            <div class="nest-dz-sub nest-mono">
              {{ (file.size / 1024).toFixed(1) }} KB ·
              <span v-if="detectedType">
                {{ t('presets.import.detected', { type: t(`presets.type.${detectedType}`) }) }}
              </span>
              <span v-else class="text-warning">
                {{ t('presets.import.cannotDetect') }}
              </span>
            </div>
            <v-btn class="mt-3" size="small" variant="text" @click.stop="ingestFile(null)">
              {{ t('library.import.chooseAnother') }}
            </v-btn>
          </template>
        </div>

        <!-- Manual override — show once a file is picked, collapsed under
             a disclosure so the happy path is one click. -->
        <v-expansion-panels v-if="file" variant="accordion" class="mt-3">
          <v-expansion-panel>
            <v-expansion-panel-title>
              <span class="nest-mono" style="font-size: 11px; letter-spacing: 0.08em; text-transform: uppercase">
                {{ t('presets.import.overrideType') }}
              </span>
            </v-expansion-panel-title>
            <v-expansion-panel-text>
              <v-select
                v-model="overrideType"
                :items="typeOptions"
                item-title="label"
                item-value="value"
                :label="t('presets.import.typeLabel')"
                :placeholder="detectedType ? t(`presets.type.${detectedType}`) : t('presets.import.pickType')"
                density="compact"
                hide-details
                clearable
              />
            </v-expansion-panel-text>
          </v-expansion-panel>
        </v-expansion-panels>

        <v-text-field
          v-model="name"
          :label="t('presets.import.nameLabel')"
          :placeholder="t('presets.import.namePlaceholder')"
          density="compact"
          class="mt-3"
        />

        <v-alert
          v-if="parseError"
          type="error"
          variant="tonal"
          density="compact"
          class="mt-3"
        >
          {{ parseError }}
        </v-alert>
        <v-alert
          v-if="apiError"
          type="error"
          variant="tonal"
          density="compact"
          class="mt-3"
        >
          {{ apiError }}
        </v-alert>
      </v-card-text>

      <v-card-actions class="px-6 pb-4">
        <v-spacer />
        <v-btn variant="text" :disabled="busy" @click="close">
          {{ t('common.cancel') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="flat"
          :loading="busy"
          :disabled="!effectiveType || !name.trim() || !!parseError"
          @click="doImport"
        >
          {{ t('presets.import.importBtn') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<style lang="scss" scoped>
.nest-import-preset {
  background: var(--nest-surface) !important;
  border: 1px solid var(--nest-border);
  border-radius: var(--nest-radius) !important;
}
.nest-import-title {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-family: var(--nest-font-display);
  font-size: 18px;
  padding: 20px 20px 8px;
}

.nest-preset-dz {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  padding: 22px 16px;
  border: 2px dashed var(--nest-border);
  border-radius: var(--nest-radius);
  background: var(--nest-bg-elevated);
  cursor: pointer;
  transition: border-color var(--nest-transition-base), background var(--nest-transition-base);

  &:hover, &.dragging {
    border-color: var(--nest-accent);
    background: var(--nest-bg);
  }
  &.hasfile {
    border-style: solid;
    border-color: var(--nest-green);
  }
}
.nest-dz-title {
  font-family: var(--nest-font-display);
  font-size: 15px;
  color: var(--nest-text);
}
.nest-dz-sub {
  font-size: 12px;
  color: var(--nest-text-muted);
  margin-top: 4px;
}
.text-warning {
  color: var(--nest-amber, #c9882a);
}
</style>
