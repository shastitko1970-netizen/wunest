<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Character, CardAsset } from '@/api/characters'

// Sprite/expression manager — renders inside NewCharacterDialog as
// one of the expansion panels. User drops multiple PNGs, names each
// (or accepts the auto-name), clicks "Upload". Uses character's V3
// assets array as the canonical store.
//
// Backend endpoints (auth-gated):
//   POST   /api/characters/:id/sprites           multipart: file + name
//   DELETE /api/characters/:id/sprites/:name

const { t } = useI18n()

const props = defineProps<{
  character: Character | null
}>()

const emit = defineEmits<{
  (e: 'updated', c: Character): void
}>()

// Pending uploads (not yet sent). Each row has a preview URL so the
// user can see what they're about to upload.
interface PendingUpload {
  file: File
  previewUrl: string
  name: string
  uploading: boolean
  error: string | null
}

const pending = ref<PendingUpload[]>([])
const fileInput = ref<HTMLInputElement | null>(null)
const deletingName = ref<string | null>(null)

// Existing uploaded sprites — derived from character.data.assets
// filtered to type="expression".
const existingSprites = computed<CardAsset[]>(() => {
  const assets = props.character?.data?.assets ?? []
  return assets.filter(a => a.type === 'expression')
})

function pickFiles() {
  fileInput.value?.click()
}

function onFilesChosen(e: Event) {
  const input = e.target as HTMLInputElement
  const files = input.files
  if (!files || files.length === 0) return
  for (const f of Array.from(files)) {
    if (!f.type.startsWith('image/')) continue
    // Default emotion name = filename without extension, lowercased.
    const stem = f.name.replace(/\.[^.]+$/, '').toLowerCase()
    pending.value.push({
      file: f,
      previewUrl: URL.createObjectURL(f),
      name: stem || 'emotion',
      uploading: false,
      error: null,
    })
  }
  input.value = ''
}

function removePending(i: number) {
  const row = pending.value[i]
  if (row) URL.revokeObjectURL(row.previewUrl)
  pending.value.splice(i, 1)
}

async function uploadAll() {
  if (!props.character) return
  for (const row of pending.value) {
    if (row.uploading || !row.name.trim()) continue
    row.uploading = true
    row.error = null
    try {
      const fd = new FormData()
      fd.append('file', row.file)
      fd.append('name', row.name.trim())
      const res = await fetch(`/api/characters/${props.character.id}/sprites`, {
        method: 'POST',
        body: fd,
      })
      if (!res.ok) {
        const body = await res.text().catch(() => '')
        throw new Error(body || res.statusText)
      }
      const json = await res.json() as { character: Character }
      emit('updated', json.character)
    } catch (err: any) {
      row.error = err?.message || String(err)
    } finally {
      row.uploading = false
    }
  }
  // Strip successful uploads from pending; keep the failed ones for
  // retry with an edited name.
  pending.value = pending.value.filter(p => p.error !== null)
}

async function deleteSprite(name: string) {
  if (!props.character) return
  if (!confirm(t('characterSprites.confirmDelete', { name }))) return
  deletingName.value = name
  try {
    const res = await fetch(
      `/api/characters/${props.character.id}/sprites/${encodeURIComponent(name)}`,
      { method: 'DELETE' },
    )
    if (!res.ok && res.status !== 204) {
      throw new Error(await res.text() || res.statusText)
    }
    // Tell parent to re-fetch the character so data.assets drops this row.
    // Simpler path: emit with an optimistic copy minus the sprite.
    if (props.character) {
      const next = JSON.parse(JSON.stringify(props.character)) as Character
      if (next.data.assets) {
        next.data.assets = next.data.assets.filter(
          a => !(a.type === 'expression' && a.name === name),
        )
      }
      emit('updated', next)
    }
  } catch (err) {
    alert((err as Error).message)
  } finally {
    deletingName.value = null
  }
}
</script>

<template>
  <div class="nest-sprites">
    <div class="nest-hint mb-3">{{ t('characterSprites.hint') }}</div>

    <!-- Existing sprites grid -->
    <div v-if="existingSprites.length" class="nest-sprite-grid mb-4">
      <div v-for="sprite in existingSprites" :key="sprite.name" class="nest-sprite-tile">
        <div class="nest-sprite-img-wrap">
          <img :src="sprite.uri" :alt="sprite.name || 'sprite'" />
        </div>
        <div class="nest-sprite-name nest-mono">{{ sprite.name }}</div>
        <v-btn
          size="x-small"
          variant="text"
          color="error"
          icon="mdi-close"
          :loading="deletingName === sprite.name"
          @click="deleteSprite(sprite.name ?? '')"
        />
      </div>
    </div>

    <!-- Pending (pre-upload) rows — user renames + confirms before
         firing the POST. -->
    <div v-if="pending.length" class="nest-sprite-pending mb-3">
      <div v-for="(row, i) in pending" :key="i" class="nest-sprite-row">
        <img :src="row.previewUrl" :alt="row.name" class="nest-sprite-preview" />
        <v-text-field
          v-model="row.name"
          :placeholder="t('characterSprites.emotionPlaceholder')"
          density="compact"
          variant="outlined"
          hide-details
        />
        <v-btn
          size="small"
          variant="text"
          color="error"
          icon="mdi-close"
          :disabled="row.uploading"
          @click="removePending(i)"
        />
        <span v-if="row.error" class="nest-sprite-err">{{ row.error }}</span>
      </div>
      <v-btn
        size="small"
        variant="flat"
        color="primary"
        prepend-icon="mdi-upload"
        class="mt-2"
        @click="uploadAll"
      >
        {{ t('characterSprites.uploadAll', { n: pending.length }) }}
      </v-btn>
    </div>

    <!-- Add-file trigger. -->
    <input
      ref="fileInput"
      type="file"
      accept="image/png,image/jpeg,image/webp"
      multiple
      style="display:none"
      @change="onFilesChosen"
    />
    <v-btn
      size="small"
      variant="outlined"
      prepend-icon="mdi-plus"
      @click="pickFiles"
    >
      {{ t('characterSprites.add') }}
    </v-btn>
  </div>
</template>

<style lang="scss" scoped>
.nest-sprites { padding: 0; }
.nest-hint {
  font-size: 12.5px;
  color: var(--nest-text-muted);
  line-height: 1.45;
}

.nest-sprite-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
  gap: 10px;
}
.nest-sprite-tile {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 8px;
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius);
  background: var(--nest-bg-elevated);
}
.nest-sprite-img-wrap {
  width: 100%;
  aspect-ratio: 3 / 4;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  border-radius: calc(var(--nest-radius) - 2px);
  background: var(--nest-bg);

  img {
    max-width: 100%;
    max-height: 100%;
    object-fit: contain;
  }
}
.nest-sprite-name {
  font-size: 11px;
  color: var(--nest-text-secondary);
}

.nest-sprite-pending {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px 12px;
  border: 1px dashed var(--nest-border-subtle);
  border-radius: var(--nest-radius);
}
.nest-sprite-row {
  display: grid;
  grid-template-columns: 48px 1fr auto;
  align-items: center;
  gap: 8px;
}
.nest-sprite-preview {
  width: 48px;
  height: 48px;
  object-fit: cover;
  border-radius: calc(var(--nest-radius) - 2px);
  background: var(--nest-bg);
}
.nest-sprite-err {
  grid-column: 1 / -1;
  font-size: 11px;
  color: rgb(var(--v-theme-error));
}
</style>
