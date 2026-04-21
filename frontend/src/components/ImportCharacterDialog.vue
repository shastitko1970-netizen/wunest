<script setup lang="ts">
import { ref } from 'vue'
import { useCharactersStore } from '@/stores/characters'

defineProps<{
  modelValue: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'imported', id: string): void
}>()

const store = useCharactersStore()

const isDragging = ref(false)
const busy = ref(false)
const error = ref<string | null>(null)
const selectedFile = ref<File | null>(null)
const inputEl = ref<HTMLInputElement | null>(null)

function close() {
  emit('update:modelValue', false)
  // Reset local state for the next open.
  selectedFile.value = null
  error.value = null
  busy.value = false
  isDragging.value = false
}

function pickFile() {
  inputEl.value?.click()
}

function onFileInput(e: Event) {
  const el = e.target as HTMLInputElement
  const file = el.files?.[0]
  if (file) selectedFile.value = file
}

function onDrop(e: DragEvent) {
  e.preventDefault()
  isDragging.value = false
  const file = e.dataTransfer?.files?.[0]
  if (file) selectedFile.value = file
}

async function upload() {
  if (!selectedFile.value) return
  busy.value = true
  error.value = null
  try {
    const char = await store.importPNG(selectedFile.value)
    emit('imported', char.id)
    close()
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <v-dialog
    :model-value="modelValue"
    max-width="520"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <v-card class="nest-import">
      <v-card-title class="nest-import-title">
        <span>Import character card</span>
        <v-btn icon="mdi-close" variant="text" size="small" @click="close" />
      </v-card-title>

      <v-card-text>
        <div
          class="nest-dropzone"
          :class="{ dragging: isDragging, hasfile: selectedFile }"
          @click="pickFile"
          @dragover.prevent="isDragging = true"
          @dragleave.prevent="isDragging = false"
          @drop="onDrop"
        >
          <input
            ref="inputEl"
            type="file"
            accept="image/png"
            hidden
            @change="onFileInput"
          />
          <template v-if="!selectedFile">
            <v-icon size="36" class="mb-2" color="primary">mdi-image-plus</v-icon>
            <div class="nest-dz-title">Drop a PNG card here</div>
            <div class="nest-dz-sub">Supports V2 (chara) and V3 (ccv3) SillyTavern-compatible cards</div>
            <v-btn
              class="mt-4"
              color="primary"
              variant="flat"
              size="small"
              @click.stop="pickFile"
            >
              Browse files
            </v-btn>
          </template>
          <template v-else>
            <v-icon size="28" color="success">mdi-file-image-outline</v-icon>
            <div class="nest-dz-title mt-2">{{ selectedFile.name }}</div>
            <div class="nest-dz-sub nest-mono">
              {{ (selectedFile.size / 1024).toFixed(1) }} KB
            </div>
            <v-btn
              class="mt-4"
              variant="text"
              size="small"
              @click.stop="selectedFile = null"
            >
              Choose a different file
            </v-btn>
          </template>
        </div>

        <v-alert
          v-if="error"
          type="error"
          variant="tonal"
          density="compact"
          class="mt-4"
        >
          {{ error }}
        </v-alert>
      </v-card-text>

      <v-card-actions class="px-6 pb-4">
        <v-spacer />
        <v-btn variant="text" @click="close" :disabled="busy">Cancel</v-btn>
        <v-btn
          color="primary"
          variant="flat"
          :loading="busy"
          :disabled="!selectedFile || busy"
          @click="upload"
        >
          Import
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<style lang="scss" scoped>
.nest-import {
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

.nest-dropzone {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
  padding: 32px 24px;
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
  font-size: 16px;
  color: var(--nest-text);
}

.nest-dz-sub {
  font-size: 12px;
  color: var(--nest-text-muted);
  margin-top: 4px;
}
</style>
