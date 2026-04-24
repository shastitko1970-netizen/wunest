<script setup lang="ts">
import { computed, ref, watch, nextTick, onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useModelsStore } from '@/stores/models'
import { useAuthStore } from '@/stores/auth'
import { useChatsStore } from '@/stores/chats'
import { countTokens } from '@/lib/tokens'  // sync; see lib/tokens.ts
import { uploadAttachment } from '@/api/uploads'
import { useQuickRepliesStore } from '@/stores/quickReplies'
import { byokApi, type BYOKKey } from '@/api/byok'

const { t } = useI18n()

// Beta gate — sending is disabled until the user has redeemed an
// access code. Whole UI stays visible (banner in AppShell explains),
// but the Send button goes grey + shows a tooltip so the user can't
// spend a turn on a request the server would reject.
const auth = useAuthStore()
const { nestAccessGranted } = storeToRefs(auth)

const props = defineProps<{
  modelValue: string
  disabled?: boolean
  streaming?: boolean
  placeholder?: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: string): void
  (e: 'send'): void
  (e: 'stop'): void
}>()

const textarea = ref<HTMLTextAreaElement | null>(null)

// Quick replies (M39.2) — render above the textarea as clickable chips.
// Click inserts into draft; send_now=true auto-fires after insert.
const quickReplies = useQuickRepliesStore()
const { items: quickReplyItems } = storeToRefs(quickReplies)
onMounted(() => { if (!quickReplies.loaded) void quickReplies.fetchAll() })

function applyQuickReply(qr: { text: string; send_now: boolean }) {
  // Insert at cursor position if textarea has focus, else append.
  const ta = textarea.value
  if (ta && document.activeElement === ta) {
    const start = ta.selectionStart ?? ta.value.length
    const end = ta.selectionEnd ?? ta.value.length
    const before = ta.value.slice(0, start)
    const after = ta.value.slice(end)
    const next = before + qr.text + after
    emit('update:modelValue', next)
    // Restore focus + cursor after Vue repaints.
    nextTick(() => {
      ta.focus()
      const pos = before.length + qr.text.length
      ta.setSelectionRange(pos, pos)
      autosize()
    })
  } else {
    const trimmed = (props.modelValue || '').trim()
    const next = trimmed ? trimmed + ' ' + qr.text : qr.text
    emit('update:modelValue', next)
    nextTick(() => autosize())
  }
  if (qr.send_now && canSend.value) {
    nextTick(() => emit('send'))
  }
}

const models = useModelsStore()
const { items: modelOptions, selected: selectedModel, loading: modelsLoading, error: modelsError } = storeToRefs(models)

// Chats store provides the active chat — we need it so the provider picker
// can pin a BYOK onto the right row, and so we know which pin (if any) is
// currently active. The initial model-list fetch is owned by Chat.vue's
// setForChat call; here we just render whatever's in the store.
const chats = useChatsStore()
const { currentChat } = storeToRefs(chats)

// ─── Provider picker (M41) ───────────────────────────────────────────
//
// Replaces the deprecated model-only picker. Lists WuApi (default pool) +
// every saved BYOK key grouped by provider. Clicking one:
//   1. POST /api/chat/{id}/byok to pin the choice on the server
//   2. Mirror the change on the cached chat so other surfaces see it
//   3. Chat.vue's watcher picks up the chat_metadata change and refreshes
//      the models store — the model chip list updates itself.
// Dismisses the menu automatically on pick.

const byokKeys = ref<BYOKKey[]>([])
const byokLoading = ref(false)
const providerBusy = ref(false)

onMounted(async () => {
  byokLoading.value = true
  try {
    const r = await byokApi.list()
    byokKeys.value = r.items
  } catch {
    // Empty list is a fine fallback — user has no BYOK keys, only WuApi option.
  } finally {
    byokLoading.value = false
  }
})

const byokKeysGrouped = computed<Record<string, BYOKKey[]>>(() => {
  const out: Record<string, BYOKKey[]> = {}
  for (const k of byokKeys.value) {
    if (!out[k.provider]) out[k.provider] = []
    out[k.provider].push(k)
  }
  return out
})

const activeByokID = computed<string | null>(() => {
  const id = currentChat.value?.chat_metadata?.byok_id
  return typeof id === 'string' && id.length > 0 ? id : null
})

const activeProviderLabel = computed(() => {
  if (!activeByokID.value) return 'WuApi'
  const k = byokKeys.value.find(x => x.id === activeByokID.value)
  if (!k) return 'BYOK'
  const p = k.provider.charAt(0).toUpperCase() + k.provider.slice(1)
  return k.label ? `${p} · ${k.label}` : p
})

async function pickProvider(byokID: string | null) {
  if (!currentChat.value || providerBusy.value) return
  providerBusy.value = true
  try {
    await byokApi.setForChat(currentChat.value.id, byokID)
    // Mirror the change in the cached chat — the watcher in Chat.vue will
    // then trigger the models-store refresh. Keeping this in one place means
    // the BYOKPickerDialog in the header doesn't need its own copy of the
    // update logic; both paths go through the store's shared cache.
    const cur = currentChat.value
    if (cur) {
      cur.chat_metadata = { ...(cur.chat_metadata ?? {}), byok_id: byokID }
      const idx = chats.list.findIndex((c: { id: string }) => c.id === cur.id)
      if (idx >= 0) {
        chats.list[idx] = {
          ...chats.list[idx],
          chat_metadata: { ...(chats.list[idx].chat_metadata ?? {}), byok_id: byokID },
        }
      }
    }
  } catch (e) {
    console.error('BYOK switch failed', e)
  } finally {
    providerBusy.value = false
  }
}

function providerPrettyName(p: string): string {
  return p.charAt(0).toUpperCase() + p.slice(1)
}

async function refreshModels() {
  await models.refresh()
}

const canSend = computed(() =>
  !props.disabled
  && !props.streaming
  && nestAccessGranted.value
  && props.modelValue.trim().length > 0,
)

// Token estimation. Pure-sync char-heuristic (see lib/tokens.ts) so we
// can bind without a debounce; the ref stays in lockstep with the input.
const tokenCount = computed(() => countTokens(props.modelValue ?? ''))

function onInput(e: Event) {
  const el = e.target as HTMLTextAreaElement
  emit('update:modelValue', el.value)
  autosize()
}

function autosize() {
  const el = textarea.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = Math.min(el.scrollHeight, 240) + 'px'
}

watch(() => props.modelValue, () => nextTick(autosize))

function onKeydown(e: KeyboardEvent) {
  if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
    e.preventDefault()
    if (canSend.value) emit('send')
    return
  }
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    if (canSend.value) emit('send')
  }
}

// ─── Attachment upload ─────────────────────────────────────────────
//
// Three entry points — paperclip button, drag-drop, Ctrl+V paste — all
// converge on uploadFiles() which handles multiple files sequentially
// (we don't parallelise: cheap to implement, no race on cursor pos, and
// users rarely drop 10 images into one message).
//
// On success we insert Markdown `![name](url)` at the cursor so the
// message already renders the image preview. The same token also
// survives round-trip export/import and is understood by every other
// Markdown-rendering chat client — no bespoke syntax.

const fileInput = ref<HTMLInputElement | null>(null)
const uploading = ref(false)
const uploadError = ref<string | null>(null)
// isDragging flips true while a file is over the input area so we can
// paint a dashed outline — gives the user unambiguous feedback.
const isDragging = ref(false)

function pickAttachment() {
  fileInput.value?.click()
}

async function onAttachmentPicked(e: Event) {
  const input = e.target as HTMLInputElement
  const files = input.files
  if (files && files.length) {
    await uploadFiles(Array.from(files))
  }
  if (input) input.value = ''
}

async function onDrop(e: DragEvent) {
  isDragging.value = false
  const files = e.dataTransfer?.files
  if (!files || !files.length) return
  await uploadFiles(Array.from(files))
}

function onDragOver(e: DragEvent) {
  // Only light up when the drag payload actually contains files —
  // avoids flashing on text selections dragged from elsewhere.
  if (!e.dataTransfer || !Array.from(e.dataTransfer.items).some(i => i.kind === 'file')) return
  e.preventDefault()
  isDragging.value = true
}

function onDragLeave(e: DragEvent) {
  // Leaving a CHILD element fires a dragleave too — suppress it by
  // checking that the cursor left the wrap entirely.
  const related = e.relatedTarget as Node | null
  const wrap = (e.currentTarget as HTMLElement)
  if (related && wrap.contains(related)) return
  isDragging.value = false
}

// Paste handler — screenshot pasted via Ctrl+V arrives as a `file`
// entry on the clipboard. Images pasted as text fall through to the
// textarea's default paste path unchanged.
async function onPaste(e: ClipboardEvent) {
  const items = e.clipboardData?.items
  if (!items) return
  const files: File[] = []
  for (const item of Array.from(items)) {
    if (item.kind === 'file') {
      const f = item.getAsFile()
      if (f) files.push(f)
    }
  }
  if (files.length) {
    e.preventDefault()
    await uploadFiles(files)
  }
}

async function uploadFiles(files: File[]) {
  uploadError.value = null
  uploading.value = true
  try {
    for (const file of files) {
      // Only allow image uploads for now — catches the 99% case
      // (character art, screenshots) without opening an audio/video path
      // we haven't thought about yet.
      if (!file.type.startsWith('image/')) {
        uploadError.value = t('chat.input.attach.notImage', { name: file.name })
        continue
      }
      try {
        const res = await uploadAttachment(file)
        insertMarkdownImage(file.name || 'image', res.url)
      } catch (err: any) {
        uploadError.value = err?.message || String(err)
      }
    }
  } finally {
    uploading.value = false
  }
}

// Insert `![alt](url)` at the current cursor position. If the caret is
// not in the textarea (e.g. paperclip click while the drag-drop fired),
// we append at the end as a graceful fallback.
function insertMarkdownImage(alt: string, url: string) {
  const ta = textarea.value
  const snippet = `![${alt}](${url})`
  if (!ta) {
    emit('update:modelValue', (props.modelValue || '') + (props.modelValue ? '\n' : '') + snippet)
    return
  }
  const start = ta.selectionStart ?? ta.value.length
  const end   = ta.selectionEnd ?? ta.value.length
  const before = ta.value.slice(0, start)
  const after  = ta.value.slice(end)
  // Add a newline before/after when sandwiched against non-whitespace
  // — keeps Markdown from concatenating the image with nearby text.
  const pre  = before && !/\s$/.test(before) ? '\n' : ''
  const post = after  && !/^\s/.test(after)  ? '\n' : ''
  const next = before + pre + snippet + post + after
  emit('update:modelValue', next)
  nextTick(() => {
    ta.focus()
    const pos = before.length + pre.length + snippet.length + post.length
    ta.setSelectionRange(pos, pos)
    autosize()
  })
}
</script>

<template>
  <!-- Quick reply chips (M39.2) — horizontal scroll, click-to-insert.
       Rendered only when the user has at least one quick reply; empty
       state surfaces via Settings → Quick replies. -->
  <div v-if="quickReplyItems.length" class="nest-qr-strip">
    <button
      v-for="qr in quickReplyItems"
      :key="qr.id"
      type="button"
      class="nest-qr-chip"
      :class="{ 'send-now': qr.send_now }"
      :title="qr.text"
      :disabled="disabled || streaming"
      @click="applyQuickReply(qr)"
    >
      <v-icon v-if="qr.send_now" size="12">mdi-send</v-icon>
      <span>{{ qr.label }}</span>
    </button>
  </div>

  <div
    class="nest-input-wrap"
    :class="{ 'nest-input-wrap--dragging': isDragging }"
    @dragover="onDragOver"
    @dragleave="onDragLeave"
    @drop.prevent="onDrop"
  >
    <textarea
      ref="textarea"
      id="send_textarea"
      class="nest-input"
      :value="modelValue"
      :placeholder="placeholder ?? t('chat.input.placeholder')"
      :disabled="disabled"
      rows="1"
      @input="onInput"
      @keydown="onKeydown"
      @paste="onPaste"
    />
    <input
      ref="fileInput"
      type="file"
      accept="image/*"
      multiple
      style="display:none"
      @change="onAttachmentPicked"
    />

    <!-- Attachment upload status — shows while uploading and on error.
         Placed between the textarea and the action row so it's visible
         without squashing anything. -->
    <div v-if="uploading || uploadError" class="nest-upload-status">
      <v-progress-circular v-if="uploading" size="16" width="2" indeterminate color="primary" />
      <span v-if="uploading">{{ t('chat.input.attach.uploading') }}</span>
      <span v-else-if="uploadError" class="nest-upload-error">
        <v-icon size="14">mdi-alert-circle</v-icon>
        {{ uploadError }}
      </span>
      <v-btn
        v-if="uploadError && !uploading"
        size="x-small"
        variant="text"
        icon="mdi-close"
        @click="uploadError = null"
      />
    </div>

    <div class="nest-input-actions">
      <!-- Paperclip → file picker. Same upload path as drag-drop/paste. -->
      <v-btn
        variant="text"
        size="small"
        icon="mdi-paperclip"
        :disabled="disabled || uploading"
        :title="t('chat.input.attach.button')"
        @click="pickAttachment"
      />

      <!-- Provider picker (M41) + model picker, side by side.
           The provider picker flips between the WuApi pool and the user's
           saved BYOK keys; the model picker is scoped to whichever is active.
           The list of models is live-fetched on provider change so what you
           see is actually what the provider exposes right now (no hardcoded
           wu-tier list, no stale openrouter catalogue). -->
      <v-menu location="top start" offset="8">
        <template #activator="{ props: menuProps }">
          <button
            class="nest-model-btn nest-provider-btn"
            v-bind="menuProps"
            type="button"
            :disabled="providerBusy || !currentChat"
            :title="t('byok.picker.title')"
          >
            <v-icon size="14" class="mr-1">
              {{ activeByokID ? 'mdi-key-variant' : 'mdi-cloud-outline' }}
            </v-icon>
            <span>{{ activeProviderLabel }}</span>
            <v-icon size="14" class="ml-1">mdi-chevron-up</v-icon>
          </button>
        </template>
        <v-list density="compact" class="nest-model-list">
          <v-list-item
            :active="!activeByokID"
            :disabled="providerBusy"
            @click="pickProvider(null)"
          >
            <template #prepend>
              <v-icon size="16">mdi-cloud-outline</v-icon>
            </template>
            <v-list-item-title>WuApi</v-list-item-title>
            <v-list-item-subtitle class="nest-mini-hint">
              {{ t('byok.picker.useDefaultHint') }}
            </v-list-item-subtitle>
          </v-list-item>
          <template v-if="byokKeys.length">
            <v-divider class="my-1" />
            <template v-for="(keys, provider) in byokKeysGrouped" :key="provider">
              <v-list-subheader class="nest-mono">
                {{ providerPrettyName(provider) }}
              </v-list-subheader>
              <v-list-item
                v-for="k in keys"
                :key="k.id"
                :active="activeByokID === k.id"
                :disabled="providerBusy"
                @click="pickProvider(k.id)"
              >
                <template #prepend>
                  <v-icon size="16">mdi-key-variant</v-icon>
                </template>
                <v-list-item-title>
                  {{ k.label || t('byok.unnamed') }}
                </v-list-item-title>
                <v-list-item-subtitle class="nest-mono nest-mini-hint">
                  {{ k.masked }}
                </v-list-item-subtitle>
              </v-list-item>
            </template>
          </template>
          <v-divider class="my-1" />
          <v-list-item :href="'/settings'" density="compact">
            <template #prepend>
              <v-icon size="16">mdi-cog-outline</v-icon>
            </template>
            <v-list-item-title class="nest-mini-hint">
              {{ t('byok.picker.manageKeys') }}
            </v-list-item-title>
          </v-list-item>
        </v-list>
      </v-menu>

      <!-- Model picker: now live-populated from whichever provider is
           active. Empty list = provider rejected /models (bad key, or their
           catalogue endpoint is temporarily down) — user still sees the
           currently-selected id so they can send if it's actually valid. -->
      <v-menu location="top start" offset="8">
        <template #activator="{ props: menuProps }">
          <button class="nest-model-btn" v-bind="menuProps" type="button">
            <v-icon size="14" class="mr-1">mdi-brain</v-icon>
            <span class="nest-mono">{{ selectedModel || t('chat.input.modelEmpty') }}</span>
            <v-progress-circular
              v-if="modelsLoading"
              indeterminate
              size="10"
              width="1.5"
              class="ml-1"
            />
            <v-icon v-else size="14" class="ml-1">mdi-chevron-up</v-icon>
          </button>
        </template>
        <v-list density="compact" class="nest-model-list">
          <!-- Surfaced error: an empty list after a failed fetch is
               indistinguishable from "no models here yet"; showing the
               actual provider message turns a debugging dead-end into a
               one-glance answer ("HTTP 401: invalid api key"). -->
          <div v-if="modelsError && !modelOptions.length" class="nest-model-error">
            <v-icon size="14" color="error">mdi-alert-circle</v-icon>
            <span class="nest-mono">{{ modelsError }}</span>
          </div>
          <v-list-item
            v-for="m in modelOptions"
            :key="m.id"
            :active="m.id === selectedModel"
            @click="models.select(m.id)"
          >
            <v-list-item-title class="nest-mono">{{ m.id }}</v-list-item-title>
          </v-list-item>
          <v-list-item v-if="!modelOptions.length && !modelsLoading && !modelsError" disabled>
            <v-list-item-title class="nest-mini-hint">
              {{ t('chat.input.modelEmpty') }}
            </v-list-item-title>
          </v-list-item>
          <v-divider class="my-1" />
          <v-list-item :disabled="modelsLoading" @click="refreshModels">
            <template #prepend>
              <v-icon size="16">mdi-refresh</v-icon>
            </template>
            <v-list-item-title class="nest-mini-hint">
              {{ t('chat.input.modelRefresh') }}
            </v-list-item-title>
          </v-list-item>
        </v-list>
      </v-menu>

      <span
        v-if="tokenCount > 0"
        class="nest-token-count nest-mono"
        :title="t('chat.input.tokensTitle')"
      >
        {{ tokenCount }} {{ t('chat.input.tokensShort') }}
      </span>

      <div class="flex-grow-1" />

      <v-btn
        v-if="streaming"
        color="error"
        variant="tonal"
        size="small"
        prepend-icon="mdi-stop-circle-outline"
        @click="emit('stop')"
      >
        {{ t('chat.input.stop') }}
      </v-btn>
      <v-btn
        v-else
        id="send_but"
        color="primary"
        variant="flat"
        size="small"
        :disabled="!canSend"
        :title="!nestAccessGranted ? t('accessBanner.body') : undefined"
        append-icon="mdi-send"
        @click="emit('send')"
      >
        {{ t('chat.input.send') }}
      </v-btn>
    </div>

    <!-- Drag overlay: pseudo-element painted by the .nest-input-wrap--dragging
         rule below. Kept purely visual so the `drop` event still fires on
         the wrapper. -->
    <div v-if="isDragging" class="nest-drop-hint">
      <v-icon size="20" color="primary">mdi-image-plus</v-icon>
      {{ t('chat.input.attach.dropHint') }}
    </div>
  </div>
</template>

<style lang="scss" scoped>
.nest-qr-strip {
  display: flex;
  gap: 6px;
  overflow-x: auto;
  padding: 4px 4px 8px;
  -webkit-overflow-scrolling: touch;

  &::-webkit-scrollbar { height: 4px; }
  &::-webkit-scrollbar-thumb { background: var(--nest-border); border-radius: 4px; }
}
.nest-qr-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
  padding: 4px 10px;
  font-size: 12px;
  color: var(--nest-text-secondary);
  background: var(--nest-bg-elevated);
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-pill);
  cursor: pointer;
  transition: border-color var(--nest-transition-fast), color var(--nest-transition-fast);

  &:hover:not(:disabled) {
    border-color: var(--nest-accent);
    color: var(--nest-text);
  }
  &.send-now {
    // Paper-plane icon marks chips that auto-send on click.
    .v-icon { color: var(--nest-accent); }
  }
  &:disabled { opacity: 0.5; cursor: not-allowed; }
}

.nest-input-wrap {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
  border: 1px solid var(--nest-border);
  border-radius: var(--nest-radius);
  background: var(--nest-surface);
  transition: border-color var(--nest-transition-fast), background var(--nest-transition-fast);

  &:focus-within {
    border-color: var(--nest-accent);
  }

  // Drag-over highlight: dashed accent outline + faint fill so the user
  // can tell the drop target is this input specifically and not the
  // whole chat view.
  &--dragging {
    border-style: dashed;
    border-color: var(--nest-accent);
    background: color-mix(in srgb, var(--nest-accent) 6%, var(--nest-surface));
  }
}

// Centered drop hint chip that appears only while a file is being
// dragged over the input. Pointer-events:none so the drop event still
// lands on the wrap.
.nest-drop-hint {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 500;
  color: var(--nest-accent);
  background: color-mix(in srgb, var(--nest-surface) 70%, transparent);
  border-radius: var(--nest-radius);
  pointer-events: none;
  z-index: 2;
}

// Upload progress / error line — compact row between the textarea and
// the action buttons.
.nest-upload-status {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12.5px;
  color: var(--nest-text-secondary);
}
.nest-upload-error {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: rgb(var(--v-theme-error));
}

.nest-input {
  width: 100%;
  resize: none;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--nest-text);
  font: 15px/1.5 var(--nest-font-body);
  max-height: 240px;
  overflow-y: auto;

  &::placeholder { color: var(--nest-text-muted); }
  &:disabled { opacity: 0.5; cursor: not-allowed; }
}

.nest-input-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.nest-model-btn {
  display: inline-flex;
  align-items: center;
  padding: 4px 10px;
  font-size: 11.5px;
  background: var(--nest-bg-elevated);
  color: var(--nest-text-secondary);
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-pill);
  cursor: pointer;
  transition: border-color var(--nest-transition-fast), color var(--nest-transition-fast);
  // Cap width on narrow viewports so a long model id doesn't push Send off-screen.
  max-width: 60vw;
  min-width: 0;

  & > .nest-mono {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  &:hover {
    border-color: var(--nest-accent);
    color: var(--nest-text);
  }
}

.nest-model-list {
  background: var(--nest-surface) !important;
  border: 1px solid var(--nest-border);
  max-height: 320px;
  min-width: 240px;
}

// Provider picker sits next to the model picker. When the chat has a BYOK
// pin active, tint the border to make the switch visible at a glance.
.nest-provider-btn {
  max-width: 40vw;

  & > span {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
}
.nest-provider-btn:not(:disabled):has(.mdi-key-variant) {
  border-color: var(--nest-accent);
  color: var(--nest-text);
}

.nest-mini-hint {
  font-size: 11px;
  color: var(--nest-text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

// Inline provider-error surfaced inside the model-picker popover. Wraps to
// multi-line because provider messages can be long ("Invalid API key: your
// organization 'org_abc' does not have access to this model family …").
.nest-model-error {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  padding: 8px 12px;
  margin: 4px 8px;
  border-radius: var(--nest-radius-sm);
  background: rgba(244, 67, 54, 0.08);
  border: 1px solid rgba(244, 67, 54, 0.25);
  font-size: 10.5px;
  max-width: 320px;
  line-height: 1.35;

  span {
    word-break: break-word;
    white-space: normal;
  }
}

.nest-token-count {
  font-size: 10.5px;
  color: var(--nest-text-muted);
  padding: 2px 6px;
  border-radius: var(--nest-radius-pill);
  background: var(--nest-bg-elevated);
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}
</style>
