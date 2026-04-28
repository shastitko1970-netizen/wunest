<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useDisplay } from 'vuetify'
import type { Message } from '@/api/chats'
import MessageContent from '@/components/MessageContent.vue'

const { t } = useI18n()
// M52.10 — mobile-only: collapse the action row into a single ⋮ menu.
// `mdAndDown` matches the chat layout breakpoint already used in
// Chat.vue (sidebar hides at the same threshold).
const { mdAndDown } = useDisplay()

const props = defineProps<{
  message: Message
  characterName?: string
  userName?: string
  streaming?: boolean
  // Only the last assistant message gets a Regenerate action in V1.
  allowRegenerate?: boolean
  /** Optional map of character_id → display name. Supplied by Chat.vue
   *  in group chats so each assistant message can show who spoke. */
  characterNames?: Record<string, string>
}>()

const emit = defineEmits<{
  (e: 'delete', m: Message): void
  (e: 'delete-after', m: Message): void
  (e: 'fork', m: Message): void
  (e: 'regenerate', m: Message): void
  (e: 'continue', m: Message): void
  (e: 'toggle-hidden', m: Message): void
  (e: 'swipe', m: Message): void
  (e: 'select-swipe', m: Message, swipeID: number): void
  (e: 'edit', m: Message, newContent: string): void
  // Bubbled from the author-supplied plate actions (data-nest-action).
  // Chat.vue listens and forwards to the chats store / composer.
  (e: 'plate-draft', text: string, send: boolean): void
  (e: 'plate-toast', level: 'info' | 'success' | 'error', text: string): void
}>()

const isUser = computed(() => props.message.role === 'user')

// Collapse state — local to the row. Users can fold an assistant reply
// into a short preview while browsing a long scroll, then expand it back.
// Local because it's a pure view choice; persisting across reloads would
// just be clutter. Streaming messages are never collapsed — watch below
// auto-expands if a collapsed row starts receiving tokens (rare but a
// continue+reroll can do it).
const collapsed = ref(false)
function toggleCollapse() {
  collapsed.value = !collapsed.value
}
watch(() => props.streaming, (s) => {
  if (s && collapsed.value) collapsed.value = false
})

// Resolve the speaker for the currently-visible swipe. Priority:
//   1. swipe_character_ids[swipe_id] — per-swipe attribution, used for
//      group-greetings pool where each swipe is a different character
//   2. message.character_id — message-level attribution (normal case)
//   3. nil — fall back to the chat-level characterName
// Returns the display string, not the id.
const displayName = computed(() => {
  if (isUser.value) return props.userName || t('chat.you')
  const names = props.characterNames ?? {}
  const swipeAttr = props.message.swipe_character_ids
  if (swipeAttr && swipeAttr.length > 0) {
    const i = Math.max(0, Math.min(swipeAttr.length - 1, props.message.swipe_id ?? 0))
    const cid = swipeAttr[i]
    if (cid && names[cid]) return names[cid]
  }
  const msgCID = props.message.character_id
  if (msgCID && names[msgCID]) {
    return names[msgCID]
  }
  return props.characterName || t('chat.assistant')
})
const timestamp = computed(() => {
  const d = new Date(props.message.created_at)
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
})

const hasError = computed(() => !!props.message.extras?.error)

// Swipe pagination: only meaningful on assistant rows that have been
// regenerated/swiped at least once. A fresh message has swipes: [] and
// we hide the strip entirely (totalSwipes === 0).
const totalSwipes = computed(() => {
  const s = props.message.swipes
  return Array.isArray(s) ? s.length : 0
})
const currentSwipe = computed(() => props.message.swipe_id ?? 0)

// Greeting detection — server tags seed-messages with extras.model =
// "greeting". We use this to label the swipe counter so users see
// "Приветствие 1/3" instead of a bare "1/3" and realize the chevrons
// let them browse alternate_greetings from the character card.
const isGreeting = computed(() => props.message.extras?.model === 'greeting')

// ── Plate action bubble handler ─────────────────────────────────
// Forwards a bubbled `data-nest-action` click from MessageContent to
// the message-level action (swipe, regenerate, edit, delete) — same
// handlers the toolbar uses, just triggered from inline buttons.
function onPlateBubbleAction(name: 'swipe-prev' | 'swipe-next' | 'regenerate' | 'edit' | 'delete') {
  switch (name) {
    case 'swipe-prev':
      if (currentSwipe.value > 0) {
        emit('select-swipe', props.message, currentSwipe.value - 1)
      }
      break
    case 'swipe-next':
      if (totalSwipes.value > 0 && currentSwipe.value < totalSwipes.value - 1) {
        emit('select-swipe', props.message, currentSwipe.value + 1)
      }
      break
    case 'regenerate':
      emit('regenerate', props.message)
      break
    case 'edit':
      startEdit()
      break
    case 'delete':
      emit('delete', props.message)
      break
  }
}
const tokensInfo = computed(() => {
  const ex = props.message.extras
  if (!ex?.tokens_out) return null
  const ms = ex.latency_ms ?? 0
  return `${ex.tokens_out} tok · ${(ms / 1000).toFixed(1)}s · ${ex.model}`
})

// Reasoning (thinking) content from <think>…</think> extraction.
// Shown as a collapsible block above the main content. Auto-collapses
// once streaming is done (opens while thinking is happening if we had
// per-token reasoning events — for now we just show the persisted value).
const reasoning = computed(() => props.message.extras?.reasoning || '')

// During streaming, the raw content may still contain <think>…</think>
// tags that the server hasn't extracted yet. Render them as a distinct
// "live" thinking block without mutating the source.
const liveSplit = computed(() => {
  if (!props.streaming || !props.message.content) {
    return { live: '', rest: props.message.content }
  }
  const open = props.message.content.indexOf('<think>')
  if (open === -1) return { live: '', rest: props.message.content }
  const close = props.message.content.indexOf('</think>', open + 7)
  if (close === -1) {
    // Unclosed — everything after <think> is reasoning so far.
    return {
      live: props.message.content.slice(open + 7),
      rest: props.message.content.slice(0, open),
    }
  }
  return {
    live: props.message.content.slice(open + 7, close),
    rest: props.message.content.slice(0, open) + props.message.content.slice(close + 8),
  }
})

// ─── Edit mode ─────────────────────────────────────────────────────
const editing = ref(false)
const draft = ref('')
const editArea = ref<HTMLTextAreaElement | null>(null)

function startEdit() {
  draft.value = props.message.content
  editing.value = true
  nextTick(() => {
    const el = editArea.value
    if (el) {
      el.focus()
      el.selectionStart = el.selectionEnd = el.value.length
      autosizeEdit()
    }
  })
}

function cancelEdit() {
  editing.value = false
  draft.value = ''
}

function saveEdit() {
  const next = draft.value.trim()
  if (next !== props.message.content) {
    emit('edit', props.message, next)
  }
  editing.value = false
}

function autosizeEdit() {
  const el = editArea.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = Math.min(el.scrollHeight, 400) + 'px'
}

watch(draft, () => nextTick(autosizeEdit))

function onEditKeydown(e: KeyboardEvent) {
  if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
    e.preventDefault()
    saveEdit()
  } else if (e.key === 'Escape') {
    e.preventDefault()
    cancelEdit()
  }
}
</script>

<template>
  <!-- ST-compat aliases: `mes` + last-of-type marker `last_mes` surface the
       same hooks an ST theme would target. WuNest's own styles stay on the
       `nest-*` classes; the aliases are layout-silent. -->
  <div
    class="nest-msg mes"
    :data-message-id="message.id"
    :class="{
      'is-user': isUser,
      'is-streaming': streaming,
      'is-hidden': !!message.hidden,
      'mes_user': isUser,
      'mes_char': !isUser,
    }"
  >
    <div class="nest-msg-header">
      <span class="nest-msg-name mes_name">{{ displayName }}</span>
      <span class="nest-msg-time nest-mono">{{ timestamp }}</span>
    </div>

    <!-- Persisted reasoning from server (after stream completes).
         <details> handles its own open/close state natively on summary click. -->
    <details v-if="reasoning && !editing" class="nest-reasoning">
      <summary class="nest-reasoning-summary">
        <v-icon size="14" class="mr-1">mdi-brain</v-icon>
        <span>{{ t('chat.thinking.label') }}</span>
        <span class="nest-mono nest-reasoning-meta">
          {{ t('chat.thinking.chars', { n: reasoning.length }) }}
        </span>
      </summary>
      <div class="nest-reasoning-body">{{ reasoning }}</div>
    </details>

    <!-- Live <think> block while streaming — rendered even before server
         extracts it at end-of-stream. -->
    <div v-if="streaming && liveSplit.live" class="nest-reasoning nest-reasoning--live">
      <div class="nest-reasoning-summary">
        <v-icon size="14" class="mr-1">mdi-brain</v-icon>
        <span>{{ t('chat.live.thinking') }}</span>
      </div>
      <div class="nest-reasoning-body">{{ liveSplit.live }}<span class="nest-cursor">▍</span></div>
    </div>

    <div class="nest-msg-body mes_block" :class="{ 'is-error': hasError }">
      <template v-if="editing">
        <textarea
          ref="editArea"
          v-model="draft"
          class="nest-edit-area"
          rows="3"
          @input="autosizeEdit"
          @keydown="onEditKeydown"
        />
        <div class="nest-edit-actions">
          <span class="nest-mono nest-edit-hint">{{ t('chat.edit.hint') }}</span>
          <v-btn size="x-small" variant="text" @click="cancelEdit">{{ t('common.cancel') }}</v-btn>
          <v-btn size="x-small" color="primary" variant="flat" @click="saveEdit">{{ t('common.save') }}</v-btn>
        </div>
      </template>
      <template v-else-if="hasError">
        <v-icon size="16" color="error" class="mr-1">mdi-alert-circle</v-icon>
        <span class="text-error">
          {{ t('chat.generationFailed', { error: message.extras?.error ?? '' }) }}
        </span>
      </template>
      <template v-else-if="!message.content && streaming && !liveSplit.live">
        <!-- "Thinking" placeholder — shown when the assistant row exists
             but no tokens have arrived yet. Wording matches the
             streaming-toggle hint in Settings ("Думает… then the full
             response"), so users who turned streaming off actually see
             what the hint promised. The three dots are separate spans
             so they can be individually staggered in the animation. -->
        <span class="nest-thinking">
          <span class="nest-thinking-label">{{ t('chat.thinkingPlaceholder') }}</span>
          <span class="nest-thinking-dot">.</span>
          <span class="nest-thinking-dot">.</span>
          <span class="nest-thinking-dot">.</span>
        </span>
      </template>
      <template v-else>
        <div
          class="nest-msg-content mes_text"
          :class="{ 'is-collapsed': collapsed }"
        >
          <!-- Streaming: while tokens fly in, render as plain text so we
               don't re-parse markdown on every chunk. Once the stream ends,
               swap to rich markdown + JSON plate rendering. -->
          <template v-if="streaming">
            <span class="nest-streaming-text">{{ liveSplit.rest }}</span><span class="nest-cursor">▍</span>
          </template>
          <MessageContent
            v-else
            :content="message.content"
            @bubble-action="onPlateBubbleAction"
            @draft="(text, send) => emit('plate-draft', text, send)"
            @toast="(level, text) => emit('plate-toast', level, text)"
          />
        </div>
        <!-- "Show full message" affordance when collapsed. Click expands. -->
        <button
          v-if="collapsed"
          class="nest-collapse-more"
          type="button"
          @click="toggleCollapse"
        >
          {{ t('chat.actions.expand') }}
        </button>
      </template>
    </div>

    <div v-if="tokensInfo && !streaming" class="nest-msg-footer nest-mono">
      {{ tokensInfo }}
    </div>

    <!-- Action row: shown on hover/focus, hidden while editing/streaming. -->
    <div v-if="!editing && !streaming" class="nest-msg-actions">
      <!-- Swipe navigation: on any assistant row with multiple variants.
           Previously we gated by allowRegenerate (last-message-only),
           which hid greeting-swipes the moment the user posted a reply.
           Now chevrons appear for ALL assistant swipes — paging through
           a greeting mid-conversation just updates that bubble, matches
           ST's behavior. The "+" new-variant button stays allowRegenerate-
           only since it costs an API turn. -->
      <template v-if="!isUser && totalSwipes > 1">
        <button
          class="nest-action-btn"
          :title="t('chat.swipe.prev')"
          :disabled="currentSwipe === 0"
          @click="emit('select-swipe', message, currentSwipe - 1)"
        >
          <v-icon size="14">mdi-chevron-left</v-icon>
        </button>
        <span class="nest-swipe-count nest-mono" :class="{ 'is-greeting': isGreeting }">
          <template v-if="isGreeting">
            {{ t('chat.swipe.greetingLabel') }}
          </template>
          {{ currentSwipe + 1 }}/{{ totalSwipes }}
        </span>
        <button
          class="nest-action-btn"
          :title="t('chat.swipe.next')"
          :disabled="currentSwipe === totalSwipes - 1"
          @click="emit('select-swipe', message, currentSwipe + 1)"
        >
          <v-icon size="14">mdi-chevron-right</v-icon>
        </button>
      </template>
      <!-- M52.10 — Action buttons after swipe-nav.
           Desktop (mdAndUp): inline row of icon-buttons (current behaviour).
           Mobile (mdAndDown): single ⋮ trigger that opens a v-menu with
           all actions as v-list items (titles + icons). Saves horizontal
           space on phones where 9 inline buttons wrapped to 2-3 lines. -->
      <template v-if="!mdAndDown">
        <template v-if="!isUser && allowRegenerate">
          <button
            class="nest-action-btn"
            :title="t('chat.swipe.newVariant')"
            @click="emit('swipe', message)"
          >
            <v-icon size="14">mdi-plus</v-icon>
          </button>
        </template>
        <button
          v-if="allowRegenerate"
          class="nest-action-btn"
          :title="t('chat.actions.continue')"
          @click="emit('continue', message)"
        >
          <v-icon size="14">mdi-play-outline</v-icon>
        </button>
        <button
          v-if="allowRegenerate"
          class="nest-action-btn"
          :title="t('chat.actions.regenerate')"
          @click="emit('regenerate', message)"
        >
          <v-icon size="14">mdi-reload</v-icon>
        </button>
        <button
          class="nest-action-btn"
          :title="collapsed ? t('chat.actions.expand') : t('chat.actions.collapse')"
          @click="toggleCollapse"
        >
          <v-icon size="14">
            {{ collapsed ? 'mdi-arrow-expand-vertical' : 'mdi-arrow-collapse-vertical' }}
          </v-icon>
        </button>
        <button
          class="nest-action-btn"
          :title="t('chat.actions.fork')"
          @click="emit('fork', message)"
        >
          <v-icon size="14">mdi-source-branch</v-icon>
        </button>
        <button
          class="nest-action-btn"
          :title="t('chat.actions.edit')"
          @click="startEdit"
        >
          <v-icon size="14">mdi-pencil-outline</v-icon>
        </button>
        <button
          class="nest-action-btn"
          :title="message.hidden ? t('chat.actions.unhide') : t('chat.actions.hide')"
          @click="emit('toggle-hidden', message)"
        >
          <v-icon size="14">{{ message.hidden ? 'mdi-eye-outline' : 'mdi-eye-off-outline' }}</v-icon>
        </button>
        <button
          class="nest-action-btn nest-action-btn--danger"
          :title="t('chat.actions.delete')"
          @click="emit('delete', message)"
        >
          <v-icon size="14">mdi-delete-outline</v-icon>
        </button>
        <button
          class="nest-action-btn nest-action-btn--danger"
          :title="t('chat.actions.deleteAfter')"
          @click="emit('delete-after', message)"
        >
          <v-icon size="14">mdi-delete-sweep-outline</v-icon>
        </button>
      </template>

      <!-- Mobile: regenerate stays as a dedicated icon-button (top-1 use
           case — users hit it on every swipe), the rest collapse into ⋮. -->
      <template v-else>
        <button
          v-if="allowRegenerate"
          class="nest-action-btn"
          :title="t('chat.actions.regenerate')"
          @click="emit('regenerate', message)"
        >
          <v-icon size="14">mdi-reload</v-icon>
        </button>
        <v-menu
          location="bottom end"
          offset="4"
        >
          <template #activator="{ props: menuProps }">
            <button
              v-bind="menuProps"
              class="nest-action-btn"
              :title="t('chat.actions.more')"
            >
              <v-icon size="14">mdi-dots-vertical</v-icon>
            </button>
          </template>
          <v-list density="compact" min-width="220">
            <v-list-item
              v-if="!isUser && allowRegenerate"
              prepend-icon="mdi-plus"
              :title="t('chat.swipe.newVariant')"
              @click="emit('swipe', message)"
            />
            <v-list-item
              v-if="allowRegenerate"
              prepend-icon="mdi-play-outline"
              :title="t('chat.actions.continue')"
              @click="emit('continue', message)"
            />
            <v-list-item
              :prepend-icon="collapsed ? 'mdi-arrow-expand-vertical' : 'mdi-arrow-collapse-vertical'"
              :title="collapsed ? t('chat.actions.expand') : t('chat.actions.collapse')"
              @click="toggleCollapse"
            />
            <v-list-item
              prepend-icon="mdi-source-branch"
              :title="t('chat.actions.fork')"
              @click="emit('fork', message)"
            />
            <v-list-item
              prepend-icon="mdi-pencil-outline"
              :title="t('chat.actions.edit')"
              @click="startEdit"
            />
            <v-list-item
              :prepend-icon="message.hidden ? 'mdi-eye-outline' : 'mdi-eye-off-outline'"
              :title="message.hidden ? t('chat.actions.unhide') : t('chat.actions.hide')"
              @click="emit('toggle-hidden', message)"
            />
            <v-divider class="my-1" />
            <v-list-item
              prepend-icon="mdi-delete-outline"
              :title="t('chat.actions.delete')"
              base-color="error"
              @click="emit('delete', message)"
            />
            <v-list-item
              prepend-icon="mdi-delete-sweep-outline"
              :title="t('chat.actions.deleteAfter')"
              base-color="error"
              @click="emit('delete-after', message)"
            />
          </v-list>
        </v-menu>
      </template>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.nest-msg {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 14px 16px;
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius);
  background: var(--nest-surface);
  max-width: 100%;
  // Defensive floor — a hostile user CSS targeting `.mes { max-width: 60px }`
  // or similar shouldn't starve the message to unreadable width. Min-width
  // ensures the bubble keeps room for at least one word across devices.
  // We intentionally DON'T use !important so users can still override
  // this consciously (e.g. compact density themes).
  min-width: min(100%, 240px);

  // Silent / hidden message — feeds into prompt but greyed out in UI.
  // Makes it unmistakable that the row is "model sees this, you don't
  // need to". Still interactive (edit, unhide, delete) — just visually
  // de-emphasised.
  &.is-hidden {
    opacity: 0.5;
    border-style: dashed;
  }

  &.is-user {
    background: var(--nest-bg-elevated);
    border-color: var(--nest-border);
  }

  &:hover .nest-msg-actions {
    opacity: 1;
    pointer-events: auto;
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
  // Scale chat message text with the appearance fontScale slider.
  // Baseline 15px × multiplier (1 = default, 0.75–1.4 range). UI chrome
  // elsewhere (Vuetify buttons, header chips) stays at native size so
  // the chrome doesn't shrink/grow with the reader's preference.
  font-size: calc(15px * var(--nest-chat-font-scale, 1));
  line-height: 1.55;
  color: var(--nest-text);
  white-space: pre-wrap;
  word-wrap: break-word;
}

.nest-msg-content { display: inline; }

// Collapsed state — clip to roughly one-and-a-half lines with a
// gradient mask so there's a visual cue the message continues. Content
// is still in the DOM (Ctrl-F / screen readers still see it), just
// clipped visually. `.nest-collapse-more` below is the click target to
// expand back.
.nest-msg-content.is-collapsed {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  position: relative;
  mask-image: linear-gradient(to bottom, black 40%, transparent 100%);
  -webkit-mask-image: linear-gradient(to bottom, black 40%, transparent 100%);
  cursor: pointer;
  max-height: 3em;
}
.nest-collapse-more {
  all: unset;
  display: inline-block;
  margin-top: 4px;
  font-size: 11px;
  color: var(--nest-accent);
  cursor: pointer;
  font-family: var(--nest-font-mono);
  letter-spacing: 0.03em;

  &:hover { text-decoration: underline; }
}

.nest-cursor {
  color: var(--nest-accent);
  animation: nest-blink 0.9s steps(2) infinite;
}

@keyframes nest-blink {
  0%,  50% { opacity: 1; }
  51%, 100% { opacity: 0; }
}

// "Thinking…" placeholder — label sits steady, the three dots pulse in
// sequence so it reads as activity rather than a dead indicator.
.nest-thinking {
  color: var(--nest-text-secondary);
  font-style: italic;
  display: inline-flex;
  align-items: baseline;
  gap: 1px;
}
.nest-thinking-label {
  color: var(--nest-accent);
  margin-right: 2px;
  font-style: italic;
}
.nest-thinking-dot {
  animation: nest-thinking-pulse 1.4s ease-in-out infinite;
  color: var(--nest-accent);
  font-weight: bold;
}
.nest-thinking-dot:nth-child(2) { animation-delay: 0.2s; }
.nest-thinking-dot:nth-child(3) { animation-delay: 0.4s; }
.nest-thinking-dot:nth-child(4) { animation-delay: 0.6s; }

@keyframes nest-thinking-pulse {
  0%, 80%, 100% { opacity: 0.25; transform: translateY(0); }
  40%           { opacity: 1;    transform: translateY(-2px); }
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

// ── Reasoning (thinking) collapsible ────────────────────────────────
.nest-reasoning {
  background: var(--nest-bg-elevated);
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
  padding: 8px 12px;
  margin: 4px 0;
  font-size: 13px;
  color: var(--nest-text-secondary);

  &[open] .nest-reasoning-summary {
    margin-bottom: 6px;
  }
}
.nest-reasoning--live {
  // Streaming live block — slightly brighter so the user sees the model
  // thinking in real time.
  border-color: var(--nest-border);
}
.nest-reasoning-summary {
  display: flex;
  align-items: center;
  gap: 4px;
  cursor: pointer;
  user-select: none;
  font-family: var(--nest-font-mono);
  font-size: 10.5px;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--nest-text-muted);
  list-style: none;

  &::-webkit-details-marker { display: none; }
}
.nest-reasoning-meta {
  margin-left: auto;
  color: var(--nest-text-muted);
  opacity: 0.6;
}
.nest-reasoning-body {
  font-family: var(--nest-font-mono);
  // Reasoning is chat content — scale with fontScale so users who
  // sized up for the message body can actually read the think block too.
  font-size: calc(12px * var(--nest-chat-font-scale, 1));
  line-height: 1.55;
  color: var(--nest-text-secondary);
  white-space: pre-wrap;
  word-wrap: break-word;
}

// ── Inline edit mode ────────────────────────────────────────────────
.nest-edit-area {
  width: 100%;
  resize: none;
  border: 1px solid var(--nest-accent);
  border-radius: var(--nest-radius-sm);
  background: var(--nest-bg);
  color: var(--nest-text);
  font: 15px/1.55 var(--nest-font-body);
  padding: 8px 10px;
  outline: none;
  max-height: 400px;
  overflow-y: auto;
}
.nest-edit-actions {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 6px;
  margin-top: 6px;
}
.nest-edit-hint {
  flex: 1;
  font-size: 10.5px;
  color: var(--nest-text-muted);
}

// ── Action icons row ────────────────────────────────────────────────
.nest-msg-actions {
  position: absolute;
  top: 6px;
  right: 6px;
  display: flex;
  gap: 2px;
  opacity: 0;
  pointer-events: none;
  transition: opacity var(--nest-transition-fast);
}
.nest-action-btn {
  background: var(--nest-bg-elevated);
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
  color: var(--nest-text-muted);
  padding: 4px;
  cursor: pointer;
  transition: color var(--nest-transition-fast), border-color var(--nest-transition-fast), background var(--nest-transition-fast);

  &:hover {
    color: var(--nest-text);
    border-color: var(--nest-border);
    background: var(--nest-surface);
  }
  &:disabled {
    opacity: 0.4;
    cursor: not-allowed;
    pointer-events: none;
  }
  &.nest-action-btn--danger:hover {
    color: var(--nest-accent);
    border-color: var(--nest-accent);
  }
}

.nest-swipe-count {
  font-size: 10.5px;
  color: var(--nest-text-muted);
  padding: 2px 4px;
  align-self: center;
  font-variant-numeric: tabular-nums;
  display: inline-flex;
  align-items: center;
  gap: 4px;

  // Greeting messages get a labeled pill so users see it's a navigator
  // for alternate_greetings, not a regenerate counter. Brighter color
  // + background so it reads as an interactive affordance on a fresh
  // chat (before other hover-opacity actions would cue-in).
  &.is-greeting {
    color: var(--nest-accent);
    background: color-mix(in srgb, var(--nest-accent) 10%, transparent);
    border-radius: var(--nest-radius-pill);
    padding: 2px 10px;
    font-size: 11px;
    font-weight: 500;
  }
}

// Mobile: actions always visible (no hover affordance).
@media (hover: none) {
  .nest-msg-actions {
    opacity: 1;
    pointer-events: auto;
    position: static;
    margin-top: 4px;
    justify-content: flex-end;
  }
}
</style>
