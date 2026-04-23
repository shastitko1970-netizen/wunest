<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useChatsStore } from '@/stores/chats'
import { useAuthStore } from '@/stores/auth'
import { useModelsStore } from '@/stores/models'
import type { Message } from '@/api/chats'
import ChatList from '@/components/ChatList.vue'
import MessageBubble from '@/components/MessageBubble.vue'
import MessageInput from '@/components/MessageInput.vue'
import GenerationSettings from '@/components/GenerationSettings.vue'
import PersonaPickerDialog from '@/components/PersonaPickerDialog.vue'
import BYOKPickerDialog from '@/components/BYOKPickerDialog.vue'
import { usePersonasStore } from '@/stores/personas'
import { countTokensMany } from '@/lib/tokens'  // sync approximation
import { chatsApi } from '@/api/chats'

const { t } = useI18n()

const route = useRoute()
const router = useRouter()
const chats = useChatsStore()
const auth = useAuthStore()
const models = useModelsStore()
const { currentChat, messages, messagesLoading, streaming, streamError } = storeToRefs(chats)
const { profile } = storeToRefs(auth)
const { selected: selectedModel } = storeToRefs(models)

const draft = ref('')
const scroller = ref<HTMLElement | null>(null)
const settingsOpen = ref(false)
const personaPickerOpen = ref(false)
const byokPickerOpen = ref(false)

// Tiny derived flag: chat has a BYOK pin in its metadata. Used to tint
// the header icon so at a glance the user knows a personal key is in
// flight for this chat.
const hasBYOKPin = computed(() => {
  const id = currentChat.value?.chat_metadata?.byok_id
  return typeof id === 'string' && id.length > 0
})

const personas = usePersonasStore()
onMounted(() => personas.fetchAll())

// Resolved "playing as" label for the chat header chip.
const activePersonaLabel = computed(() => {
  const chat = currentChat.value
  const overrideId = chat?.chat_metadata?.persona_id ?? null
  if (overrideId) {
    const p = personas.items.find(x => x.id === overrideId)
    if (p) return p.name
  }
  if (personas.defaultPersona) return personas.defaultPersona.name
  return profile.value?.first_name || profile.value?.username || ''
})

// Rolling estimate of "what would go in the next prompt" — the full message
// history content. Sync approximation, no debounce needed.
const contextTokens = computed(() =>
  countTokensMany((messages.value ?? []).map(m => m.content ?? '')),
)

onMounted(async () => {
  await chats.fetchList()
  await maybeLoadFromRoute()
})

watch(() => route.params.id, () => maybeLoadFromRoute())

// Auto-scroll to bottom when new messages arrive or tokens stream in.
watch([messages, streaming], () => {
  nextTick(() => {
    const el = scroller.value
    if (el) el.scrollTop = el.scrollHeight
  })
}, { deep: true })

async function maybeLoadFromRoute() {
  const id = route.params.id as string | undefined
  if (id) {
    await chats.open(id)
  }
}

const characterName = computed(() => currentChat.value?.character_name ?? undefined)
const userName = computed(() => profile.value?.first_name || profile.value?.username || 'You')
const hasSelection = computed(() => !!currentChat.value)

async function send() {
  const text = draft.value.trim()
  if (!text) return
  draft.value = ''
  await chats.send({ content: text, model: selectedModel.value })
}

async function regenerate(_m: Message) {
  await chats.regenerate({ model: selectedModel.value })
}

async function onSwipe(m: Message) {
  await chats.swipe(m, { model: selectedModel.value })
}

async function onSelectSwipe(m: Message, swipeID: number) {
  await chats.selectSwipe(m, swipeID)
}

async function exportCurrentChat() {
  const id = currentChat.value?.id
  if (!id) return
  try {
    const { blob, filename } = await chatsApi.exportJsonl(id)
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    a.click()
    URL.revokeObjectURL(url)
  } catch (e) {
    console.error('export failed', e)
  }
}

async function onEditMessage(m: Message, newContent: string) {
  try {
    await chats.editMessage(m, newContent)
  } catch (e) {
    console.error('edit failed', e)
  }
}

async function onDeleteMessage(m: Message) {
  try {
    await chats.deleteMessage(m)
  } catch (e) {
    console.error('delete failed', e)
  }
}

// Only the most recent assistant message can be regenerated in V1.
const lastAssistantId = computed(() => {
  for (let i = messages.value.length - 1; i >= 0; i--) {
    const m = messages.value[i]
    if (m && m.role === 'assistant') return m.id
  }
  return null
})
</script>

<template>
  <div class="nest-chat-layout">
    <!-- Sidebar with chat list -->
    <aside class="nest-chat-sidebar">
      <ChatList />
    </aside>

    <!-- Main chat panel -->
    <section class="nest-chat-main">
      <template v-if="!hasSelection">
        <div class="nest-chat-empty">
          <v-icon size="56" color="surface-variant">mdi-forum-outline</v-icon>
          <h2 class="nest-h2 mt-4">{{ t('chat.empty.title') }}</h2>
          <p class="nest-subtitle mt-2">{{ t('chat.empty.hint') }}</p>
          <v-btn
            class="mt-4"
            color="primary"
            variant="flat"
            prepend-icon="mdi-bookshelf"
            @click="router.push('/library')"
          >
            {{ t('chat.empty.openLibrary') }}
          </v-btn>
        </div>
      </template>

      <template v-else>
        <!-- Header -->
        <header class="nest-chat-header">
          <div class="nest-chat-title">
            <div class="nest-chat-name">{{ currentChat!.name }}</div>
            <div v-if="characterName" class="nest-mono nest-chat-char">
              {{ t('chat.with', { name: characterName }) }}
            </div>
          </div>
          <div class="nest-chat-tools">
            <span
              v-if="contextTokens > 0"
              class="nest-mono nest-ctx-chip"
              :title="t('chat.contextTokensTitle')"
            >
              {{ contextTokens }} {{ t('chat.input.tokensShort') }}
            </span>
            <v-btn
              variant="text"
              size="small"
              :title="t('personas.picker.title') + (activePersonaLabel ? ': ' + activePersonaLabel : '')"
              icon="mdi-drama-masks"
              @click="personaPickerOpen = true"
            />
            <v-btn
              variant="text"
              size="small"
              :color="hasBYOKPin ? 'primary' : undefined"
              :title="t('byok.picker.title')"
              icon="mdi-key-variant"
              @click="byokPickerOpen = true"
            />
            <v-btn
              variant="text"
              size="small"
              :title="t('chat.export.btn')"
              icon="mdi-download-outline"
              @click="exportCurrentChat"
            />
            <v-btn
              variant="text"
              size="small"
              :title="t('chat.sampler.title')"
              icon="mdi-tune-variant"
              @click="settingsOpen = true"
            />
          </div>
        </header>

        <!-- Scrollable messages.
             ST-compat: `#chat` is the canonical SillyTavern container ID
             users target in custom CSS (e.g. `#chat { background: ... }`).
             Adding it as an alias makes ST themes Just Work™ for the chat
             surface. The React-equivalent conflict (one id per doc) is
             fine: there's only ever one open chat at a time. -->
        <div ref="scroller" class="nest-chat-scroll" id="chat">
          <div class="nest-chat-messages">
            <div v-if="messagesLoading" class="nest-state">
              <v-progress-circular indeterminate size="24" />
            </div>
            <template v-else-if="messages.length === 0">
              <div class="nest-chat-firstturn">
                <span class="nest-mono text-medium-emphasis">{{ t('chat.sayHi') }}</span>
              </div>
            </template>
            <template v-else>
              <MessageBubble
                v-for="(m, i) in messages"
                :key="m.id"
                :message="m"
                :character-name="characterName"
                :user-name="userName"
                :streaming="streaming && i === messages.length - 1 && m.role === 'assistant'"
                :allow-regenerate="!streaming && m.role === 'assistant' && m.id === lastAssistantId"
                @regenerate="regenerate"
                @swipe="onSwipe"
                @select-swipe="onSelectSwipe"
                @edit="onEditMessage"
                @delete="onDeleteMessage"
              />
            </template>
            <v-alert
              v-if="streamError"
              type="error"
              variant="tonal"
              density="compact"
              class="mt-2"
            >
              {{ streamError }}
            </v-alert>
          </div>
        </div>

        <!-- Input. ST-compat id `send_form` so ST CSS targeting the
             composer area (e.g. `#send_form { background: ... }`) hits. -->
        <div class="nest-chat-input" id="send_form">
          <MessageInput
            v-model="draft"
            :streaming="streaming"
            @send="send"
            @stop="chats.stopStreaming"
          />
        </div>
      </template>
    </section>

    <!-- Generation settings drawer — lazily mounts sampler form. -->
    <GenerationSettings v-model="settingsOpen" />

    <!-- Persona picker for the current chat. -->
    <PersonaPickerDialog
      v-model="personaPickerOpen"
      :chat="currentChat ?? null"
    />

    <!-- BYOK picker — per-chat override for the upstream provider key. -->
    <BYOKPickerDialog
      v-model="byokPickerOpen"
      :chat="currentChat ?? null"
    />
  </div>
</template>

<style lang="scss" scoped>
// Anchor the chat layout to viewport edges with position: fixed so we don't
// depend on v-main's height math. With the topbar always 56px tall and no
// desktop sidebar, this is viewport-independent and behaves identically in
// Firefox / Chrome / Safari / mobile browsers.
//
// Previous approach (height: calc(100vh - …) inside flex v-main) broke in
// Firefox because of strict flex min-size rules and on mobile because of
// URL-bar collapse changing 100vh mid-scroll.
.nest-chat-layout {
  position: fixed;
  top: var(--nest-header-height);
  left: 0;
  right: 0;
  bottom: 0;
  display: grid;
  grid-template-columns: 280px 1fr;
  grid-template-rows: 1fr;
  background: var(--nest-bg);
  overflow: hidden;
  min-height: 0;
}

.nest-chat-sidebar {
  border-right: 1px solid var(--nest-border);
  background: var(--nest-bg-elevated);
  overflow: hidden;
  min-height: 0;
  min-width: 0;
}

.nest-chat-main {
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  height: 100%;                  // anchor the flex container to the grid cell
}

.nest-chat-empty {
  flex: 1;
  min-height: 0;
  display: grid;
  place-items: center;
  padding: 40px;
  text-align: center;
  color: var(--nest-text-muted);
}

.nest-chat-header {
  flex: 0 0 auto;                // header always visible, doesn't shrink
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 20px;
  border-bottom: 1px solid var(--nest-border);
}
.nest-chat-title {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.nest-chat-tools {
  display: flex;
  align-items: center;
  gap: 4px;
}
.nest-ctx-chip {
  font-size: 11px;
  color: var(--nest-text-muted);
  padding: 2px 8px;
  border-radius: var(--nest-radius-pill);
  background: var(--nest-bg-elevated);
  border: 1px solid var(--nest-border-subtle);
  font-variant-numeric: tabular-nums;
}
.nest-chat-name {
  font-family: var(--nest-font-display);
  font-size: 18px;
  font-weight: 500;
  color: var(--nest-text);
}
.nest-chat-char {
  font-size: 11px;
  color: var(--nest-text-muted);
  letter-spacing: 0.04em;
}

.nest-chat-scroll {
  flex: 1 1 auto;
  min-height: 0;                 // Firefox refuses to shrink without this
  overflow-y: auto;
  overflow-x: hidden;
  overscroll-behavior: contain;  // scroll chain doesn't bubble to the shell
  scroll-behavior: smooth;
  -webkit-overflow-scrolling: touch;  // iOS momentum scroll
}

// chat-width (set by AppearancePanel) is a percent of the chat column —
// same semantic as SillyTavern's `chat_width` field. 100% uses the whole
// column; narrower values center a readable measure.
.nest-chat-messages {
  max-width: var(--nest-chat-width, 820px);
  width: 100%;
  margin: 0 auto;
  padding: 24px 20px 60px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.nest-chat-firstturn {
  padding: 40px;
  text-align: center;
}

.nest-chat-input {
  flex: 0 0 auto;                // pinned to the bottom of the flex column
  padding: 12px 20px max(16px, env(safe-area-inset-bottom));
  border-top: 1px solid var(--nest-border);
  background: var(--nest-bg);
  max-width: 820px;
  width: 100%;
  margin: 0 auto;
}

.nest-state { padding: 40px; display: grid; place-items: center; }

@media (max-width: 960px) {
  .nest-chat-layout {
    grid-template-columns: 1fr;
  }
  .nest-chat-sidebar { display: none; }
}

// Header real-estate on phones. At 375px we have: chat name, character
// caption, token chip, and four icons (persona, BYOK, export, sampler).
// That's too much. Shave hard:
//   - Tighter header padding (14→10px v, 20→12px h)
//   - Smaller chat name
//   - Hide the character caption (it's repeated in the message list anyway)
//   - Hide the token chip in the header — still visible in the composer
//   - No extra gap between icon buttons
@media (max-width: 520px) {
  .nest-chat-header { padding: 10px 12px; }
  .nest-chat-name   { font-size: 15px; }
  .nest-chat-char   { display: none; }
  .nest-ctx-chip    { display: none; }
  .nest-chat-tools  { gap: 0; }
  .nest-chat-tools .v-btn { --v-btn-size: 28px; }
  .nest-chat-messages { padding: 14px 12px 56px; }
  .nest-chat-input    { padding: 10px 12px max(14px, env(safe-area-inset-bottom)); }
}
</style>
