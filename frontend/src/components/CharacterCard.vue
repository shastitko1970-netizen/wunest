<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import type { Character } from '@/api/characters'
import { useChatsStore } from '@/stores/chats'

const { t } = useI18n()

const chats = useChatsStore()
const { list: chatsList } = storeToRefs(chats)

const props = defineProps<{
  character: Character
}>()

const emit = defineEmits<{
  (e: 'open', c: Character): void
  /** Primary action: if the character already has a chat, reopen the
   *  newest one; otherwise create a new chat. */
  (e: 'chat', c: Character): void
  /** Explicit "new chat" override from the dots menu — always creates. */
  (e: 'new-chat', c: Character): void
  (e: 'favorite', c: Character): void
  (e: 'delete', c: Character): void
  (e: 'worlds', c: Character): void
}>()

// True iff this character already owns at least one chat in the loaded
// list. When true, the primary button label changes to "Continue" and
// the dots menu exposes an explicit "New chat" option so users can
// still spin up a fresh thread when they want one.
const hasExistingChat = computed(() =>
  chatsList.value.some(c => c.character_id === props.character.id),
)

const initials = computed(() => {
  const name = props.character.name.trim()
  if (!name) return '?'
  return name
    .split(/\s+/)
    .slice(0, 2)
    .map(w => w[0]?.toUpperCase() ?? '')
    .join('')
})

const tagline = computed(() => {
  const d = props.character.data.description || props.character.data.scenario || ''
  if (!d) return ''
  return d.length > 140 ? d.slice(0, 140) + '…' : d
})
</script>

<template>
  <v-card
    class="nest-character-card"
    :class="{ 'is-favorite': character.favorite }"
    @click="emit('open', character)"
  >
    <!-- Left stripe (Dossier-inspired) -->
    <div class="nest-stripe"></div>

    <div class="nest-card-body">
      <div class="nest-card-top">
        <v-avatar
          :size="44"
          :color="character.avatar_url ? undefined : 'surface-variant'"
        >
          <img v-if="character.avatar_url" :src="character.avatar_url" :alt="character.name" />
          <span v-else class="text-body-2">{{ initials }}</span>
        </v-avatar>

        <button
          class="nest-fav-btn"
          :class="{ active: character.favorite }"
          :aria-label="character.favorite ? 'Unfavorite' : 'Favorite'"
          @click.stop="emit('favorite', character)"
        >
          <v-icon size="18">
            {{ character.favorite ? 'mdi-star' : 'mdi-star-outline' }}
          </v-icon>
        </button>
      </div>

      <div class="nest-card-name">{{ character.name }}</div>
      <div v-if="tagline" class="nest-card-tagline">{{ tagline }}</div>

      <div v-if="character.tags.length" class="nest-card-tags">
        <span
          v-for="tag in character.tags.slice(0, 4)"
          :key="tag"
          class="nest-tag"
        >
          {{ tag }}
        </span>
        <span v-if="character.tags.length > 4" class="nest-tag muted">
          +{{ character.tags.length - 4 }}
        </span>
      </div>

      <div class="nest-card-actions" @click.stop>
        <v-btn
          size="small"
          variant="flat"
          color="primary"
          :prepend-icon="hasExistingChat ? 'mdi-forum' : 'mdi-forum-plus-outline'"
          @click="emit('chat', character)"
        >
          {{ hasExistingChat ? t('library.card.continue') : t('library.card.chat') }}
        </v-btn>
        <v-btn
          size="small"
          variant="text"
          icon="mdi-dots-horizontal"
          density="comfortable"
        >
          <v-icon>mdi-dots-horizontal</v-icon>
          <v-menu activator="parent">
            <v-list density="compact">
              <!-- Explicit "new chat" — ALWAYS available. The primary
                   button smart-reuses an existing chat (continue); if
                   the user wants a fresh conversation regardless of
                   history, they reach for this menu item. Keeping it
                   unconditional means new users see it too, matching
                   the ST mental model where chat creation is distinct
                   from chat resuming. -->
              <v-list-item
                prepend-icon="mdi-forum-plus-outline"
                :title="t('library.card.newChat')"
                @click="emit('new-chat', character)"
              />
              <v-list-item
                prepend-icon="mdi-pencil"
                :title="t('common.edit')"
                @click="emit('open', character)"
              />
              <v-list-item
                prepend-icon="mdi-book-open-page-variant-outline"
                :title="t('library.card.lorebooks')"
                @click="emit('worlds', character)"
              />
              <v-list-item
                prepend-icon="mdi-delete-outline"
                :title="t('common.delete')"
                base-color="error"
                @click="emit('delete', character)"
              />
            </v-list>
          </v-menu>
        </v-btn>
      </div>
    </div>
  </v-card>
</template>

<style lang="scss" scoped>
.nest-character-card {
  position: relative;
  background: var(--nest-surface) !important;
  border: 1px solid var(--nest-border);
  border-radius: var(--nest-radius) !important;
  overflow: hidden;
  transition: border-color var(--nest-transition-base), transform var(--nest-transition-base);
  cursor: pointer;

  &:hover {
    border-color: var(--nest-accent);
    transform: translateY(-2px);
  }

  &.is-favorite .nest-stripe {
    background: var(--nest-gold);
  }
}

.nest-stripe {
  position: absolute;
  top: 0; left: 0; bottom: 0;
  width: 3px;
  background: var(--nest-border);
  transition: background var(--nest-transition-base);
}

.nest-card-body {
  padding: 16px 16px 12px 18px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-height: 180px;
}

.nest-card-top {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}

.nest-fav-btn {
  background: transparent;
  border: none;
  padding: 4px;
  border-radius: 6px;
  color: var(--nest-text-muted);
  cursor: pointer;
  transition: color var(--nest-transition-fast), background var(--nest-transition-fast);

  &:hover { background: var(--nest-bg-elevated); color: var(--nest-text); }
  &.active { color: var(--nest-gold); }
}

.nest-card-name {
  font-family: var(--nest-font-display);
  font-size: 18px;
  font-weight: 500;
  line-height: 1.2;
  letter-spacing: -0.01em;
  color: var(--nest-text);
}

.nest-card-tagline {
  font-size: 13px;
  line-height: 1.4;
  color: var(--nest-text-secondary);
  display: -webkit-box;
  -webkit-line-clamp: 3;
  line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.nest-card-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.nest-tag {
  font-family: var(--nest-font-mono);
  font-size: 10.5px;
  padding: 2px 6px;
  letter-spacing: 0.04em;
  text-transform: lowercase;
  color: var(--nest-text-secondary);
  background: var(--nest-bg-elevated);
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-pill);

  &.muted {
    color: var(--nest-text-muted);
  }
}

.nest-card-actions {
  display: flex;
  gap: 6px;
  margin-top: auto;
  justify-content: space-between;
  align-items: center;
}
</style>
