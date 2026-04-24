<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useChatsStore } from '@/stores/chats'
import { chatsApi } from '@/api/chats'
import type { Chat } from '@/api/chats'

const { t } = useI18n()
const router = useRouter()
const store = useChatsStore()
const { list, currentId, listLoading, listError } = storeToRefs(store)

const importInput = ref<HTMLInputElement | null>(null)
const importError = ref<string | null>(null)
const importNotice = ref<string | null>(null)

async function onImportPick(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0] ?? null
  if (importInput.value) importInput.value.value = ''
  if (!f) return
  importError.value = null
  importNotice.value = null
  try {
    const report = await chatsApi.importJsonl(f)
    await store.fetchList()
    // If the server skipped rows, show a one-liner above the list so the
    // user knows the chat is partial and can check the file. Success-path
    // (zero skipped) goes silent — the navigation itself is feedback.
    if (report.skipped > 0) {
      const details = report.skipped_details
        .map(d => `#${d.line}: ${d.reason}`)
        .join('; ')
      const overflow = report.skipped_overflow > 0
        ? ` (+${report.skipped_overflow} more)`
        : ''
      importNotice.value = `Imported ${report.imported} of ${report.total_data_lines}. ${report.skipped} skipped — ${details}${overflow}`
    }
    router.push(`/chat/${report.chat.id}`)
  } catch (err) {
    importError.value = (err as Error).message
  }
}

const GROUP_KEYS = {
  today: 'chat.list.groupToday',
  yesterday: 'chat.list.groupYesterday',
  week: 'chat.list.groupWeek',
  older: 'chat.list.groupOlder',
} as const
type GroupKey = keyof typeof GROUP_KEYS

// Simple tag filter — single-tag toggle, click a chip to activate,
// click again to clear. Power users can filter by multiple tags via
// the search dialog (M37) with syntax like `tag:RP tag:dragon`.
const activeTagFilter = ref<string>('')
function toggleTagFilter(tag: string) {
  activeTagFilter.value = activeTagFilter.value === tag ? '' : tag
}

const filteredList = computed(() => {
  if (!activeTagFilter.value) return list.value
  return list.value.filter(c => (c.tags ?? []).includes(activeTagFilter.value))
})

const grouped = computed(() => groupByDay(filteredList.value))

function select(c: Chat) {
  router.push(`/chat/${c.id}`)
}

// Two-stage delete: first click stages the chat in `pendingDelete`, which
// opens a confirm dialog. Guards against a stray click wiping out a chat
// the user spent hours on. Earlier builds deleted immediately on icon
// click — the TODO for this confirm had been open since M4 and a tester
// with a 5000-message RP chat understandably wanted it sooner.
const pendingDelete = ref<Chat | null>(null)

function stageDelete(c: Chat, ev: Event) {
  ev.stopPropagation()
  pendingDelete.value = c
}

async function confirmDelete() {
  const c = pendingDelete.value
  pendingDelete.value = null
  if (!c) return
  await store.remove(c.id)
  if (currentId.value === c.id) router.push('/chat')
}
</script>

<template>
  <div class="nest-chatlist">
    <div class="nest-chatlist-header">
      <span class="nest-eyebrow">{{ t('chat.list.title') }}</span>
      <div class="d-flex ga-1 align-center">
        <v-btn
          size="x-small"
          variant="text"
          icon="mdi-upload"
          :title="t('chat.import.btn')"
          @click="importInput?.click()"
        />
        <input
          ref="importInput"
          type="file"
          accept="application/x-ndjson,application/jsonl,.jsonl,.json,text/plain"
          hidden
          @change="onImportPick"
        />
        <v-btn
          size="x-small"
          variant="text"
          icon="mdi-plus"
          :title="t('chat.list.browse')"
          @click="router.push('/library')"
        />
      </div>
    </div>

    <v-alert
      v-if="importError"
      type="error"
      variant="tonal"
      density="compact"
      closable
      class="ma-2"
      @click:close="importError = null"
    >
      {{ importError }}
    </v-alert>
    <v-alert
      v-if="importNotice"
      type="warning"
      variant="tonal"
      density="compact"
      closable
      class="ma-2"
      @click:close="importNotice = null"
    >
      {{ importNotice }}
    </v-alert>

    <div v-if="listLoading" class="nest-state">
      <v-progress-circular indeterminate size="20" />
    </div>
    <div v-else-if="listError" class="nest-state text-error">{{ listError }}</div>
    <div v-else-if="list.length === 0" class="nest-empty">
      <p class="text-medium-emphasis mb-2">{{ t('chat.list.empty') }}</p>
      <v-btn
        size="small"
        variant="tonal"
        color="primary"
        prepend-icon="mdi-bookshelf"
        @click="router.push('/library')"
      >
        {{ t('chat.list.browse') }}
      </v-btn>
    </div>

    <template v-else>
      <div v-for="(group, key) in grouped" :key="key" class="nest-group">
        <div class="nest-group-label nest-mono">{{ t(GROUP_KEYS[key as GroupKey]) }}</div>
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
          <!-- Tag chips inline — clipped to 3 for density, rest in a
               "+N" pill. Click any tag → activate filter. -->
          <div v-if="(c.tags ?? []).length" class="nest-chatitem-tags">
            <span
              v-for="tag in (c.tags ?? []).slice(0, 3)"
              :key="tag"
              class="nest-chatitem-tag"
              :class="{ active: tag === activeTagFilter }"
              @click.stop="toggleTagFilter(tag)"
            >{{ tag }}</span>
            <span v-if="(c.tags ?? []).length > 3" class="nest-chatitem-tag muted">
              +{{ (c.tags ?? []).length - 3 }}
            </span>
          </div>
          <button
            class="nest-chatitem-del"
            :aria-label="`Delete chat ${c.name}`"
            @click="(ev) => stageDelete(c, ev)"
          >
            <v-icon size="14">mdi-close</v-icon>
          </button>
        </div>
      </div>
    </template>

    <!-- Confirm-delete dialog. Message-count + chat name surface so a user
         who's drunk/tired (their words) has one last chance to bail on
         wiping a long chat. Cancel is the default-focused action. -->
    <v-dialog
      :model-value="pendingDelete !== null"
      max-width="420"
      @update:model-value="v => !v && (pendingDelete = null)"
    >
      <v-card class="nest-confirm">
        <v-card-title>{{ t('chat.list.deleteConfirm.title') }}</v-card-title>
        <v-card-text>
          {{ t('chat.list.deleteConfirm.body', { name: pendingDelete?.name || '' }) }}
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="pendingDelete = null">
            {{ t('common.cancel') }}
          </v-btn>
          <v-btn color="error" variant="flat" @click="confirmDelete">
            {{ t('common.delete') }}
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<script lang="ts">
// Returns chats bucketed by day-range, keyed by a stable identifier the
// template translates via t(GROUP_KEYS[key]). Empty groups are omitted.
function groupByDay(chats: Chat[]): Record<'today' | 'yesterday' | 'week' | 'older', Chat[]> {
  const today = new Date(); today.setHours(0, 0, 0, 0)
  const yesterday = new Date(today); yesterday.setDate(today.getDate() - 1)
  const weekAgo = new Date(today); weekAgo.setDate(today.getDate() - 7)

  const groups: Record<string, Chat[]> = {}
  for (const c of chats) {
    const ts = new Date(c.last_message_at ?? c.updated_at)
    let key: string
    if (ts >= today) key = 'today'
    else if (ts >= yesterday) key = 'yesterday'
    else if (ts >= weekAgo) key = 'week'
    else key = 'older'
    if (!groups[key]) groups[key] = []
    groups[key].push(c)
  }
  return groups as Record<'today' | 'yesterday' | 'week' | 'older', Chat[]>
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
.nest-chatitem-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 4px;
}
.nest-chatitem-tag {
  display: inline-flex;
  align-items: center;
  padding: 1px 6px;
  font-size: 10px;
  line-height: 1.4;
  background: var(--nest-bg-elevated);
  color: var(--nest-text-secondary);
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-pill);
  cursor: pointer;
  transition: border-color var(--nest-transition-fast);

  &:hover:not(.muted) { border-color: var(--nest-accent); }
  &.active {
    color: var(--nest-text-on-accent, #fff);
    background: var(--nest-accent);
    border-color: var(--nest-accent);
  }
  &.muted { opacity: 0.7; cursor: default; }
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

// Touch-first: on devices without hover the delete ✕ stays visible
// permanently. Without this rule, tapping on a chat-item on mobile
// would never expose the delete button — you'd have to long-press to
// find some other escape. DS adaptive rule: "actions always visible
// on touch".
@media (hover: none) {
  .nest-chatitem-del { opacity: 0.6; }
}
</style>
