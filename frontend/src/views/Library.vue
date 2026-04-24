<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useCharactersStore } from '@/stores/characters'
import { useChatsStore } from '@/stores/chats'
import type { Character } from '@/api/characters'
import CharacterCard from '@/components/CharacterCard.vue'
import ImportCharacterDialog from '@/components/ImportCharacterDialog.vue'
import NewCharacterDialog from '@/components/NewCharacterDialog.vue'
import BrowseLibraryDialog from '@/components/BrowseLibraryDialog.vue'
import GroupChatSetupDialog from '@/components/GroupChatSetupDialog.vue'
import WorldsPanel from '@/components/WorldsPanel.vue'
import CharacterWorldsDialog from '@/components/CharacterWorldsDialog.vue'
import PersonasPanel from '@/components/PersonasPanel.vue'
import PresetsPanel from '@/components/PresetsPanel.vue'

const { t } = useI18n()
const store = useCharactersStore()
const chats = useChatsStore()
const router = useRouter()
const route = useRoute()
const { filtered, loading, error, allTags, query, activeTag, favoriteOnly } = storeToRefs(store)

// Tab selection is mirrored to ?tab=… so deep links (and the /presets
// legacy-redirect) can land on the right pane.
type Tab = 'characters' | 'worlds' | 'personas' | 'presets'
const ALL_TABS: Tab[] = ['characters', 'worlds', 'personas', 'presets']

function tabFromQuery(q: unknown): Tab {
  return typeof q === 'string' && (ALL_TABS as string[]).includes(q)
    ? (q as Tab)
    : 'characters'
}

const activeTab = ref<Tab>(tabFromQuery(route.query.tab))

watch(() => route.query.tab, (q) => { activeTab.value = tabFromQuery(q) })
watch(activeTab, (t) => {
  if (t === tabFromQuery(route.query.tab)) return
  router.replace({ query: { ...route.query, tab: t === 'characters' ? undefined : t } })
})
const importOpen = ref(false)
const createOpen = ref(false)
const browseOpen = ref(false)
const groupSetupOpen = ref(false)
const confirmDeleteId = ref<string | null>(null)
const worldsDialogOpen = ref(false)
// Edit-mode reuses NewCharacterDialog with a `character` prop. This ref
// holds the target row while the dialog is open. When null and
// createOpen=true → create; when non-null and editOpen=true → edit.
const editOpen = ref(false)
const editingCharacter = ref<Character | null>(null)
const worldsDialogChar = ref<Character | null>(null)

function onAttachWorlds(c: Character) {
  worldsDialogChar.value = c
  worldsDialogOpen.value = true
}

onMounted(() => {
  store.fetchAll()
  // Also fetch chats — the character cards use `chats.list` to decide
  // whether to show "Продолжить" (existing chat present) vs "Начать
  // чат" (fresh) and to enable the ⋯ menu's "Новый чат" shortcut.
  // Without this the button could label stale (say "Начать чат" when
  // the user in fact already has 10 chats with that character).
  chats.fetchList()
})

function onOpen(c: Character) {
  // "Open" from the card's ⋯ menu means "Edit the character" — the only
  // action users expect when clicking a pencil icon. We reuse the
  // New-Character dialog in edit mode (pass the full character).
  editingCharacter.value = c
  editOpen.value = true
}

async function onChat(c: Character) {
  // Reopen the newest existing chat for this character if any — users
  // reported piling up 10+ chats per character because "Chat" always
  // created new. Explicit "new chat" lives behind the card's dots menu.
  try {
    const existing = chats.existingForCharacter(c.id)
    if (existing) {
      router.push(`/chat/${existing.id}`)
      return
    }
    const chat = await chats.createForCharacter(c.id)
    router.push(`/chat/${chat.id}`)
  } catch (e) {
    console.error('create chat failed', e)
  }
}

async function onNewChat(c: Character) {
  try {
    const chat = await chats.createForCharacter(c.id)
    router.push(`/chat/${chat.id}`)
  } catch (e) {
    console.error('create new chat failed', e)
  }
}

async function onFavorite(c: Character) {
  await store.toggleFavorite(c)
}

function askDelete(c: Character) {
  confirmDeleteId.value = c.id
}

async function confirmDelete() {
  if (confirmDeleteId.value) {
    await store.remove(confirmDeleteId.value)
    confirmDeleteId.value = null
  }
}
</script>

<template>
  <v-container class="nest-library" fluid>
    <!-- Header row -->
    <div class="nest-page-head">
      <div>
        <div class="nest-eyebrow">{{ t('library.title') }}</div>
        <h1 class="nest-h1 mt-1">{{ t('library.headline') }}</h1>
      </div>
      <div class="d-flex ga-2 flex-wrap">
        <v-btn
          color="primary"
          variant="flat"
          prepend-icon="mdi-earth"
          @click="browseOpen = true"
        >
          {{ t('library.actions.browse') }}
        </v-btn>
        <v-btn
          variant="outlined"
          prepend-icon="mdi-upload"
          @click="importOpen = true"
        >
          {{ t('library.actions.import') }}
        </v-btn>
        <v-btn
          variant="outlined"
          prepend-icon="mdi-plus"
          @click="createOpen = true"
        >
          {{ t('library.actions.new') }}
        </v-btn>
        <v-btn
          variant="outlined"
          prepend-icon="mdi-account-multiple-plus-outline"
          @click="groupSetupOpen = true"
        >
          {{ t('library.actions.groupChat') }}
        </v-btn>
      </div>
    </div>

    <!-- Tab bar -->
    <v-tabs
      v-model="activeTab"
      color="primary"
      density="compact"
      class="mt-6"
      :grow="false"
    >
      <!-- All "things in your library" live as tabs here: content you create
           or import. Generation templates (Пресеты) were a standalone page
           before; consolidating here per user request keeps one roof over
           characters/lorebooks/personas/templates. -->
      <v-tab value="characters">{{ t('library.tabs.characters') }}</v-tab>
      <v-tab value="worlds">{{ t('library.tabs.worlds') }}</v-tab>
      <v-tab value="personas">{{ t('library.tabs.personas') }}</v-tab>
      <v-tab value="presets">{{ t('library.tabs.presets') }}</v-tab>
    </v-tabs>

    <v-divider />

    <!-- :touch="false" — tester reported that horizontal swipes inside
         tab content (e.g. panning a carousel or just scrolling) were
         flipping the whole tab on mobile. Tap-only now. -->
    <v-window v-model="activeTab" class="mt-4" :touch="false">
      <v-window-item value="characters">
        <!-- Filter bar -->
        <div class="nest-filterbar">
          <v-text-field
            v-model="query"
            :placeholder="t('library.search')"
            prepend-inner-icon="mdi-magnify"
            hide-details
            density="compact"
            single-line
            clearable
            class="nest-filterbar-search"
          />
          <v-btn
            :variant="favoriteOnly ? 'tonal' : 'text'"
            :color="favoriteOnly ? 'secondary' : undefined"
            size="small"
            :prepend-icon="favoriteOnly ? 'mdi-star' : 'mdi-star-outline'"
            @click="favoriteOnly = !favoriteOnly"
          >
            {{ t('library.favorites') }}
          </v-btn>
        </div>

        <!-- Tag chips -->
        <div v-if="allTags.length" class="nest-tagbar">
          <v-chip
            :variant="activeTag === null ? 'tonal' : 'outlined'"
            :color="activeTag === null ? 'primary' : undefined"
            size="small"
            class="mr-1 mb-1"
            @click="activeTag = null"
          >
            {{ t('library.all') }}
          </v-chip>
          <v-chip
            v-for="t in allTags.slice(0, 20)"
            :key="t.tag"
            :variant="activeTag === t.tag ? 'tonal' : 'outlined'"
            :color="activeTag === t.tag ? 'primary' : undefined"
            size="small"
            class="mr-1 mb-1"
            @click="activeTag = activeTag === t.tag ? null : t.tag"
          >
            {{ t.tag }}
            <span class="nest-mono ml-1 text-medium-emphasis">{{ t.count }}</span>
          </v-chip>
        </div>

        <!-- Content -->
        <div v-if="loading" class="nest-state">
          <v-progress-circular indeterminate color="primary" size="28" />
        </div>
        <div v-else-if="error" class="nest-state">
          <v-alert type="error" variant="tonal">{{ error }}</v-alert>
        </div>
        <div v-else-if="filtered.length === 0" class="nest-state text-center">
          <v-icon size="48" color="surface-variant">mdi-bookshelf</v-icon>
          <div class="nest-h2 mt-4">{{ t('library.empty.title') }}</div>
          <p class="nest-subtitle nest-empty-hint mt-2">
            {{ t('library.empty.hint') }}
          </p>
          <div class="d-flex ga-2 mt-4 justify-center flex-wrap">
            <v-btn color="primary" variant="flat" prepend-icon="mdi-earth" @click="browseOpen = true">
              {{ t('library.actions.browse') }}
            </v-btn>
            <v-btn variant="outlined" prepend-icon="mdi-upload" @click="importOpen = true">
              {{ t('library.actions.import') }}
            </v-btn>
          </div>
        </div>
        <div v-else class="nest-card-grid">
          <CharacterCard
            v-for="c in filtered"
            :key="c.id"
            :character="c"
            @open="onOpen"
            @chat="onChat"
            @new-chat="onNewChat"
            @favorite="onFavorite"
            @delete="askDelete"
            @worlds="onAttachWorlds"
          />
        </div>
      </v-window-item>

      <v-window-item value="worlds">
        <WorldsPanel class="mt-3" />
      </v-window-item>

      <v-window-item value="personas">
        <PersonasPanel class="mt-3" />
      </v-window-item>

      <v-window-item value="presets">
        <PresetsPanel class="mt-3" />
      </v-window-item>
    </v-window>

    <!-- Import dialog -->
    <ImportCharacterDialog v-model="importOpen" />

    <!-- Create-from-scratch dialog -->
    <NewCharacterDialog v-model="createOpen" />
    <!-- Same dialog in edit mode — `character` prop triggers hydration
         from the row + save writes PATCH instead of POST. Closing
         (via @update:modelValue false) clears the ref so a subsequent
         createOpen doesn't accidentally stay in edit mode. -->
    <NewCharacterDialog
      v-model="editOpen"
      :character="editingCharacter"
      @update:model-value="v => !v && (editingCharacter = null)"
    />

    <!-- CHUB browse dialog -->
    <BrowseLibraryDialog v-model="browseOpen" />

    <!-- Group chat setup: multi-select, 2..6 participants, name. -->
    <GroupChatSetupDialog
      v-model="groupSetupOpen"
      @created="(id) => router.push(`/chat/${id}`)"
    />

    <!-- Per-character lorebook attachment -->
    <CharacterWorldsDialog
      v-model="worldsDialogOpen"
      :character="worldsDialogChar"
    />

    <!-- Delete confirmation -->
    <v-dialog :model-value="confirmDeleteId !== null" max-width="360" @update:model-value="v => !v && (confirmDeleteId = null)">
      <v-card class="nest-confirm">
        <v-card-title>{{ t('library.delete.title') }}</v-card-title>
        <v-card-text>{{ t('library.delete.body') }}</v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="confirmDeleteId = null">{{ t('common.cancel') }}</v-btn>
          <v-btn color="error" variant="flat" @click="confirmDelete">{{ t('common.delete') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </v-container>
</template>

<style lang="scss" scoped>
.nest-library {
  max-width: 1200px;
  padding: 32px 24px;
}

.nest-page-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  flex-wrap: wrap;
  gap: 16px;
}

.nest-filterbar {
  display: flex;
  gap: 12px;
  align-items: center;
  padding: 16px 0;
  flex-wrap: wrap;
}
// Search field width — was inline style; lifted into class so mod
// authors can .nest-filterbar-search { max-width: ... } without
// needing !important.
.nest-filterbar-search { max-width: 320px; }

// Empty-state caption centred + capped so long copy doesn't span the
// full page width. Was inline style; moved here per DS contract.
.nest-empty-hint {
  max-width: 360px;
  margin: 0 auto;
}

.nest-tagbar {
  display: flex;
  flex-wrap: wrap;
  margin-bottom: 20px;
}

// .nest-card-grid is distinct from the DS-contract .nest-grid (which is a
// strict two-column that collapses to one at 640px). Here we want
// adaptive cards — 3+ columns on desktop, down to two on narrow phones.
// Different shape → different name so they don't drift into each other.
.nest-card-grid {
  display: grid;
  gap: 16px;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
}

.nest-state {
  padding: 80px 24px;
  display: grid;
  place-items: center;
  color: var(--nest-text-muted);
}

.nest-confirm {
  background: var(--nest-surface) !important;
  border: 1px solid var(--nest-border);
}

// DS-canonical 640px. Below this the 3+ col card grid collapses to two.
@media (max-width: 640px) {
  .nest-library { padding: 20px 12px; }
  .nest-card-grid { grid-template-columns: repeat(2, 1fr); gap: 10px; }
}
</style>
