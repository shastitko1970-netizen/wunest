<script setup lang="ts">
import { ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useWorldsStore } from '@/stores/worlds'
import { worldsApi } from '@/api/worlds'
import type { Character } from '@/api/characters'

// A little dialog for the one action nobody else gives us: picking which
// lorebooks a character pulls context from. Multi-select; writes happen
// one-at-a-time so we don't need a batch endpoint.
const { t } = useI18n()

const props = defineProps<{
  modelValue: boolean
  character: Character | null
}>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
}>()

const worlds = useWorldsStore()
const { items, loading } = storeToRefs(worlds)

const attached = ref<Set<string>>(new Set())
const busy = ref(false)
const apiError = ref<string | null>(null)

watch(() => props.modelValue, async (open) => {
  if (!open || !props.character) return
  apiError.value = null
  attached.value = new Set()
  await worlds.fetchAll()
  try {
    const resp = await worldsApi.listForCharacter(props.character.id)
    attached.value = new Set(resp.world_ids)
  } catch (e) {
    apiError.value = (e as Error).message
  }
})

async function toggle(worldID: string) {
  if (!props.character) return
  busy.value = true
  apiError.value = null
  try {
    if (attached.value.has(worldID)) {
      await worldsApi.detach(props.character.id, worldID)
      attached.value.delete(worldID)
    } else {
      await worldsApi.attach(props.character.id, worldID)
      attached.value.add(worldID)
    }
    // Force reactivity.
    attached.value = new Set(attached.value)
  } catch (e) {
    apiError.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

function close() {
  emit('update:modelValue', false)
}
</script>

<template>
  <v-dialog
    :model-value="modelValue"
    max-width="480"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <v-card class="nest-char-worlds">
      <v-card-title class="nest-char-worlds-title">
        <div>
          <div class="nest-eyebrow">{{ character?.name || '' }}</div>
          <span>{{ t('worlds.attach.title') }}</span>
        </div>
        <v-btn icon="mdi-close" variant="text" size="small" @click="close" />
      </v-card-title>

      <v-card-text>
        <p class="nest-subtitle mb-3">{{ t('worlds.attach.hint') }}</p>

        <v-alert
          v-if="apiError"
          type="error"
          variant="tonal"
          density="compact"
          class="mb-3"
        >
          {{ apiError }}
        </v-alert>

        <div v-if="loading && !items.length" class="py-4 text-center">
          <v-progress-circular indeterminate color="primary" size="22" />
        </div>
        <div v-else-if="items.length === 0" class="nest-attach-empty">
          {{ t('worlds.attach.empty') }}
        </div>
        <div v-else class="nest-attach-list">
          <button
            v-for="b in items"
            :key="b.id"
            class="nest-attach-row"
            :class="{ checked: attached.has(b.id) }"
            :disabled="busy"
            @click="toggle(b.id)"
          >
            <v-icon size="18" :color="attached.has(b.id) ? 'primary' : 'surface-variant'">
              {{ attached.has(b.id) ? 'mdi-checkbox-marked-outline' : 'mdi-checkbox-blank-outline' }}
            </v-icon>
            <div class="nest-attach-main">
              <div class="nest-attach-name">{{ b.name }}</div>
              <div class="nest-attach-meta">
                <span class="nest-mono">{{ b.entry_count }}</span> {{ t('worlds.entriesShort') }}
                <template v-if="b.description">· {{ b.description }}</template>
              </div>
            </div>
          </button>
        </div>
      </v-card-text>

      <v-card-actions class="px-6 pb-4">
        <v-spacer />
        <v-btn variant="text" @click="close">{{ t('common.close') }}</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<style lang="scss" scoped>
.nest-char-worlds {
  background: var(--nest-surface) !important;
  border: 1px solid var(--nest-border);
  border-radius: var(--nest-radius) !important;
}
.nest-char-worlds-title {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  font-family: var(--nest-font-display);
  font-size: 17px;
  padding: 20px 20px 8px;
}

.nest-attach-empty {
  color: var(--nest-text-muted);
  font-size: 13px;
  padding: 12px 0;
}

.nest-attach-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.nest-attach-row {
  all: unset;
  display: flex;
  gap: 10px;
  align-items: flex-start;
  padding: 8px 10px;
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
  cursor: pointer;
  transition: border-color var(--nest-transition-fast), background var(--nest-transition-fast);

  &:hover { border-color: var(--nest-border); }
  &.checked { border-color: var(--nest-accent); background: var(--nest-bg); }
  &[disabled] { cursor: progress; opacity: 0.6; }
}
.nest-attach-main { flex: 1; min-width: 0; }
.nest-attach-name {
  font-size: 13px;
  color: var(--nest-text);
  font-weight: 500;
}
.nest-attach-meta {
  font-size: 11.5px;
  color: var(--nest-text-muted);
  margin-top: 2px;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}
</style>
