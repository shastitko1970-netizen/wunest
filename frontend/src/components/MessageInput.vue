<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'

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
  // Cmd/Ctrl+Enter = send regardless of shift.
  if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
    e.preventDefault()
    if (canSend.value) emit('send')
    return
  }
  // Enter (without shift) = send; Shift+Enter = newline (default behaviour).
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

  &::placeholder {
    color: var(--nest-text-muted);
  }
  &:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
}

.nest-input-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
