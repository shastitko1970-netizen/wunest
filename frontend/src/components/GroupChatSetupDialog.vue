<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useDisplay } from 'vuetify'
import { useCharactersStore } from '@/stores/characters'
import { useChatsStore } from '@/stores/chats'
import type { Character } from '@/api/characters'

// Minimum / maximum participants in v1. Hard floor = 2 (single-char is the
// regular chat flow); soft ceiling = 6 to keep prompt size sane and
// speaker-picker UI uncrowded. Backend has no enforced upper bound.
const MIN_PARTICIPANTS = 2
const MAX_PARTICIPANTS = 6

const { t } = useI18n()
const { smAndDown } = useDisplay()

const props = defineProps<{
  modelValue: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  /** Fired when the group chat has been created. Parent usually
   *  router.push()'es to /chat/{id}. */
  (e: 'created', chatId: string): void
}>()

const charsStore = useCharactersStore()
const { items: characters } = storeToRefs(charsStore)
const chatsStore = useChatsStore()

const selected = ref<string[]>([])
const search = ref('')
const chatName = ref('')
const creating = ref(false)
const createError = ref<string | null>(null)

// Reset state every time the dialog opens; we don't want a previous
// abandoned draft to leak into the next session.
watch(() => props.modelValue, async (open) => {
  if (open) {
    selected.value = []
    search.value = ''
    chatName.value = ''
    createError.value = null
    creating.value = false
    if (!characters.value.length) {
      await charsStore.fetchAll()
    }
  }
})

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return characters.value
  return characters.value.filter(c =>
    c.name.toLowerCase().includes(q)
    || (c.tags ?? []).some(t => t.toLowerCase().includes(q)),
  )
})

const canCreate = computed(() =>
  selected.value.length >= MIN_PARTICIPANTS
  && selected.value.length <= MAX_PARTICIPANTS
  && !creating.value,
)

// Summary chip — shows "Alice + Bob + 1" for quick glance feedback
// while the user is still picking.
const selectedSummary = computed(() => {
  const names = selected.value
    .map(id => characters.value.find(c => c.id === id)?.name)
    .filter((n): n is string => !!n)
  if (names.length === 0) return ''
  if (names.length <= 2) return names.join(' + ')
  return `${names[0]} + ${names[1]} + ${names.length - 2}`
})

function toggle(c: Character) {
  const i = selected.value.indexOf(c.id)
  if (i >= 0) {
    selected.value.splice(i, 1)
  } else if (selected.value.length < MAX_PARTICIPANTS) {
    selected.value.push(c.id)
  }
}

function isSelected(c: Character): boolean {
  return selected.value.includes(c.id)
}

async function create() {
  createError.value = null
  creating.value = true
  try {
    const chat = await chatsStore.createGroupChat(selected.value, chatName.value.trim() || undefined)
    emit('created', chat.id)
    emit('update:modelValue', false)
  } catch (err) {
    createError.value = (err as Error).message
  } finally {
    creating.value = false
  }
}

function close() {
  if (creating.value) return
  emit('update:modelValue', false)
}
</script>

<template>
  <v-dialog
    :model-value="modelValue"
    :max-width="smAndDown ? undefined : 680"
    :fullscreen="smAndDown"
    scrollable
    @update:model-value="close"
  >
    <v-card class="nest-group-setup">
      <v-card-title class="d-flex align-center ga-2">
        <v-icon size="20">mdi-account-multiple-plus-outline</v-icon>
        {{ t('groupChat.setup.title') }}
      </v-card-title>
      <v-card-subtitle class="text-body-2">
        {{ t('groupChat.setup.subtitle', { min: MIN_PARTICIPANTS, max: MAX_PARTICIPANTS }) }}
      </v-card-subtitle>

      <v-card-text class="pt-2">
        <v-text-field
          v-model="chatName"
          :label="t('groupChat.setup.nameLabel')"
          :placeholder="t('groupChat.setup.namePlaceholder')"
          density="compact"
          variant="outlined"
          hide-details
          class="mb-3"
        />

        <v-text-field
          v-model="search"
          prepend-inner-icon="mdi-magnify"
          :placeholder="t('groupChat.setup.search')"
          density="compact"
          variant="outlined"
          hide-details
          class="mb-3"
        />

        <div class="nest-selected-bar" v-if="selected.length">
          <v-icon size="14" class="mr-1">mdi-account-multiple</v-icon>
          <span>{{ selectedSummary }}</span>
          <span class="nest-selected-count">
            {{ selected.length }} / {{ MAX_PARTICIPANTS }}
          </span>
        </div>

        <div v-if="filtered.length === 0" class="nest-empty text-center py-8">
          <v-icon size="32" color="medium-emphasis">mdi-account-off-outline</v-icon>
          <div class="text-body-2 mt-2">{{ t('groupChat.setup.empty') }}</div>
        </div>
        <div v-else class="nest-character-grid">
          <button
            v-for="c in filtered"
            :key="c.id"
            type="button"
            class="nest-character-tile"
            :class="{ selected: isSelected(c), disabled: !isSelected(c) && selected.length >= MAX_PARTICIPANTS }"
            :disabled="!isSelected(c) && selected.length >= MAX_PARTICIPANTS"
            @click="toggle(c)"
          >
            <v-avatar :size="44" :color="c.avatar_url ? undefined : 'surface-variant'">
              <img v-if="c.avatar_url" :src="c.avatar_url" :alt="c.name" />
              <span v-else class="text-body-2">{{ c.name.slice(0, 2).toUpperCase() }}</span>
            </v-avatar>
            <div class="nest-character-meta">
              <div class="nest-character-name">{{ c.name }}</div>
              <div v-if="c.data?.description" class="nest-character-desc">
                {{ c.data.description.slice(0, 60) }}{{ c.data.description.length > 60 ? '…' : '' }}
              </div>
            </div>
            <v-icon
              class="nest-character-check"
              :icon="isSelected(c) ? 'mdi-check-circle' : 'mdi-circle-outline'"
              :color="isSelected(c) ? 'primary' : 'medium-emphasis'"
              size="20"
            />
          </button>
        </div>

        <v-alert
          v-if="createError"
          type="error"
          variant="tonal"
          density="compact"
          class="mt-3"
          closable
          @click:close="createError = null"
        >
          {{ createError }}
        </v-alert>
      </v-card-text>

      <v-card-actions>
        <v-btn variant="text" :disabled="creating" @click="close">
          {{ t('common.cancel') }}
        </v-btn>
        <v-spacer />
        <v-btn
          variant="flat"
          color="primary"
          prepend-icon="mdi-check"
          :disabled="!canCreate"
          :loading="creating"
          @click="create"
        >
          {{ t('groupChat.setup.create') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<style lang="scss" scoped>
.nest-group-setup {
  background: var(--nest-surface);
}
.nest-selected-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  margin-bottom: 10px;
  font-size: 13px;
  background: var(--nest-bg-elevated);
  border-radius: var(--nest-radius);
  border: 1px solid var(--nest-border-subtle);
}
.nest-selected-count {
  margin-left: auto;
  font-size: 11px;
  color: var(--nest-text-muted);
  font-variant-numeric: tabular-nums;
}

.nest-character-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 8px;
}

.nest-character-tile {
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  background: var(--nest-bg-elevated);
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius);
  text-align: left;
  cursor: pointer;
  transition:
    border-color var(--nest-transition-fast),
    background var(--nest-transition-fast),
    transform var(--nest-transition-fast);

  &:hover:not(.disabled) {
    border-color: var(--nest-accent);
  }
  &.selected {
    border-color: var(--nest-accent);
    background: color-mix(in srgb, var(--nest-accent) 10%, var(--nest-bg-elevated));
  }
  &.disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
}
.nest-character-meta {
  min-width: 0;
}
.nest-character-name {
  font-weight: 600;
  font-size: 13.5px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.nest-character-desc {
  font-size: 11.5px;
  color: var(--nest-text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-top: 2px;
}
.nest-character-check {
  flex-shrink: 0;
}

.nest-empty {
  color: var(--nest-text-muted);
}
</style>
