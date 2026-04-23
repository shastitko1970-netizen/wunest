<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useDisplay } from 'vuetify'
import { useChatsStore } from '@/stores/chats'
import { byokApi, type BYOKKey } from '@/api/byok'
import type { Chat } from '@/api/chats'

// Picker for the chat's active BYOK key. Lists user's saved keys
// (grouped by provider) plus a "Use WuApi key" option that clears the
// per-chat pin. Applies immediately on click and mutates the cached
// chat so the header updates without a refetch. Mirrors the shape of
// PersonaPickerDialog for consistency.

const { t } = useI18n()
const { smAndDown } = useDisplay()

const props = defineProps<{
  modelValue: boolean
  chat: Chat | null
}>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
}>()

const chats = useChatsStore()

const items = ref<BYOKKey[]>([])
const loading = ref(false)
const busy = ref(false)
const apiError = ref<string | null>(null)

watch(() => props.modelValue, async (open) => {
  if (!open) return
  apiError.value = null
  busy.value = false
  loading.value = true
  try {
    const r = await byokApi.list()
    items.value = r.items
  } catch (e) {
    apiError.value = (e as Error).message
  } finally {
    loading.value = false
  }
})

const pinnedId = computed(() => props.chat?.chat_metadata?.byok_id ?? null)

const resolvedLabel = computed(() => {
  if (pinnedId.value) {
    const k = items.value.find(x => x.id === pinnedId.value)
    if (k) return `${providerLabel(k.provider)} · ${k.label || k.masked}`
  }
  return t('byok.picker.useDefault')
})

const groupedByProvider = computed<Record<string, BYOKKey[]>>(() => {
  const g: Record<string, BYOKKey[]> = {}
  for (const k of items.value) {
    if (!g[k.provider]) g[k.provider] = []
    g[k.provider].push(k)
  }
  return g
})

async function pick(byokID: string | null) {
  if (!props.chat) return
  busy.value = true
  apiError.value = null
  try {
    await byokApi.setForChat(props.chat.id, byokID)
    // Mutate the cached chat so the header updates immediately.
    const cur = chats.currentChat
    if (cur && cur.id === props.chat.id) {
      cur.chat_metadata = { ...(cur.chat_metadata ?? {}), byok_id: byokID }
    }
    const idx = chats.list.findIndex((c: { id: string }) => c.id === props.chat!.id)
    if (idx >= 0) {
      chats.list[idx] = {
        ...chats.list[idx],
        chat_metadata: { ...(chats.list[idx].chat_metadata ?? {}), byok_id: byokID },
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

function providerLabel(p: string): string {
  return p.charAt(0).toUpperCase() + p.slice(1)
}
</script>

<template>
  <v-dialog
    :model-value="modelValue"
    :max-width="smAndDown ? undefined : 460"
    :fullscreen="smAndDown"
    scrollable
    @update:model-value="emit('update:modelValue', $event)"
  >
    <v-card class="nest-byok-pick">
      <v-card-title class="nest-byok-pick-title">
        <div>
          <div class="nest-eyebrow">{{ t('byok.picker.title') }}</div>
          <span class="nest-h3 mt-1">{{ resolvedLabel }}</span>
        </div>
        <v-btn icon="mdi-close" variant="text" size="small" @click="close" />
      </v-card-title>

      <v-card-text>
        <p class="nest-subtitle mb-3">{{ t('byok.picker.hint') }}</p>

        <v-alert
          v-if="apiError"
          type="error"
          variant="tonal"
          density="compact"
          class="mb-3"
        >
          {{ apiError }}
        </v-alert>

        <!-- "Use WuApi key" (pinnedId === null) -->
        <button
          class="nest-pick-row"
          :class="{ active: !pinnedId }"
          :disabled="busy"
          @click="pick(null)"
        >
          <v-avatar :size="32" color="surface-variant">
            <v-icon size="18">mdi-cloud-outline</v-icon>
          </v-avatar>
          <div class="nest-pick-body">
            <div class="nest-pick-name">{{ t('byok.picker.useDefault') }}</div>
            <div class="nest-pick-meta">{{ t('byok.picker.useDefaultHint') }}</div>
          </div>
          <v-icon v-if="!pinnedId" size="18" color="primary">mdi-check-circle</v-icon>
        </button>

        <v-divider class="my-2" />

        <div v-if="loading && !items.length" class="text-center py-3">
          <v-progress-circular indeterminate size="22" />
        </div>
        <div v-else-if="items.length === 0" class="nest-pick-empty">
          {{ t('byok.picker.emptyHint') }}
        </div>
        <div v-else class="nest-byok-pick-groups">
          <div
            v-for="(keys, provider) in groupedByProvider"
            :key="provider"
            class="nest-byok-pick-group"
          >
            <div class="nest-byok-pick-group-label nest-mono">{{ providerLabel(provider) }}</div>
            <div class="nest-pick-list">
              <button
                v-for="k in keys"
                :key="k.id"
                class="nest-pick-row"
                :class="{ active: pinnedId === k.id }"
                :disabled="busy"
                @click="pick(k.id)"
              >
                <v-avatar :size="32" color="surface-variant">
                  <v-icon size="18">mdi-key-variant</v-icon>
                </v-avatar>
                <div class="nest-pick-body">
                  <div class="nest-pick-name">
                    {{ k.label || t('byok.unnamed') }}
                  </div>
                  <div class="nest-pick-meta nest-mono">{{ k.masked }}</div>
                </div>
                <v-icon v-if="pinnedId === k.id" size="18" color="primary">mdi-check-circle</v-icon>
              </button>
            </div>
          </div>
        </div>
      </v-card-text>
    </v-card>
  </v-dialog>
</template>

<style lang="scss" scoped>
.nest-byok-pick {
  background: var(--nest-surface) !important;
  border: 1px solid var(--nest-border);
  border-radius: var(--nest-radius) !important;
}
.nest-byok-pick-title {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  padding: 20px 20px 8px;
}

.nest-byok-pick-groups {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.nest-byok-pick-group-label {
  font-size: 10.5px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--nest-text-muted);
  margin-bottom: 4px;
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
