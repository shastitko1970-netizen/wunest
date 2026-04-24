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
import { usePresetsStore } from '@/stores/presets'
import { countTokensMany } from '@/lib/tokens'  // sync approximation
import { chatsApi } from '@/api/chats'
import { useDisplay } from 'vuetify'

const { t } = useI18n()
const { mdAndDown } = useDisplay()

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
// Mobile-only chat-list drawer. Desktop sidebar is always visible; mobile
// hid the sidebar entirely, leaving users with no way to switch chats.
// Hamburger in the chat header toggles this overlay.
const chatListDrawerOpen = ref(false)

// Tiny derived flag: chat has a BYOK pin in its metadata. Used to tint
// the header icon so at a glance the user knows a personal key is in
// flight for this chat.
const hasBYOKPin = computed(() => {
  const id = currentChat.value?.chat_metadata?.byok_id
  return typeof id === 'string' && id.length > 0
})

const personas = usePersonasStore()
const presets = usePresetsStore()
onMounted(() => {
  personas.fetchAll()
  presets.fetchAll()
})

// Quick-switch chip for the chat header: shows the currently active
// sampler preset so the user can flip it without opening the drawer. Only
// rendered when the user has at least one sampler preset — for fresh
// accounts the chip would just say "none" and waste space.
const samplerChipLabel = computed<string | null>(() => {
  const active = presets.activePreset('sampler')
  if (active) return active.name
  if (presets.samplers.length === 0) return null  // hide the chip entirely
  return t('chat.preset.noneChip')
})

async function pickActiveSampler(id: string | null) {
  await presets.setActive('sampler', id)
}

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

watch(() => route.params.id, () => {
  maybeLoadFromRoute()
  // Close the mobile chat-list drawer on chat change so the new message
  // isn't occluded by a still-open list.
  chatListDrawerOpen.value = false
})

// Auto-scroll to bottom — but only when the user is already near the
// bottom. If they've scrolled up (e.g. to re-read earlier content
// while a long response streams), we leave their viewport alone.
// Once they scroll back to the bottom, auto-scroll re-engages
// automatically.
//
// `autoStickBottom` is the latch. A "new messages below" chip shows
// when autoStickBottom is false AND there's fresh content, so users
// who wandered up have a one-tap escape back.
const autoStickBottom = ref(true)
const hasNewBelow = ref(false)
// Pixels of slack for "near bottom". 160 covers small rounding errors
// + a message's bottom padding. Lower = stricter (needs exact bottom);
// higher = loose (re-engages even from a few cm up).
const STICK_THRESHOLD = 160

function isNearBottom(el: HTMLElement): boolean {
  return el.scrollHeight - el.scrollTop - el.clientHeight <= STICK_THRESHOLD
}

function onScroll() {
  const el = scroller.value
  if (!el) return
  if (isNearBottom(el)) {
    autoStickBottom.value = true
    hasNewBelow.value = false
  } else {
    autoStickBottom.value = false
  }
}

// On new tokens / message counts: scroll to bottom if we're stuck
// there, else surface a "new messages below" pill so the user knows
// content is landing off-screen.
watch([messages, streaming], () => {
  nextTick(() => {
    const el = scroller.value
    if (!el) return
    if (autoStickBottom.value) {
      el.scrollTop = el.scrollHeight
    } else {
      hasNewBelow.value = true
    }
  })
}, { deep: true })

// Explicit "jump to bottom" — used by the new-messages pill.
function jumpToBottom() {
  const el = scroller.value
  if (!el) return
  el.scrollTop = el.scrollHeight
  autoStickBottom.value = true
  hasNewBelow.value = false
}

// Reset stick state when the chat itself changes — opening a new
// chat should always land at the bottom.
watch(() => route.params.id, () => {
  autoStickBottom.value = true
  hasNewBelow.value = false
})

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

// ── Plate action bridge (M32 interactive plates) ─────────────────
// Author-supplied <button data-nest-action="say|send"> bubbles up a
// `plate-draft` here; we fill the composer with the text and optionally
// send it immediately. `plate-toast` surfaces a brief snackbar for
// confirmation-style actions (copy, dice roll results).
function onPlateDraft(text: string, shouldSend: boolean) {
  draft.value = text
  if (shouldSend) {
    // Defer a tick so the composer has the text painted before send
    // fires (matches the user-typed send flow).
    nextTick(() => { void send() })
  }
}

// Plate snackbar. Separate from streamError (that's a persistent
// inline alert for generation failures) — plate-toast is transient
// and bottom-centered.
const plateToast = ref<{ show: boolean; level: 'info' | 'success' | 'error'; text: string }>({
  show: false,
  level: 'info',
  text: '',
})
function onPlateToast(level: 'info' | 'success' | 'error', text: string) {
  plateToast.value = { show: true, level, text }
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
        <!-- On mobile the sidebar is hidden, so "no chat selected" must
             surface the chat list itself — otherwise users land on a
             blank page with no way to enter a chat from here. Desktop
             keeps the original hero card because the sidebar is already
             showing the list. -->
        <div v-if="mdAndDown" class="nest-chat-mobile-list">
          <ChatList />
        </div>
        <div v-else class="nest-chat-empty">
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
          <!-- Mobile burger opens the chat list drawer. Without this the
               chat list on phones is stranded behind the hidden sidebar
               (`display: none` on <=960px) and users can't switch chats. -->
          <v-btn
            v-if="mdAndDown"
            variant="text"
            size="small"
            icon="mdi-menu"
            class="nest-chat-menu-btn"
            :title="t('chat.list.title')"
            @click="chatListDrawerOpen = true"
          />
          <div class="nest-chat-title">
            <div class="nest-chat-name">{{ currentChat!.name }}</div>
            <div v-if="characterName" class="nest-mono nest-chat-char">
              {{ t('chat.with', { name: characterName }) }}
            </div>
          </div>
          <div class="nest-chat-tools">
            <!-- Active-sampler chip: instant preset switcher without
                 opening the settings drawer. Click → menu of all sampler
                 presets (+ "none" option). Hidden when user has no
                 sampler presets at all. -->
            <v-menu
              v-if="samplerChipLabel"
              location="bottom end"
              offset="4"
            >
              <template #activator="{ props: menuProps }">
                <button
                  v-bind="menuProps"
                  class="nest-preset-chip nest-mono"
                  :title="t('chat.preset.switchTitle')"
                >
                  <v-icon size="12" class="mr-1">mdi-tune-variant</v-icon>
                  {{ samplerChipLabel }}
                  <v-icon size="12" class="ml-1">mdi-menu-down</v-icon>
                </button>
              </template>
              <v-list density="compact" min-width="200">
                <v-list-item
                  v-for="p in presets.samplers"
                  :key="p.id"
                  :active="presets.isActive(p)"
                  @click="pickActiveSampler(p.id)"
                >
                  <v-list-item-title>{{ p.name }}</v-list-item-title>
                </v-list-item>
                <v-divider />
                <v-list-item
                  :active="!presets.activeID('sampler')"
                  @click="pickActiveSampler(null)"
                >
                  <v-list-item-title class="text-medium-emphasis">
                    {{ t('chat.preset.none') }}
                  </v-list-item-title>
                </v-list-item>
              </v-list>
            </v-menu>
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
             fine: there's only ever one open chat at a time.
             Passive scroll listener updates autoStickBottom so new
             streaming tokens don't drag us back down when the user
             scrolled up to re-read earlier content. -->
        <div ref="scroller" class="nest-chat-scroll" id="chat" @scroll.passive="onScroll">
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
                @plate-draft="onPlateDraft"
                @plate-toast="onPlateToast"
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

        <!-- Jump-to-bottom pill. Only shown when the user has scrolled
             up AND new content has landed below — removes the "why
             isn't it scrolling?!" panic without forcibly yanking the
             viewport during a long stream the user is reading above. -->
        <transition name="nest-fade">
          <button
            v-if="hasNewBelow && !autoStickBottom"
            class="nest-jump-bottom"
            type="button"
            :title="t('chat.jumpToBottom')"
            @click="jumpToBottom"
          >
            <v-icon size="16">mdi-arrow-down</v-icon>
            <span>{{ t('chat.jumpToBottom') }}</span>
          </button>
        </transition>

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

    <!-- Plate-action confirmation toast. Fires for copy/dice and other
         transient feedback from author-supplied <button data-nest-action>
         clicks. Separate from the streamError banner which is for
         generation failures. -->
    <v-snackbar
      v-model="plateToast.show"
      :color="plateToast.level"
      :timeout="2400"
      location="bottom"
    >
      {{ plateToast.text }}
    </v-snackbar>

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

    <!-- Mobile-only chat list drawer. Triggered by the header hamburger.
         Scoped to Chat.vue because desktop already shows the list in the
         sidebar. Auto-closes on route change (i.e. when the user picks
         a chat from it) so we don't stack overlays. -->
    <v-navigation-drawer
      v-if="mdAndDown"
      v-model="chatListDrawerOpen"
      temporary
      location="left"
      width="320"
      class="nest-mobile-chatlist-drawer"
    >
      <ChatList />
    </v-navigation-drawer>
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
  position: relative;             // anchor for .nest-jump-bottom pill
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
  // Let the title column shrink when tools get crowded; without this the
  // flex parent keeps the title at its natural width and pushes the
  // tools right edge off-screen on narrow phones.
  min-width: 0;
  flex: 1 1 auto;
  overflow: hidden;

  .nest-chat-name,
  .nest-chat-char {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}
.nest-chat-tools {
  display: flex;
  align-items: center;
  gap: 4px;
  // Cap width so buttons don't shove the chat title off-screen on narrow
  // viewports, and allow horizontal scroll as a fallback when the sum of
  // tools still exceeds that cap. Without min-width:0 the flexbox parent
  // refuses to shrink this child, leaving the topbar uneditable on some
  // Android keyboards.
  min-width: 0;
  max-width: 100%;
  overflow-x: auto;
  overflow-y: hidden;
  scrollbar-width: none;
  &::-webkit-scrollbar { display: none; }
}
// Active-preset chip in the chat header. Visually kin to the nav-chip
// pattern used elsewhere — subtle outline, gets a primary-accent border
// on hover so it reads as an interactive control.
.nest-preset-chip {
  all: unset;
  display: inline-flex;
  align-items: center;
  padding: 3px 10px;
  font-size: 11.5px;
  color: var(--nest-text-secondary);
  background: var(--nest-bg-elevated);
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-pill);
  cursor: pointer;
  max-width: 180px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;          // let flex shrink it under pressure
  flex-shrink: 1;
  transition: border-color var(--nest-transition-fast), color var(--nest-transition-fast);

  &:hover {
    border-color: var(--nest-accent);
    color: var(--nest-text);
  }
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
// column; narrower values center a readable measure. On phones the
// setting gets ignored because a 60% column = ~220px of readable text
// which is unusable.
.nest-chat-messages {
  max-width: var(--nest-chat-width, 820px);
  width: 100%;
  margin: 0 auto;
  padding: 24px 20px 60px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
@media (max-width: 600px) {
  .nest-chat-messages { max-width: 100%; }
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

// Jump-to-bottom pill — floats above the input, only when the user
// has scrolled up AND new content arrived below.
.nest-jump-bottom {
  position: absolute;
  bottom: calc(env(safe-area-inset-bottom) + 88px);
  left: 50%;
  transform: translateX(-50%);
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 14px 6px 10px;
  font-size: 12.5px;
  line-height: 1;
  color: var(--nest-text-on-accent, #fff);
  background: var(--nest-accent);
  border: 0;
  border-radius: 999px;
  box-shadow: 0 6px 22px rgba(0, 0, 0, 0.25);
  cursor: pointer;
  z-index: 5;
  transition: transform var(--nest-transition-fast), box-shadow var(--nest-transition-fast);

  &:hover { transform: translateX(-50%) translateY(-1px); }
  &:active { transform: translateX(-50%) scale(0.97); }
}
.nest-fade-enter-active, .nest-fade-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}
.nest-fade-enter-from, .nest-fade-leave-to {
  opacity: 0;
  transform: translateX(-50%) translateY(6px);
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
  // Preset chip gets tighter on phones. The label is still there but
  // capped harder so it can't shove the Send-settings button off-screen.
  .nest-preset-chip {
    max-width: 96px;
    padding: 3px 6px;
    font-size: 10.5px;
  }
  .nest-chat-messages { padding: 14px 12px 56px; }
  .nest-chat-input    { padding: 10px 12px max(14px, env(safe-area-inset-bottom)); }
}

// Mobile chat-list layout when no chat is selected — the ChatList
// component fills the main panel so users always see their chats from
// /chat without a selected id.
.nest-chat-mobile-list {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  padding: 8px;
}

// Burger button sits to the LEFT of the title, tighter than the regular
// icon row on the right.
.nest-chat-menu-btn {
  margin-right: 4px;
  flex-shrink: 0;
}
</style>
