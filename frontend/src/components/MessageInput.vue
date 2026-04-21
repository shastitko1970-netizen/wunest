<script setup lang="ts">
import { computed, ref, watch, nextTick, onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { useModelsStore } from '@/stores/models'

const props = defineProps<{
  modelValue: string
  disabled?: boolean
  streaming?: boolean
  placeholder?: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: string): void
  (e: 'send'): void
  (e: 'stop'): void
}>()

const textarea = ref<HTMLTextAreaElement | null>(null)

const models = useModelsStore()
const { options: modelOptions, selected: selectedModel } = storeToRefs(models)

// Lazy-load the model list on mount. The picker uses fallback models
// (wu-tier list) until the API call resolves — feels instant.
onMounted(() => { if (!models.loaded) void models.fetchList() })

const canSend = computed(() =>
  !props.disabled && !props.streaming && props.modelValue.trim().length > 0,
)

function onInput(e: Event) {
  const el = e.target as HTMLTextAreaElement
  emit('update:modelValue', el.value)
  autosize()
}

function autosize() {
  const el = textarea.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = Math.min(el.scrollHeight, 240) + 'px'
}

watch(() => props.modelValue, () => nextTick(autosize))

function onKeydown(e: KeyboardEvent) {
  if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
    e.preventDefault()
    if (canSend.value) emit('send')
    return
  }
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    if (canSend.value) emit('send')
  }
}
</script>

<template>
  <div class="nest-input-wrap">
    <textarea
      ref="textarea"
      class="nest-input"
      :value="modelValue"
      :placeholder="placeholder ?? 'Write a message… (Enter to send, Shift+Enter for newline)'"
      :disabled="disabled"
      rows="1"
      @input="onInput"
      @keydown="onKeydown"
    />
    <div class="nest-input-actions">
      <!-- Model picker: free-tier & paid aliases merged into one list.
           Selected value is persisted in localStorage via the models store. -->
      <v-menu location="top start" offset="8">
        <template #activator="{ props: menuProps }">
          <button class="nest-model-btn" v-bind="menuProps" type="button">
            <v-icon size="14" class="mr-1">mdi-brain</v-icon>
            <span class="nest-mono">{{ selectedModel }}</span>
            <v-icon size="14" class="ml-1">mdi-chevron-up</v-icon>
          </button>
        </template>
        <v-list density="compact" class="nest-model-list">
          <v-list-item
            v-for="m in modelOptions"
            :key="m.id"
            :active="m.id === selectedModel"
            @click="models.select(m.id)"
          >
            <v-list-item-title class="nest-mono">{{ m.id }}</v-list-item-title>
          </v-list-item>
        </v-list>
      </v-menu>

      <div class="flex-grow-1" />

      <v-btn
        v-if="streaming"
        color="error"
        variant="tonal"
        size="small"
        prepend-icon="mdi-stop-circle-outline"
        @click="emit('stop')"
      >
        Stop
      </v-btn>
      <v-btn
        v-else
        color="primary"
        variant="flat"
        size="small"
        :disabled="!canSend"
        append-icon="mdi-send"
        @click="emit('send')"
      >
        Send
      </v-btn>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.nest-input-wrap {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
  border: 1px solid var(--nest-border);
  border-radius: var(--nest-radius);
  background: var(--nest-surface);
  transition: border-color var(--nest-transition-fast);

  &:focus-within {
    border-color: var(--nest-accent);
  }
}

.nest-input {
  width: 100%;
  resize: none;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--nest-text);
  font: 15px/1.5 var(--nest-font-body);
  max-height: 240px;
  overflow-y: auto;

  &::placeholder { color: var(--nest-text-muted); }
  &:disabled { opacity: 0.5; cursor: not-allowed; }
}

.nest-input-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.nest-model-btn {
  display: inline-flex;
  align-items: center;
  padding: 4px 10px;
  font-size: 11.5px;
  background: var(--nest-bg-elevated);
  color: var(--nest-text-secondary);
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-pill);
  cursor: pointer;
  transition: border-color var(--nest-transition-fast), color var(--nest-transition-fast);

  &:hover {
    border-color: var(--nest-accent);
    color: var(--nest-text);
  }
}

.nest-model-list {
  background: var(--nest-surface) !important;
  border: 1px solid var(--nest-border);
  max-height: 320px;
  min-width: 200px;
}
</style>
