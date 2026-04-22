<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { usePresetsStore } from '@/stores/presets'
import { PRESET_TYPES, type PresetType } from '@/api/presets'

// Import SillyTavern-style preset JSON. No schema validation — we accept
// any object and store it as-is. That keeps weird fields (textgen-specific
// samplers, etc.) lossless for power users, at the cost of being able to
// surface them in the default editor without follow-up work.
const { t } = useI18n()

const props = defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'imported', id: string): void
}>()

const presets = usePresetsStore()

const selectedType = ref<PresetType>('sampler')
const name = ref('')
const file = ref<File | null>(null)
const rawJson = ref<string>('')
const parseError = ref<string | null>(null)
const busy = ref(false)
const apiError = ref<string | null>(null)

const typeOptions = computed(() =>
  PRESET_TYPES.map(type => ({
    value: type,
    label: t(`presets.type.${type}`),
  })),
)

watch(() => props.modelValue, (open) => {
  if (open) {
    selectedType.value = 'sampler'
    name.value = ''
    file.value = null
    rawJson.value = ''
    parseError.value = null
    apiError.value = null
    busy.value = false
  }
})

function close() {
  emit('update:modelValue', false)
}

async function onFile(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0] ?? null
  file.value = f
  parseError.value = null
  apiError.value = null
  if (!f) {
    rawJson.value = ''
    return
  }
  try {
    rawJson.value = await f.text()
    // Derive a default name from the file name if the user hasn't typed
    // one yet. Strip extension.
    if (!name.value) {
      name.value = f.name.replace(/\.[^.]+$/, '').slice(0, 60)
    }
    // Parse-only sanity check — reject clearly broken JSON early.
    JSON.parse(rawJson.value)
  } catch (err) {
    parseError.value = t('presets.import.invalidJson')
  }
}

async function doImport() {
  if (!rawJson.value.trim()) {
    parseError.value = t('presets.import.pickFile')
    return
  }
  let data: unknown
  try {
    data = JSON.parse(rawJson.value)
  } catch {
    parseError.value = t('presets.import.invalidJson')
    return
  }
  if (!name.value.trim()) {
    apiError.value = t('presets.import.nameRequired')
    return
  }
  busy.value = true
  apiError.value = null
  try {
    const created = await presets.create(selectedType.value, name.value.trim(), data)
    emit('imported', created.id)
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
    max-width="560"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <v-card class="nest-import-preset">
      <v-card-title class="nest-import-title">
        <span>{{ t('presets.import.title') }}</span>
        <v-btn icon="mdi-close" variant="text" size="small" @click="close" />
      </v-card-title>

      <v-card-text>
        <v-select
          v-model="selectedType"
          :items="typeOptions"
          item-title="label"
          item-value="value"
          :label="t('presets.import.typeLabel')"
          density="compact"
          hide-details
          class="mb-3"
        />

        <v-text-field
          v-model="name"
          :label="t('presets.import.nameLabel')"
          :placeholder="t('presets.import.namePlaceholder')"
          density="compact"
          class="mb-3"
        />

        <label class="nest-file-picker">
          <span class="nest-file-text">
            {{ file ? file.name : t('presets.import.pickFile') }}
          </span>
          <input
            type="file"
            accept="application/json,.json"
            hidden
            @change="onFile"
          />
          <v-btn
            variant="outlined"
            size="small"
            prepend-icon="mdi-file-upload-outline"
            component="span"
          >
            {{ t('presets.import.choose') }}
          </v-btn>
        </label>

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
          :disabled="!rawJson || !name.trim() || !!parseError"
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
.nest-file-picker {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border: 1px dashed var(--nest-border);
  border-radius: var(--nest-radius-sm);
  cursor: pointer;

  &:hover { border-color: var(--nest-accent); }
}
.nest-file-text {
  flex: 1;
  font-size: 13px;
  color: var(--nest-text-secondary);
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}
</style>
