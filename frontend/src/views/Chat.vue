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
import { usePersonasStore } from '@/stores/personas'

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
              :title="t('chat.sampler.title')"
              icon="mdi-tune-variant"
              @click="settingsOpen = true"
            />
          </div>
        </header>

        <!-- Scrollable messages -->
        <div ref="scroller" class="nest-chat-scroll">
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

        <!-- Input -->
        <div class="nest-chat-input">
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
  </div>
</template>

<style lang="scss" scoped>
.nest-chat-layout {
  height: calc(100vh - var(--nest-header-height));
  display: grid;
  grid-template-columns: 280px 1fr;
  background: var(--nest-bg);
}

.nest-chat-sidebar {
  border-right: 1px solid var(--nest-border);
  background: var(--nest-bg-elevated);
  overflow: hidden;
}

.nest-chat-main {
  display: flex;
  flex-direction: column;
  min-width: 0; /* so children with overflow work inside grid */
}

.nest-chat-empty {
  flex: 1;
  display: grid;
  place-items: center;
  padding: 40px;
  text-align: center;
  color: var(--nest-text-muted);
}

.nest-chat-header {
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
  flex: 1;
  overflow-y: auto;
  scroll-behavior: smooth;
}

.nest-chat-messages {
  max-width: 820px;
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
  padding: 12px 20px 16px;
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
</style>
