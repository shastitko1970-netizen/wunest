<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { useChatsStore } from '@/stores/chats'
import type { Chat } from '@/api/chats'

const router = useRouter()
const store = useChatsStore()
const { list, currentId, listLoading, listError } = storeToRefs(store)

const grouped = computed(() => groupByDay(list.value))

function select(c: Chat) {
  router.push(`/chat/${c.id}`)
}
async function del(c: Chat, ev: Event) {
  ev.stopPropagation()
  // TODO: confirm dialog in M4
  await store.remove(c.id)
  if (currentId.value === c.id) router.push('/chat')
}
</script>

<template>
  <div class="nest-chatlist">
    <div class="nest-chatlist-header">
      <span class="nest-eyebrow">Chats</span>
      <v-btn
        size="x-small"
        variant="text"
        icon="mdi-plus"
        @click="router.push('/library')"
      />
    </div>

    <div v-if="listLoading" class="nest-state">
      <v-progress-circular indeterminate size="20" />
    </div>
    <div v-else-if="listError" class="nest-state text-error">{{ listError }}</div>
    <div v-else-if="list.length === 0" class="nest-empty">
      <p class="text-medium-emphasis mb-2">No chats yet.</p>
      <v-btn
        size="small"
        variant="tonal"
        color="primary"
        prepend-icon="mdi-bookshelf"
        @click="router.push('/library')"
      >
        Browse Library
      </v-btn>
    </div>

    <template v-else>
      <div v-for="(group, label) in grouped" :key="label" class="nest-group">
        <div class="nest-group-label nest-mono">{{ label }}</div>
        <div
          v-for="c in group"
          :key="c.id"
          class="nest-chatitem"
          :class="{ active: currentId === c.id }"
          @click="select(c)"
        >
          <div class="nest-chatitem-name">{{ c.name }}</div>
          <div class="nest-chatitem-meta nest-mono">
            <span v-if="c.character_name" class="nest-chatitem-char">{{ c.character_name }}</span>
          </div>
          <button
            class="nest-chatitem-del"
            :aria-label="`Delete chat ${c.name}`"
            @click="(ev) => del(c, ev)"
          >
            <v-icon size="14">mdi-close</v-icon>
          </button>
        </div>
      </div>
    </template>
  </div>
</template>

<script lang="ts">
function groupByDay(chats: Chat[]): Record<string, Chat[]> {
  const today = new Date(); today.setHours(0, 0, 0, 0)
  const yesterday = new Date(today); yesterday.setDate(today.getDate() - 1)
  const weekAgo = new Date(today); weekAgo.setDate(today.getDate() - 7)

  const groups: Record<string, Chat[]> = {}
  for (const c of chats) {
    const ts = new Date(c.last_message_at ?? c.updated_at)
    let label: string
    if (ts >= today) label = 'Today'
    else if (ts >= yesterday) label = 'Yesterday'
    else if (ts >= weekAgo) label = 'This week'
    else label = 'Older'
    if (!groups[label]) groups[label] = []
    groups[label].push(c)
  }
  return groups
}
</script>

<style lang="scss" scoped>
.nest-chatlist {
  height: 100%;
  display: flex;
  flex-direction: column;
  padding: 12px 8px;
  overflow-y: auto;
}
.nest-chatlist-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 8px 8px;
}

.nest-state { padding: 20px; text-align: center; }
.nest-empty { padding: 20px 8px; text-align: center; }

.nest-group { margin-bottom: 12px; }
.nest-group-label {
  padding: 8px 8px 4px;
  font-size: 10px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--nest-text-muted);
}

.nest-chatitem {
  position: relative;
  padding: 8px 10px;
  border-radius: var(--nest-radius-sm);
  cursor: pointer;
  transition: background var(--nest-transition-fast);

  &:hover {
    background: var(--nest-bg);
    .nest-chatitem-del { opacity: 1; }
  }
  &.active {
    background: var(--nest-surface);
    box-shadow: inset 2px 0 0 var(--nest-accent);
  }
}

.nest-chatitem-name {
  font-size: 13.5px;
  color: var(--nest-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  padding-right: 20px;
}
.nest-chatitem-meta {
  font-size: 10.5px;
  color: var(--nest-text-muted);
  letter-spacing: 0.02em;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.nest-chatitem-char { }

.nest-chatitem-del {
  position: absolute;
  top: 6px;
  right: 6px;
  border: 0;
  background: transparent;
  color: var(--nest-text-muted);
  opacity: 0;
  padding: 2px;
  border-radius: 4px;
  cursor: pointer;
  transition: opacity var(--nest-transition-fast), background var(--nest-transition-fast);

  &:hover { background: var(--nest-border); color: var(--nest-text); }
}
</style>
