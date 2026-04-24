<script setup lang="ts">
import { computed, ref, watch, nextTick, onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useModelsStore } from '@/stores/models'
import { useAuthStore } from '@/stores/auth'
import { countTokens } from '@/lib/tokens'  // sync; see lib/tokens.ts
import { uploadAttachment } from '@/api/uploads'
import { useQuickRepliesStore } from '@/stores/quickReplies'

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
const { options: modelOptions, selected: selectedModel } = storeToRefs(models)

// Lazy-load the model list on mount. The picker uses fallback models
// (wu-tier list) until the API call resolves — feels instant.
onMounted(() => { if (!models.loaded) void models.fetchList() })

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

      <!-- Model picker: free-tier & paid aliases merged into one list.
           Selected value is persisted in localStorage via the models store. -->
      <v-menu location="top start" offset="8">
        <template #activator="{ props: menuProps }">
          <button class="nest-model-btn" v-bind="menuProps" type="button">
            <v-icon size="14" class="mr-1">mdi-brain</v-icon>
            <span class="nest-mono">{{ selectedModel }}</span>
            <v-icon size="14" class="ml-1">mdi-chevron-up</v-icon>
          </button>
        </template>
        <v-list density="compact" class="nest-model-list">
          <v-list-item
            v-for="m in modelOptions"
            :key="m.id"
            :active="m.id === selectedModel"
            @click="models.select(m.id)"
          >
            <v-list-item-title class="nest-mono">{{ m.id }}</v-list-item-title>
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
  min-width: 200px;
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
