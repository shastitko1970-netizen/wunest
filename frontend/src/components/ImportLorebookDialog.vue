<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useDisplay } from 'vuetify'
import { useWorldsStore } from '@/stores/worlds'

// Import a SillyTavern lorebook .json file. Accepts both shapes:
//   - { name, entries: [...] }
//   - { name, entries: { "0": {...}, "1": {...} } }
// We send the `entries` through untouched — the server normalises.
const { t } = useI18n()
const { smAndDown } = useDisplay()

const props = defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'imported', id: string): void
}>()

const worlds = useWorldsStore()

const name = ref('')
const description = ref('')
const file = ref<File | null>(null)
const rawJson = ref<string>('')
const parseError = ref<string | null>(null)
const busy = ref(false)
const apiError = ref<string | null>(null)

watch(() => props.modelValue, (open) => {
  if (open) {
    name.value = ''
    description.value = ''
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
    const parsed = JSON.parse(rawJson.value)
    // Derive name/description from parsed file if fields aren't filled.
    if (!name.value && typeof parsed?.name === 'string') name.value = parsed.name.slice(0, 120)
    if (!name.value) name.value = f.name.replace(/\.[^.]+$/, '').slice(0, 120)
    if (!description.value && typeof parsed?.description === 'string') {
      description.value = parsed.description.slice(0, 500)
    }
  } catch {
    parseError.value = t('worlds.import.invalidJson')
  }
}

async function doImport() {
  if (!rawJson.value.trim()) {
    parseError.value = t('worlds.import.pickFile')
    return
  }
  let parsed: any
  try {
    parsed = JSON.parse(rawJson.value)
  } catch {
    parseError.value = t('worlds.import.invalidJson')
    return
  }
  const finalName = (name.value.trim() || parsed?.name || 'Imported').toString()

  busy.value = true
  apiError.value = null
  try {
    const created = await worlds.importST(
      finalName,
      description.value,
      parsed.entries ?? parsed,
    )
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
    :max-width="smAndDown ? undefined : 560"
    :fullscreen="smAndDown"
    scrollable
    @update:model-value="emit('update:modelValue', $event)"
  >
    <v-card class="nest-import-lore">
      <v-card-title class="nest-import-title">
        <span>{{ t('worlds.import.title') }}</span>
        <v-btn icon="mdi-close" variant="text" size="small" @click="close" />
      </v-card-title>

      <v-card-text>
        <v-text-field
          v-model="name"
          :label="t('worlds.nameLabel')"
          :placeholder="t('worlds.namePlaceholder')"
          density="compact"
          class="mb-3"
        />
        <v-text-field
          v-model="description"
          :label="t('worlds.descLabel')"
          :placeholder="t('worlds.descPlaceholder')"
          density="compact"
          class="mb-3"
        />

        <label class="nest-file-picker">
          <span class="nest-file-text">
            {{ file ? file.name : t('worlds.import.pickFile') }}
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
            {{ t('worlds.import.choose') }}
          </v-btn>
        </label>

        <v-alert v-if="parseError" type="error" variant="tonal" density="compact" class="mt-3">
          {{ parseError }}
        </v-alert>
        <v-alert v-if="apiError" type="error" variant="tonal" density="compact" class="mt-3">
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
          :disabled="!rawJson || !!parseError"
          @click="doImport"
        >
          {{ t('worlds.import.importBtn') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<style lang="scss" scoped>
.nest-import-lore {
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
