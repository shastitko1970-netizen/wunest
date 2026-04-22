<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useCharactersStore } from '@/stores/characters'
import { useChatsStore } from '@/stores/chats'
import type { Character } from '@/api/characters'
import CharacterCard from '@/components/CharacterCard.vue'
import ImportCharacterDialog from '@/components/ImportCharacterDialog.vue'
import NewCharacterDialog from '@/components/NewCharacterDialog.vue'
import BrowseLibraryDialog from '@/components/BrowseLibraryDialog.vue'
import WorldsPanel from '@/components/WorldsPanel.vue'
import CharacterWorldsDialog from '@/components/CharacterWorldsDialog.vue'

const { t } = useI18n()
const store = useCharactersStore()
const chats = useChatsStore()
const router = useRouter()
const { filtered, loading, error, allTags, query, activeTag, favoriteOnly } = storeToRefs(store)

const activeTab = ref<'characters' | 'worlds' | 'presets' | 'personas'>('characters')
const importOpen = ref(false)
const createOpen = ref(false)
const browseOpen = ref(false)
const confirmDeleteId = ref<string | null>(null)
const worldsDialogOpen = ref(false)
const worldsDialogChar = ref<Character | null>(null)

function onAttachWorlds(c: Character) {
  worldsDialogChar.value = c
  worldsDialogOpen.value = true
}

onMounted(() => {
  store.fetchAll()
})

function onOpen(c: Character) {
  // TODO: navigate to /library/character/:id detail page in a later PR.
  // For now — noop-log until the detail view exists.
  console.debug('open character', c.id)
}

async function onChat(c: Character) {
  try {
    const chat = await chats.createForCharacter(c.id)
    router.push(`/chat/${chat.id}`)
  } catch (e) {
    console.error('create chat failed', e)
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
          {{ t('library.actions.importPng') }}
        </v-btn>
        <v-btn
          variant="outlined"
          prepend-icon="mdi-plus"
          @click="createOpen = true"
        >
          {{ t('library.actions.new') }}
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
      <v-tab value="characters">{{ t('library.tabs.characters') }}</v-tab>
      <v-tab value="worlds">{{ t('library.tabs.worlds') }}</v-tab>
      <v-tab value="presets" disabled>{{ t('library.tabs.presets') }}</v-tab>
      <v-tab value="personas" disabled>{{ t('library.tabs.personas') }}</v-tab>
    </v-tabs>

    <v-divider />

    <v-window v-model="activeTab" class="mt-4">
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
            style="max-width: 320px"
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
          <p class="nest-subtitle mt-2" style="max-width: 360px; margin: 0 auto">
            {{ t('library.empty.hint') }}
          </p>
          <div class="d-flex ga-2 mt-4 justify-center flex-wrap">
            <v-btn color="primary" variant="flat" prepend-icon="mdi-earth" @click="browseOpen = true">
              {{ t('library.actions.browse') }}
            </v-btn>
            <v-btn variant="outlined" prepend-icon="mdi-upload" @click="importOpen = true">
              {{ t('library.actions.importPng') }}
            </v-btn>
          </div>
        </div>
        <div v-else class="nest-grid">
          <CharacterCard
            v-for="c in filtered"
            :key="c.id"
            :character="c"
            @open="onOpen"
            @chat="onChat"
            @favorite="onFavorite"
            @delete="askDelete"
            @worlds="onAttachWorlds"
          />
        </div>
      </v-window-item>

      <v-window-item value="worlds">
        <WorldsPanel class="mt-3" />
      </v-window-item>
    </v-window>

    <!-- Import dialog -->
    <ImportCharacterDialog v-model="importOpen" />

    <!-- Create-from-scratch dialog -->
    <NewCharacterDialog v-model="createOpen" />

    <!-- CHUB browse dialog -->
    <BrowseLibraryDialog v-model="browseOpen" />

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

.nest-tagbar {
  display: flex;
  flex-wrap: wrap;
  margin-bottom: 20px;
}

.nest-grid {
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

@media (max-width: 600px) {
  .nest-library { padding: 20px 12px; }
  .nest-grid { grid-template-columns: repeat(2, 1fr); gap: 10px; }
}
</style>
