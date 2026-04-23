<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useDisplay } from 'vuetify'
import { usePersonasStore } from '@/stores/personas'
import { useChatsStore } from '@/stores/chats'
import { useAuthStore } from '@/stores/auth'
import { personasApi } from '@/api/personas'
import type { Chat } from '@/api/chats'

// Picker for the chat's active persona. Shows a list of the user's personas
// + a "Use default (or session name)" option that clears the per-chat
// override. Applies immediately on click.
const { t } = useI18n()
const { smAndDown } = useDisplay()

const props = defineProps<{
  modelValue: boolean
  chat: Chat | null
}>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
}>()

const personas = usePersonasStore()
const chats = useChatsStore()
const auth = useAuthStore()
const { items, loading } = storeToRefs(personas)
const { profile } = storeToRefs(auth)

const busy = ref(false)
const apiError = ref<string | null>(null)

watch(() => props.modelValue, async (open) => {
  if (!open) return
  apiError.value = null
  busy.value = false
  await personas.fetchAll()
})

// The chat's explicit override, if any.
const overrideId = computed(() => props.chat?.chat_metadata?.persona_id ?? null)

// Resolved "who you're playing as right now", for display in the header.
const resolvedLabel = computed(() => {
  if (overrideId.value) {
    const p = items.value.find(x => x.id === overrideId.value)
    if (p) return p.name
  }
  if (personas.defaultPersona) return personas.defaultPersona.name
  return profile.value?.first_name || profile.value?.username || t('personas.sessionFallback')
})

async function pick(personaID: string | null) {
  if (!props.chat) return
  busy.value = true
  apiError.value = null
  try {
    await personasApi.setForChat(props.chat.id, personaID)
    // Mutate the cached chat so the header updates immediately.
    const cur = chats.currentChat
    if (cur && cur.id === props.chat.id) {
      cur.chat_metadata = { ...(cur.chat_metadata ?? {}), persona_id: personaID }
    }
    const idx = chats.list.findIndex((c: { id: string }) => c.id === props.chat!.id)
    if (idx >= 0) {
      chats.list[idx] = {
        ...chats.list[idx],
        chat_metadata: { ...(chats.list[idx].chat_metadata ?? {}), persona_id: personaID },
      }
    }
    emit('update:modelValue', false)
  } catch (e) {
    apiError.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

function close() {
  emit('update:modelValue', false)
}

function initialsFor(name: string): string {
  if (!name) return '?'
  return name.split(/\s+/).slice(0, 2).map(w => w[0]?.toUpperCase() ?? '').join('')
}
</script>

<template>
  <v-dialog
    :model-value="modelValue"
    :max-width="smAndDown ? undefined : 440"
    :fullscreen="smAndDown"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <v-card class="nest-persona-pick">
      <v-card-title class="nest-persona-pick-title">
        <div>
          <div class="nest-eyebrow">{{ t('personas.picker.title') }}</div>
          <span class="nest-h3 mt-1">{{ resolvedLabel }}</span>
        </div>
        <v-btn icon="mdi-close" variant="text" size="small" @click="close" />
      </v-card-title>

      <v-card-text>
        <p class="nest-subtitle mb-3">{{ t('personas.picker.hint') }}</p>

        <v-alert
          v-if="apiError"
          type="error"
          variant="tonal"
          density="compact"
          class="mb-3"
        >
          {{ apiError }}
        </v-alert>

        <!-- Default / session fallback option -->
        <button
          class="nest-pick-row"
          :class="{ active: !overrideId }"
          :disabled="busy"
          @click="pick(null)"
        >
          <v-avatar :size="32" color="surface-variant">
            <v-icon size="18">mdi-account-circle-outline</v-icon>
          </v-avatar>
          <div class="nest-pick-body">
            <div class="nest-pick-name">{{ t('personas.picker.useDefault') }}</div>
            <div class="nest-pick-meta">
              {{ personas.defaultPersona
                ? t('personas.picker.defaultHint', { name: personas.defaultPersona.name })
                : t('personas.picker.sessionHint', { name: profile?.first_name || profile?.username || 'You' }) }}
            </div>
          </div>
          <v-icon v-if="!overrideId" size="18" color="primary">mdi-check-circle</v-icon>
        </button>

        <v-divider class="my-2" />

        <div v-if="loading && !items.length" class="text-center py-3">
          <v-progress-circular indeterminate size="22" />
        </div>
        <div v-else-if="items.length === 0" class="nest-pick-empty">
          {{ t('personas.picker.emptyHint') }}
        </div>
        <div v-else class="nest-pick-list">
          <button
            v-for="p in items"
            :key="p.id"
            class="nest-pick-row"
            :class="{ active: overrideId === p.id }"
            :disabled="busy"
            @click="pick(p.id)"
          >
            <v-avatar :size="32" :color="p.avatar_url ? undefined : 'surface-variant'">
              <img v-if="p.avatar_url" :src="p.avatar_url" :alt="p.name" referrerpolicy="no-referrer" />
              <span v-else class="text-caption">{{ initialsFor(p.name) }}</span>
            </v-avatar>
            <div class="nest-pick-body">
              <div class="nest-pick-name">
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
              <div v-if="p.description" class="nest-pick-meta">{{ p.description }}</div>
            </div>
            <v-icon v-if="overrideId === p.id" size="18" color="primary">mdi-check-circle</v-icon>
          </button>
        </div>
      </v-card-text>
    </v-card>
  </v-dialog>
</template>

<style lang="scss" scoped>
.nest-persona-pick {
  background: var(--nest-surface) !important;
  border: 1px solid var(--nest-border);
  border-radius: var(--nest-radius) !important;
}
.nest-persona-pick-title {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  padding: 20px 20px 8px;
}

.nest-pick-empty {
  color: var(--nest-text-muted);
  font-size: 13px;
  padding: 8px 2px;
}

.nest-pick-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.nest-pick-row {
  all: unset;
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  box-sizing: border-box;
  padding: 8px 10px;
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
  cursor: pointer;
  transition: border-color var(--nest-transition-fast), background var(--nest-transition-fast);

  &:hover { border-color: var(--nest-border); }
  &.active {
    border-color: var(--nest-accent);
    background: var(--nest-bg);
  }
  &[disabled] { cursor: progress; opacity: 0.6; }
}
.nest-pick-body { flex: 1; min-width: 0; }
.nest-pick-name {
  font-size: 13.5px;
  color: var(--nest-text);
  font-weight: 500;
  display: flex;
  align-items: center;
}
.nest-pick-meta {
  font-size: 11.5px;
  color: var(--nest-text-muted);
  margin-top: 2px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
