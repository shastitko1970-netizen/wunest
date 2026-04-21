<script setup lang="ts">
import { computed } from 'vue'
import type { Message } from '@/api/chats'

const props = defineProps<{
  message: Message
  characterName?: string
  userName?: string
  streaming?: boolean
}>()

defineEmits<{
  (e: 'delete', m: Message): void
}>()

const isUser = computed(() => props.message.role === 'user')
const displayName = computed(() => {
  if (isUser.value) return props.userName || 'You'
  return props.characterName || 'Assistant'
})
const timestamp = computed(() => {
  const d = new Date(props.message.created_at)
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
})

const hasError = computed(() => !!props.message.extras?.error)
const tokensInfo = computed(() => {
  const ex = props.message.extras
  if (!ex?.tokens_out) return null
  const ms = ex.latency_ms ?? 0
  return `${ex.tokens_out} tok · ${(ms / 1000).toFixed(1)}s · ${ex.model}`
})
</script>

<template>
  <div class="nest-msg" :class="{ 'is-user': isUser, 'is-streaming': streaming }">
    <div class="nest-msg-header">
      <span class="nest-msg-name">{{ displayName }}</span>
      <span class="nest-msg-time nest-mono">{{ timestamp }}</span>
    </div>
    <div class="nest-msg-body" :class="{ 'is-error': hasError }">
      <template v-if="hasError">
        <v-icon size="16" color="error" class="mr-1">mdi-alert-circle</v-icon>
        <span class="text-error">
          Generation failed: {{ message.extras?.error }}
        </span>
      </template>
      <template v-else-if="!message.content && streaming">
        <span class="nest-thinking">▍</span>
      </template>
      <template v-else>
        <div class="nest-msg-content">{{ message.content }}<span v-if="streaming" class="nest-cursor">▍</span></div>
      </template>
    </div>
    <div v-if="tokensInfo && !streaming" class="nest-msg-footer nest-mono">
      {{ tokensInfo }}
    </div>
  </div>
</template>

<style lang="scss" scoped>
.nest-msg {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 14px 16px;
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius);
  background: var(--nest-surface);
  max-width: 100%;

  &.is-user {
    background: var(--nest-bg-elevated);
    border-color: var(--nest-border);
  }
}

.nest-msg-header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
}

.nest-msg-name {
  font-family: var(--nest-font-display);
  font-size: 14px;
  font-weight: 500;
  letter-spacing: -0.01em;
  color: var(--nest-text);
}

.nest-msg-time {
  font-size: 10.5px;
  color: var(--nest-text-muted);
  letter-spacing: 0.05em;
}

.nest-msg-body {
  font-size: 15px;
  line-height: 1.55;
  color: var(--nest-text);
  white-space: pre-wrap;
  word-wrap: break-word;
}

.nest-msg-content { display: inline; }

.nest-cursor,
.nest-thinking {
  color: var(--nest-accent);
  animation: nest-blink 0.9s steps(2) infinite;
}

@keyframes nest-blink {
  0%,  50% { opacity: 1; }
  51%, 100% { opacity: 0; }
}

.nest-msg-footer {
  font-size: 10.5px;
  color: var(--nest-text-muted);
  letter-spacing: 0.03em;
  padding-top: 4px;
  border-top: 1px dashed var(--nest-border-subtle);
}

.is-error {
  color: var(--nest-accent);
}
</style>
