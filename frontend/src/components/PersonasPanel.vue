<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { usePersonasStore } from '@/stores/personas'
import type { Persona } from '@/api/personas'
import { uploadAvatar } from '@/api/uploads'

// Two-pane Personas management: list on the left, form on the right.
// Matches the WorldsPanel layout for consistency.
const { t } = useI18n()
const store = usePersonasStore()
const { items, loading, error } = storeToRefs(store)

const selectedId = ref<string | null>(null)
const draftName = ref('')
const draftDesc = ref('')
const draftAvatar = ref('')
const saving = ref(false)
const saveError = ref<string | null>(null)
const isNew = ref(false)
const confirmDeleteId = ref<string | null>(null)

// Avatar upload state — kept separate from `saveError` so an upload
// failure doesn't clobber a pending save error message, and vice versa.
const avatarFileInput = ref<HTMLInputElement | null>(null)
const avatarUploading = ref(false)
const avatarError = ref<string | null>(null)

async function onAvatarFileChange(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  try {
    avatarUploading.value = true
    avatarError.value = null
    const res = await uploadAvatar(file)
    // Only the thumbnail URL matters for personas — we don't surface the
    // full-size original anywhere in the persona UI. If that changes,
    // plumb res.avatar_original_url through Persona type + repo.
    draftAvatar.value = res.avatar_url
  } catch (err: any) {
    avatarError.value = err?.message || String(err)
  } finally {
    avatarUploading.value = false
    if (input) input.value = ''
  }
}

function pickAvatarFile() {
  avatarFileInput.value?.click()
}

function clearAvatar() {
  draftAvatar.value = ''
  avatarError.value = null
}

onMounted(async () => {
  await store.fetchAll()
  // Auto-select default or first.
  const first = store.defaultPersona ?? items.value[0]
  if (first) openPersona(first.id)
})

const selected = computed<Persona | null>(() => {
  if (!selectedId.value) return null
  return items.value.find(p => p.id === selectedId.value) ?? null
})

function openPersona(id: string) {
  const p = items.value.find(x => x.id === id)
  if (!p) return
  selectedId.value = id
  isNew.value = false
  draftName.value = p.name
  draftDesc.value = p.description
  draftAvatar.value = p.avatar_url ?? ''
  saveError.value = null
  avatarError.value = null
}

function startNew() {
  selectedId.value = null
  isNew.value = true
  draftName.value = ''
  draftDesc.value = ''
  draftAvatar.value = ''
  saveError.value = null
  avatarError.value = null
}

// ── SillyTavern persona import ──────────────────────────────────────
// ST personas ship as a tiny JSON object: `{name, description,
// avatar_url?}`. Some older exports use `avatar` instead of
// `avatar_url`, and some dump persona data as a character-card shape
// with the relevant fields inside `data`. We accept all three.
const importInput = ref<HTMLInputElement | null>(null)

async function onImportPick(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0] ?? null
  if (importInput.value) importInput.value.value = ''
  if (!f) return
  saveError.value = null
  try {
    const text = await f.text()
    const parsed = JSON.parse(text) as Record<string, unknown>
    // Try the three layouts in order of how often ST users actually ship them.
    const src: Record<string, unknown> =
      (typeof parsed.data === 'object' && parsed.data !== null)
        ? (parsed.data as Record<string, unknown>)
        : parsed
    const name = stringify(src.name) || stringify(parsed.name)
    const description = stringify(src.description) || stringify(parsed.description)
    const avatar = stringify(src.avatar_url) || stringify(src.avatar) || stringify(parsed.avatar_url)
    if (!name) {
      saveError.value = 'JSON must include a "name" field'
      return
    }
    // Seed the editor; user can review before saving.
    startNew()
    draftName.value = name
    draftDesc.value = description
    draftAvatar.value = avatar
  } catch (err) {
    saveError.value = (err as Error).message
  }
}

function stringify(v: unknown): string {
  return typeof v === 'string' ? v : ''
}

const hasDraftChanges = computed(() => {
  if (isNew.value) return draftName.value.trim().length > 0
  if (!selected.value) return false
  return draftName.value !== selected.value.name
    || draftDesc.value !== selected.value.description
    || (draftAvatar.value || '') !== (selected.value.avatar_url || '')
})

async function save() {
  saving.value = true
  saveError.value = null
  try {
    if (isNew.value) {
      const p = await store.create({
        name: draftName.value.trim(),
        description: draftDesc.value,
        avatar_url: draftAvatar.value.trim(),
        is_default: items.value.length === 0, // first one becomes default
      })
      openPersona(p.id)
    } else if (selected.value) {
      await store.update(selected.value.id, {
        name: draftName.value.trim(),
        description: draftDesc.value,
        avatar_url: draftAvatar.value.trim(),
      })
    }
  } catch (e) {
    saveError.value = (e as Error).message
  } finally {
    saving.value = false
  }
}

async function toggleDefault() {
  if (!selected.value) return
  await store.setDefault(selected.value.is_default ? null : selected.value.id)
}

async function doDelete() {
  if (!confirmDeleteId.value) return
  const id = confirmDeleteId.value
  confirmDeleteId.value = null
  try {
    await store.remove(id)
    if (selectedId.value === id) {
      selectedId.value = null
      isNew.value = false
    }
  } catch (e) {
    saveError.value = (e as Error).message
  }
}

function initialsFor(p: Persona | null): string {
  const name = (p?.name || '').trim()
  if (!name) return '?'
  return name.split(/\s+/).slice(0, 2).map(w => w[0]?.toUpperCase() ?? '').join('')
}

const draftInitials = computed(() => {
  const name = draftName.value.trim() || (selected.value?.name ?? '')
  if (!name) return '?'
  return name.split(/\s+/).slice(0, 2).map(w => w[0]?.toUpperCase() ?? '').join('')
})

watch(items, () => {
  // If current selection was removed externally, bail out gracefully.
  if (selectedId.value && !items.value.some(p => p.id === selectedId.value)) {
    selectedId.value = null
    isNew.value = false
  }
})
</script>

<template>
  <div class="nest-personas-panel">
    <!-- List -->
    <aside class="nest-personas-list">
      <div class="nest-personas-head">
        <h3 class="nest-h3">{{ t('personas.yours') }}</h3>
        <div class="d-flex ga-1">
          <v-btn
            size="small"
            variant="outlined"
            prepend-icon="mdi-upload"
            :title="t('personas.actions.import')"
            @click="importInput?.click()"
          >
            {{ t('common.import') }}
          </v-btn>
          <input
            ref="importInput"
            type="file"
            accept="application/json,.json"
            hidden
            @change="onImportPick"
          />
          <v-btn
            size="small"
            variant="flat"
            color="primary"
            prepend-icon="mdi-plus"
            @click="startNew"
          >
            {{ t('common.new') }}
          </v-btn>
        </div>
      </div>

      <div v-if="loading && !items.length" class="nest-state">
        <v-progress-circular indeterminate color="primary" size="24" />
      </div>
      <v-alert v-else-if="error" type="error" variant="tonal" density="compact">{{ error }}</v-alert>
      <div v-else-if="items.length === 0 && !isNew" class="nest-personas-empty">
        {{ t('personas.empty.hint') }}
      </div>
      <div v-else class="nest-personas-rows">
        <button
          v-for="p in items"
          :key="p.id"
          class="nest-persona-row"
          :class="{ active: selectedId === p.id }"
          @click="openPersona(p.id)"
        >
          <v-avatar :size="36" :color="p.avatar_url ? undefined : 'surface-variant'">
            <img v-if="p.avatar_url" :src="p.avatar_url" :alt="p.name" referrerpolicy="no-referrer" />
            <span v-else class="text-caption">{{ initialsFor(p) }}</span>
          </v-avatar>
          <div class="nest-persona-body">
            <div class="nest-persona-name">
              {{ p.name }}
              <v-chip
                v-if="p.is_default"
                size="x-small"
                color="secondary"
                variant="tonal"
                class="ml-2 nest-mono"
              >
                {{ t('personas.defaultBadge') }}
              </v-chip>
            </div>
            <div v-if="p.description" class="nest-persona-desc">{{ p.description }}</div>
          </div>
        </button>
      </div>
    </aside>

    <!-- Editor -->
    <section class="nest-persona-editor">
      <div v-if="!isNew && !selected" class="nest-state">
        <v-icon size="40" color="surface-variant">mdi-account-circle-outline</v-icon>
        <p class="nest-subtitle mt-3">{{ t('personas.pickOrCreate') }}</p>
      </div>
      <template v-else>
        <div class="nest-editor-head">
          <div class="nest-editor-head-left">
            <!-- Click-to-upload: portrait-aspect preview (3:4) with
                 object-fit: contain so the user sees the WHOLE uploaded
                 image — no face-cropping into a circle. Same file picker
                 the Upload button uses, so power users can do either. -->
            <button
              type="button"
              class="nest-persona-avatar-btn"
              :class="{ empty: !draftAvatar }"
              :title="t('library.create.avatarUpload')"
              :disabled="avatarUploading"
              @click="pickAvatarFile"
            >
              <img
                v-if="draftAvatar"
                :src="draftAvatar"
                :alt="draftName"
                referrerpolicy="no-referrer"
              />
              <span v-else class="text-body-1">{{ draftInitials }}</span>
              <!-- Camera edit badge only when there's an actual avatar to
                   overlay. On empty (initials-only) state it clutters the
                   clean letter mark — hover already signals clickability. -->
              <v-icon v-if="draftAvatar" class="nest-persona-avatar-edit" size="14">mdi-camera</v-icon>
              <v-progress-circular
                v-if="avatarUploading"
                class="nest-persona-avatar-spinner"
                size="20"
                width="2"
                indeterminate
                color="primary"
              />
            </button>
            <div>
              <div class="nest-eyebrow">
                {{ isNew ? t('personas.newTitle') : t('personas.editTitle') }}
              </div>
              <div class="nest-h3">{{ draftName || t('personas.unnamed') }}</div>
            </div>
          </div>
          <div class="nest-editor-head-right">
            <v-btn
              v-if="!isNew && selected"
              size="small"
              :variant="selected.is_default ? 'tonal' : 'text'"
              :color="selected.is_default ? 'secondary' : undefined"
              :prepend-icon="selected.is_default ? 'mdi-star' : 'mdi-star-outline'"
              @click="toggleDefault"
            >
              {{ selected.is_default ? t('personas.actions.unsetDefault') : t('personas.actions.setDefault') }}
            </v-btn>
            <v-btn
              v-if="!isNew && selected"
              size="small"
              variant="text"
              color="error"
              prepend-icon="mdi-delete-outline"
              @click="confirmDeleteId = selected.id"
            >
              {{ t('common.delete') }}
            </v-btn>
          </div>
        </div>

        <v-alert v-if="saveError" type="error" variant="tonal" density="compact" class="mb-3">
          {{ saveError }}
        </v-alert>

        <v-text-field
          v-model="draftName"
          :label="t('personas.nameLabel')"
          :placeholder="t('personas.namePlaceholder')"
          density="compact"
          class="mb-3"
        />

        <v-textarea
          v-model="draftDesc"
          :label="t('personas.descLabel')"
          :placeholder="t('personas.descPlaceholder')"
          rows="5"
          auto-grow
          density="compact"
          class="mb-3"
        />

        <div class="nest-persona-avatar-row mb-3">
          <v-text-field
            v-model="draftAvatar"
            :label="t('personas.avatarLabel')"
            :placeholder="t('personas.avatarPlaceholder')"
            density="compact"
            hide-details
            style="flex: 1"
          />
          <!-- Hidden <input type="file"> — both the head avatar and the
               Upload button trigger this via pickAvatarFile(). -->
          <input
            ref="avatarFileInput"
            type="file"
            accept="image/png,image/jpeg,image/webp,image/gif"
            style="display:none"
            @change="onAvatarFileChange"
          />
          <v-btn
            size="small"
            variant="tonal"
            prepend-icon="mdi-upload"
            :loading="avatarUploading"
            @click="pickAvatarFile"
          >
            {{ t('library.create.avatarUpload') }}
          </v-btn>
          <v-btn
            v-if="draftAvatar"
            size="small"
            variant="text"
            color="error"
            icon="mdi-close"
            :title="t('library.create.avatarClear')"
            @click="clearAvatar"
          />
        </div>
        <v-alert
          v-if="avatarError"
          type="error"
          density="compact"
          variant="tonal"
          closable
          class="mb-3"
          @click:close="avatarError = null"
        >
          {{ avatarError }}
        </v-alert>

        <div class="d-flex justify-end">
          <v-btn
            :disabled="!hasDraftChanges"
            :loading="saving"
            color="primary"
            variant="flat"
            prepend-icon="mdi-content-save"
            @click="save"
          >
            {{ isNew ? t('personas.actions.create') : t('common.save') }}
          </v-btn>
        </div>
      </template>
    </section>

    <!-- Delete confirmation -->
    <v-dialog
      :model-value="confirmDeleteId !== null"
      max-width="400"
      @update:model-value="v => !v && (confirmDeleteId = null)"
    >
      <v-card class="nest-confirm">
        <v-card-title>{{ t('personas.delete.title') }}</v-card-title>
        <v-card-text>{{ t('personas.delete.body') }}</v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="confirmDeleteId = null">{{ t('common.cancel') }}</v-btn>
          <v-btn color="error" variant="flat" @click="doDelete">{{ t('common.delete') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<style lang="scss" scoped>
.nest-personas-panel {
  display: grid;
  grid-template-columns: 320px 1fr;
  gap: 16px;
  min-height: 500px;
}

.nest-personas-list {
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius);
  padding: 12px;
  background: var(--nest-surface);
  display: flex;
  flex-direction: column;
  gap: 10px;
  max-height: 80vh;
  overflow-y: auto;
}
.nest-personas-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.nest-personas-rows {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.nest-persona-row {
  all: unset;
  display: flex;
  gap: 10px;
  align-items: flex-start;
  padding: 8px 10px;
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
  background: var(--nest-bg);
  cursor: pointer;
  transition: border-color var(--nest-transition-fast), background var(--nest-transition-fast);

  &:hover { border-color: var(--nest-border); }
  &.active {
    border-color: var(--nest-accent);
    background: var(--nest-surface);
  }
}
.nest-persona-body { min-width: 0; flex: 1; }
.nest-persona-name {
  font-size: 13.5px;
  font-weight: 500;
  color: var(--nest-text);
  display: flex;
  align-items: center;
}
.nest-persona-desc {
  font-size: 11.5px;
  color: var(--nest-text-muted);
  margin-top: 3px;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.nest-personas-empty {
  color: var(--nest-text-muted);
  font-size: 13px;
  padding: 12px 4px;
}

.nest-persona-editor {
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius);
  padding: 20px;
  background: var(--nest-surface);
  min-width: 0;
}

.nest-state {
  padding: 60px 24px;
  display: grid;
  place-items: center;
  color: var(--nest-text-muted);
  text-align: center;
}

.nest-editor-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  padding-bottom: 16px;
  border-bottom: 1px solid var(--nest-border-subtle);
  margin-bottom: 16px;
  flex-wrap: wrap;
}
.nest-editor-head-left {
  display: flex;
  gap: 14px;
  align-items: center;
}
.nest-editor-head-right {
  display: flex;
  gap: 6px;
}

// Click-to-upload avatar button. Portrait 3:4 box with object-fit:
// contain so the whole character art is visible while editing — cards
// (which use the global avatarStyle setting) can still crop for
// density. Overlay camera badge + hover lift cue interactivity.
.nest-persona-avatar-btn {
  position: relative;
  flex-shrink: 0;
  width: 64px;
  aspect-ratio: 3 / 4;
  padding: 0;
  border: 0;
  background: var(--nest-surface-variant, var(--nest-bg-elevated));
  overflow: hidden;
  cursor: pointer;
  // Token instead of hardcoded 12 — matches the persona tile radius
  // scale with the global --nest-radius token.
  border-radius: var(--nest-radius);
  display: flex;
  align-items: center;
  justify-content: center;
  transition: transform var(--nest-transition-fast), box-shadow var(--nest-transition-fast);

  img {
    width: 100%;
    height: 100%;
    object-fit: contain;
  }
  span {
    color: var(--nest-text-muted);
  }

  &:hover {
    transform: scale(1.03);
  }
  &:disabled {
    cursor: wait;
    opacity: 0.85;
  }
  &:focus-visible {
    outline: 2px solid var(--nest-accent);
    outline-offset: 2px;
  }
}
.nest-persona-avatar-edit {
  position: absolute;
  bottom: -2px;
  right: -2px;
  background: var(--nest-accent);
  color: var(--nest-text-on-accent, #fff);
  border: 2px solid var(--nest-surface);
  border-radius: 50%;
  padding: 3px;
}
.nest-persona-avatar-spinner {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
}

// URL field + Upload + Clear buttons in one row. Wraps on narrow mobile.
.nest-persona-avatar-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.nest-confirm {
  background: var(--nest-surface) !important;
  border: 1px solid var(--nest-border);
}

// DS primary 960px: two-column personas panel collapses to single column.
// Previously 860 — unified to DS allowed-list.
@media (max-width: 960px) {
  .nest-personas-panel { grid-template-columns: 1fr; }
  .nest-personas-list { max-height: none; }
}
</style>
