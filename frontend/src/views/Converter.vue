<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useModelsStore } from '@/stores/models'
import { byokApi, type BYOKKey } from '@/api/byok'
import { convertTheme, listJobs, retryJob, type ConvertJob, type ConvertResponse } from '@/api/convert'
import { ApiError } from '@/api/client'
import { useAppearanceStore } from '@/stores/appearance'
import { fromST, type STTheme } from '@/api/appearance'

// /convert — admin surface. Theme-converter: ST theme.json → WuNest
// theme.json via the user's chosen LLM. Entire feature is opt-in —
// users pay their own tokens (BYOK or WuApi pool), and server stores
// each conversion for 24h so the URL can be shared.
//
// `.nest-admin` on root so aggressive user themes in scope=global
// can't wipe the form (see M43 admin-isolation in cssScope.ts).

const { t } = useI18n()
const router = useRouter()

// Source picker: WuApi pool OR one of user's BYOK keys. Default to
// wuapi so a brand-new user without a BYOK sees a working flow
// immediately — they spend their WuApi quota instead of being
// gated on setting up a key first.
type Source = { kind: 'wuapi' } | { kind: 'byok'; id: string }
const byokList = ref<BYOKKey[]>([])
const byokLoading = ref(false)
const source = ref<Source>({ kind: 'wuapi' })

// Models store does the "fetch catalogue per source, remember last
// pick" dance for us — re-used from Chat view. Switching source
// re-fetches and re-selects a remembered model.
const models = useModelsStore()
const { items: modelItems, selected: selectedModel, loading: modelsLoading } = storeToRefs(models)

onMounted(async () => {
  // Fetch BYOK list — the source picker needs it.
  byokLoading.value = true
  try {
    const res = await byokApi.list()
    byokList.value = res.items
  } catch {
    byokList.value = []
  } finally {
    byokLoading.value = false
  }
  await onSourceChange()
})

async function onSourceChange() {
  const s = source.value
  if (s.kind === 'wuapi') {
    await models.setActiveSource('wuapi')
  } else {
    await models.setActiveSource({ byokID: s.id })
  }
}

// v-radio-group emits `string | null`. We treat null as "keep current"
// (can happen during transient re-render) and otherwise parse the
// encoded value (e.g. "byok:<uuid>") into our Source union.
async function onSourcePick(v: string | null) {
  if (v == null) return
  if (v === 'wuapi') {
    source.value = { kind: 'wuapi' }
  } else if (v.startsWith('byok:')) {
    source.value = { kind: 'byok', id: v.slice(5) }
  }
  await onSourceChange()
}

// ── Input mode (M51 Sprint 2 wave 2) ──────────────────────────────
// Two ways to feed the converter: file upload (original) or paste-text
// (new). Files now accept .json AND .css — bare CSS gets wrapped into
// the ST envelope `{name, custom_css}` on submit. Paste-text auto-
// detects: leading `{` ⇒ JSON, anything else ⇒ CSS.
type InputMode = 'file' | 'paste'
const inputMode = ref<InputMode>('file')

// ── File upload state ─────────────────────────────────────────────
const fileInput = ref<HTMLInputElement | null>(null)
const selectedFile = ref<File | null>(null)
const dragOver = ref(false)

// ── Paste-text state ──────────────────────────────────────────────
const pasteText = ref('')

// Display bytes as KB with one decimal — themes are always well
// under 1MB so no MB/GB logic needed.
function formatKB(bytes: number): string {
  return (bytes / 1024).toFixed(1) + ' KB'
}

const MAX_BYTES = 500 * 1024
const MAX_KB_LABEL = '500 KB'

// Validation surfaces inline under the input — runs over either the
// file or the paste-text depending on active mode.
const inputError = computed<string | null>(() => {
  if (inputMode.value === 'file') {
    if (!selectedFile.value) return null
    if (selectedFile.value.size > MAX_BYTES) return t('converter.errors.tooLarge', { max: MAX_KB_LABEL })
    if (!/\.(json|css)$/i.test(selectedFile.value.name)) return t('converter.errors.notJsonOrCss')
    return null
  }
  // Paste mode.
  const txt = pasteText.value
  if (!txt.trim()) return null
  // Encode UTF-8 → use Blob to get accurate byte length.
  const sizeBytes = new TextEncoder().encode(txt).length
  if (sizeBytes > MAX_BYTES) return t('converter.errors.tooLarge', { max: MAX_KB_LABEL })
  // Looks like JSON but malformed? Save the user a 30-180s LLM call.
  const trimmed = txt.trimStart()
  if (trimmed.startsWith('{')) {
    try { JSON.parse(txt) } catch {
      return t('converter.errors.malformedJson')
    }
  }
  return null
})

// Whether we have something to convert at all.
const hasInput = computed<boolean>(() => {
  return inputMode.value === 'file' ? !!selectedFile.value : pasteText.value.trim().length > 0
})

function pickFile() { fileInput.value?.click() }
function onFileChange(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0]
  if (f) selectedFile.value = f
}
function onDrop(e: DragEvent) {
  e.preventDefault()
  dragOver.value = false
  const f = e.dataTransfer?.files?.[0]
  if (f) selectedFile.value = f
}

// ── Build the request payload ─────────────────────────────────────
// Centralised in one place so both submit-modes (file/paste) and the
// auto-wrap-CSS logic agree on what gets sent. Returns `{blob, filename}`
// plus `null` when there's nothing to send (no file / empty paste).
//
// Behaviour:
//   - File `.json` → as-is
//   - File `.css`  → wrapped: `{name: <stem>, custom_css: <bytes>}`
//   - Paste JSON   → as-is
//   - Paste CSS    → wrapped: `{name: 'Untitled CSS', custom_css: <text>}`
async function buildPayload(): Promise<{ blob: Blob; filename: string } | null> {
  if (inputMode.value === 'file') {
    const f = selectedFile.value
    if (!f) return null
    if (/\.json$/i.test(f.name)) {
      return { blob: f, filename: f.name }
    }
    if (/\.css$/i.test(f.name)) {
      const css = await f.text()
      const stem = f.name.replace(/\.css$/i, '')
      const envelope = JSON.stringify({ name: stem, custom_css: css })
      return {
        blob: new Blob([envelope], { type: 'application/json' }),
        filename: stem + '.json',
      }
    }
    // Should be unreachable thanks to inputError validation, but
    // guard anyway — return as-is and let backend reject.
    return { blob: f, filename: f.name }
  }

  // Paste mode.
  const txt = pasteText.value.trim()
  if (!txt) return null
  const looksLikeJson = txt.startsWith('{')
  if (looksLikeJson) {
    return {
      blob: new Blob([txt], { type: 'application/json' }),
      filename: 'pasted.json',
    }
  }
  const envelope = JSON.stringify({ name: 'Untitled CSS', custom_css: txt })
  return {
    blob: new Blob([envelope], { type: 'application/json' }),
    filename: 'pasted.json',
  }
}

// ── Convert action ────────────────────────────────────────────────
const converting = ref(false)
const convertError = ref<string | null>(null)
const rateLimitUntil = ref<Date | null>(null)
const result = ref<ConvertResponse | null>(null)

async function doConvert() {
  if (!hasInput.value || inputError.value || !selectedModel.value) return
  const payload = await buildPayload()
  if (!payload) return
  converting.value = true
  convertError.value = null
  rateLimitUntil.value = null
  result.value = null
  try {
    const byokId = source.value.kind === 'byok' ? source.value.id : undefined
    const res = await convertTheme({
      blob: payload.blob,
      filename: payload.filename,
      model: selectedModel.value,
      byokId,
    })
    result.value = res
    // Kick a refresh of the recent-jobs list so the chip strip updates.
    await fetchRecent()
  } catch (err) {
    handleConvertError(err)
  } finally {
    converting.value = false
  }
}

// Shared error mapping used by both convert and retry. Kept here
// (not in convert.ts) because the surfaced message depends on i18n.
function handleConvertError(err: unknown) {
  if (err instanceof ApiError) {
    if (err.status === 429) {
      const [, when] = err.message.split('|')
      if (when) rateLimitUntil.value = new Date(when)
      convertError.value = t('converter.errors.rateLimited')
    } else if (err.status === 410) {
      // M51 — retry handler returns 410 for legacy rows / expired source.
      convertError.value = t('converter.errors.retryGone')
    } else if (err.status === 502) {
      convertError.value = t('converter.errors.modelFailed', { detail: err.message })
    } else {
      convertError.value = err.message || t('converter.errors.unknown')
    }
  } else {
    convertError.value = (err as Error).message || t('converter.errors.unknown')
  }
}

// ── Retry flow (M51 Sprint 2 wave 2) ──────────────────────────────
// Re-runs an existing job's input through a different model. Two
// entry points: «Попробовать другую модель» on the result panel
// after a fresh conversion, OR the refresh-icon on each history row.
// Both surface the same picker dialog and hit POST /api/convert/{id}/retry.
const retryDialogOpen = ref(false)
const retryJobId = ref<string | null>(null)
const retryModel = ref<string>('')

// Convert a job-row source state into our Source union for prefilling
// the model picker. For BYOK entries we just use the stored id; if
// that key was deleted since the job ran the picker will harmlessly
// fall back to "no key" (loading state on the model store).
function openRetryDialog(job: ConvertJob | ConvertResponse['job']) {
  retryJobId.value = job.id
  retryModel.value = job.model
  // Pre-select source matching the source job (best-effort — user can
  // change in the dialog).
  if (job.byok_id) {
    source.value = { kind: 'byok', id: job.byok_id }
  } else {
    source.value = { kind: 'wuapi' }
  }
  void onSourceChange()
  retryDialogOpen.value = true
}

async function doRetry() {
  if (!retryJobId.value || !selectedModel.value) return
  converting.value = true
  convertError.value = null
  rateLimitUntil.value = null
  retryDialogOpen.value = false
  try {
    const byokId = source.value.kind === 'byok' ? source.value.id : undefined
    const res = await retryJob(retryJobId.value, selectedModel.value, byokId)
    result.value = res
    await fetchRecent()
    // Scroll back to result so the new outcome is visible.
    if (typeof window !== 'undefined') {
      const el = document.querySelector('.nest-converter-result')
      el?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }
  } catch (err) {
    handleConvertError(err)
  } finally {
    converting.value = false
  }
}

// ── Recent jobs strip (last 24h) ──────────────────────────────────
const recentJobs = ref<ConvertJob[]>([])
async function fetchRecent() {
  try {
    const r = await listJobs()
    recentJobs.value = r.items
  } catch {
    /* non-fatal */
  }
}
onMounted(() => { fetchRecent() })

function jobAgeLabel(iso: string): string {
  const ms = Date.now() - new Date(iso).getTime()
  const hrs = Math.floor(ms / 3_600_000)
  const mins = Math.floor((ms % 3_600_000) / 60_000)
  if (hrs > 0) return `${hrs}h ${mins}m`
  return `${mins}m`
}

// ── Apply result directly to my appearance ────────────────────────
// After a successful conversion, user can either download the JSON
// (share with friends) or apply it straight into their own appearance.
// "Apply" path runs the result through fromST() just like manual
// import does — same entry point, no divergence.
const appearance = useAppearanceStore()
const applying = ref(false)
const applied = ref(false)

// M51 Sprint 1 wave 3 — runtime guard before applying converter output.
// Previously `result.value.output` was just cast to `STTheme` and fed to
// fromST(), which silently dropped wrong types. M43.1 caught a regression
// where `avatar_style: "1"` (string!) made it through and broke avatars.
// The check is intentionally lenient: any field MAY be present, MAY be
// undefined, but if present it must be the right shape.
function isPlausibleSTTheme(o: unknown): o is STTheme {
  if (!o || typeof o !== 'object') return false
  const t = o as Record<string, unknown>
  const stringFields = [
    'name', 'main_text_color', 'italics_text_color', 'quote_text_color',
    'border_color', 'blur_tint_color', 'custom_css',
  ]
  const numberFields = ['font_scale', 'chat_width', 'avatar_style', 'chat_display', 'blur_strength']
  const boolFields = ['noShadows', 'reduced_motion']
  for (const f of stringFields) if (t[f] !== undefined && typeof t[f] !== 'string') return false
  for (const f of numberFields) if (t[f] !== undefined && typeof t[f] !== 'number') return false
  for (const f of boolFields) if (t[f] !== undefined && typeof t[f] !== 'boolean') return false
  return true
}

const validationError = ref<string | null>(null)

function applyToMe() {
  if (!result.value) return
  validationError.value = null
  if (!isPlausibleSTTheme(result.value.output)) {
    validationError.value = t('converter.result.validationFailed')
    return
  }
  applying.value = true
  try {
    const theme = result.value.output as STTheme
    const mapped = fromST(theme)
    mapped.customCssScope = 'chat'
    appearance.update(mapped)  // sync — debounced save inside
    applied.value = true
    setTimeout(() => (applied.value = false), 2000)
  } finally {
    applying.value = false
  }
}

// ── Preview helpers ───────────────────────────────────────────────
// First N lines of the generated `custom_css`, so the user can sanity-
// check what's about to be applied without downloading the file. Cheap
// to compute, capped to keep the panel small.
const CSS_PREVIEW_LINES = 30
const cssPreview = computed<string>(() => {
  const css = (result.value?.output as STTheme | undefined)?.custom_css
  if (!css) return ''
  const lines = css.split('\n')
  if (lines.length <= CSS_PREVIEW_LINES) return css
  return lines.slice(0, CSS_PREVIEW_LINES).join('\n') + '\n…'
})

// Count of WuNest-style selectors (`.nest-*`) in the generated CSS —
// rough proxy for "did the converter actually do its job rewriting ST
// selectors". A theme conversion that produces 0 nest-* selectors is
// suspicious; the user-facing line says "we rewrote N selectors" so the
// user can tell at a glance the LLM didn't just echo back the input.
const selectorRewriteCount = computed<number>(() => {
  const css = (result.value?.output as STTheme | undefined)?.custom_css
  if (!css) return 0
  // Count distinct `.nest-…` occurrences (selectors only — non-greedy
  // word-boundary capture). Includes `.nest-msg`, `.nest-msg-body`, etc.
  // Each occurrence is +1; CSS authors duplicating the same selector
  // for cascade reasons get counted multiple times — that's fine, it
  // still reflects effort.
  const matches = css.match(/\.nest-[a-z0-9-]+/g)
  return matches ? matches.length : 0
})

// ── Download as file ──────────────────────────────────────────────
function downloadResult() {
  if (!result.value) return
  // Server already has a dedicated /download endpoint that sets
  // Content-Disposition — just navigate to it and the browser does
  // the rest. Same-origin so credentials are preserved.
  window.location.href = result.value.download_url
}

// ── Source option label ───────────────────────────────────────────
function byokLabel(k: BYOKKey): string {
  return (k.label || k.provider) + ' · ' + k.masked
}

// Nice label for the active source shown as secondary text.
const sourceLabel = computed(() => {
  const s = source.value
  if (s.kind === 'wuapi') return t('converter.source.wuapi')
  const k = byokList.value.find(x => x.id === s.id)
  return k ? byokLabel(k) : '—'
})

function goHome() { router.push('/') }

// Pull the converter notes out of the result payload with a clear
// typed accessor. The LLM emits `_converter_notes: string[]` in the
// output per our prompt spec — but we tolerate absence and wrong
// types rather than blowing up the UI if the model diverged.
interface ConvertOutputShape { _converter_notes?: string[] }
const converterNotes = computed<string[]>(() => {
  const out = result.value?.output as ConvertOutputShape | undefined
  return Array.isArray(out?._converter_notes) ? out!._converter_notes! : []
})
</script>

<template>
  <v-container class="nest-converter nest-admin">
    <!-- Header -->
    <div class="nest-eyebrow">{{ t('converter.eyebrow') }}</div>
    <h1 class="nest-h1 mt-1">{{ t('converter.title') }}</h1>
    <p class="nest-subtitle mt-2 nest-converter-lead">
      {{ t('converter.lead') }}
    </p>

    <!-- COST / LIMITS disclosure banner. Required per spec: users MUST
         see that they pay the LLM cost themselves before clicking. -->
    <v-alert
      type="info"
      variant="tonal"
      density="compact"
      class="mt-4 nest-converter-disclosure"
    >
      <div>
        <strong>{{ t('converter.disclosure.title') }}</strong>
      </div>
      <ul class="nest-converter-disclosure-list">
        <li>{{ t('converter.disclosure.tokens') }}</li>
        <li>{{ t('converter.disclosure.rateLimit') }}</li>
        <li>{{ t('converter.disclosure.size', { max: MAX_KB_LABEL }) }}</li>
        <li>{{ t('converter.disclosure.storage') }}</li>
      </ul>
    </v-alert>

    <!-- ── Step 1. Source + model ─────────────────────────────── -->
    <section class="nest-section">
      <h2 class="nest-h2">{{ t('converter.step1.title') }}</h2>
      <p class="nest-subtitle mb-3">{{ t('converter.step1.tagline') }}</p>

      <!-- Source pick: WuApi pool vs one of user's BYOK keys. If user
           has no BYOK, we hide the BYOK group entirely so UI doesn't
           look broken. -->
      <v-radio-group
        :model-value="source.kind === 'wuapi' ? 'wuapi' : 'byok:' + source.id"
        hide-details
        density="compact"
        @update:model-value="onSourcePick"
      >
        <v-radio value="wuapi">
          <template #label>
            <span class="nest-mono">wuapi</span>
            <span class="nest-converter-hint">· {{ t('converter.source.wuapiHint') }}</span>
          </template>
        </v-radio>
        <v-radio
          v-for="k in byokList"
          :key="k.id"
          :value="'byok:' + k.id"
        >
          <template #label>
            <span class="nest-mono">byok</span>
            <span class="nest-converter-hint">· {{ byokLabel(k) }}</span>
          </template>
        </v-radio>
      </v-radio-group>

      <!-- Model picker — populated by the models store -->
      <v-select
        :model-value="selectedModel"
        :items="modelItems.map(m => m.id)"
        :loading="modelsLoading"
        :label="t('converter.step1.modelLabel')"
        density="compact"
        hide-details
        class="nest-converter-select mt-4"
        @update:model-value="(id: string) => models.select(id)"
      />
    </section>

    <!-- ── Step 2. Input (M51 Sprint 2 wave 2) ─────────────────────
         Two ways to feed the converter:
           • Файл — JSON or CSS file (CSS gets auto-wrapped on submit)
           • Вставить — paste raw text, auto-detect JSON vs CSS -->
    <section class="nest-section">
      <h2 class="nest-h2">{{ t('converter.step2.title') }}</h2>
      <p class="nest-subtitle mb-3">{{ t('converter.step2.tagline', { max: MAX_KB_LABEL }) }}</p>

      <v-tabs
        v-model="inputMode"
        density="compact"
        class="nest-converter-tabs mb-3"
        color="primary"
      >
        <v-tab value="file" prepend-icon="mdi-file-upload-outline">{{ t('converter.step2.tabFile') }}</v-tab>
        <v-tab value="paste" prepend-icon="mdi-text-box-edit-outline">{{ t('converter.step2.tabPaste') }}</v-tab>
      </v-tabs>

      <!-- File mode — accepts .json and .css; CSS gets wrapped on submit. -->
      <div v-if="inputMode === 'file'">
        <div
          class="nest-converter-dropzone"
          :class="{ 'is-dragging': dragOver, 'is-selected': !!selectedFile, 'is-invalid': !!inputError }"
          @dragover.prevent="dragOver = true"
          @dragleave.prevent="dragOver = false"
          @drop="onDrop"
          @click="pickFile"
        >
          <input
            ref="fileInput"
            type="file"
            accept="application/json,.json,.css,text/css"
            class="nest-converter-file-input"
            @change="onFileChange"
          />
          <template v-if="selectedFile">
            <v-icon size="28" :color="inputError ? 'error' : 'primary'">
              {{ inputError ? 'mdi-alert-circle-outline' : 'mdi-file-code-outline' }}
            </v-icon>
            <div class="nest-converter-filename nest-mono">{{ selectedFile.name }}</div>
            <div class="nest-converter-filesize">{{ formatKB(selectedFile.size) }}</div>
          </template>
          <template v-else>
            <v-icon size="28" color="surface-variant">mdi-cloud-upload-outline</v-icon>
            <div class="nest-converter-drop-label">{{ t('converter.step2.dropLabel') }}</div>
            <div class="nest-converter-drop-hint nest-mono">{{ t('converter.step2.dropHint', { max: MAX_KB_LABEL }) }}</div>
          </template>
        </div>
      </div>

      <!-- Paste mode — monospaced textarea, auto-detect JSON vs CSS. -->
      <div v-else>
        <v-textarea
          v-model="pasteText"
          :placeholder="t('converter.step2.pastePlaceholder')"
          :error="!!inputError"
          rows="14"
          variant="outlined"
          density="comfortable"
          class="nest-converter-paste"
          spellcheck="false"
          autocapitalize="off"
          autocorrect="off"
          hide-details="auto"
        />
        <p class="nest-converter-drop-hint nest-mono mt-1">
          {{ t('converter.step2.pasteHint', { max: MAX_KB_LABEL }) }}
        </p>
      </div>

      <p v-if="inputError" class="nest-hint text-error mt-2">{{ inputError }}</p>
    </section>

    <!-- ── Action ──────────────────────────────────────────────── -->
    <section class="nest-section">
      <div class="nest-converter-action">
        <v-btn
          color="primary"
          size="large"
          :loading="converting"
          :disabled="!hasInput || !!inputError || !selectedModel || converting"
          @click="doConvert"
        >
          <v-icon size="20" class="mr-2">mdi-auto-fix</v-icon>
          {{ t('converter.convertButton') }}
        </v-btn>
        <span v-if="!converting" class="nest-caption nest-converter-costhint nest-mono">
          {{ t('converter.costHint', { source: sourceLabel }) }}
        </span>
      </div>

      <!-- Error surfaces inline: 429 rate-limit, 502 LLM failure, other. -->
      <v-alert
        v-if="convertError"
        type="error"
        variant="tonal"
        density="compact"
        class="mt-4"
      >
        <div>{{ convertError }}</div>
        <div v-if="rateLimitUntil" class="nest-mono nest-hint--sm mt-1">
          {{ t('converter.errors.retryAt', { at: rateLimitUntil.toLocaleString() }) }}
        </div>
      </v-alert>
    </section>

    <!-- ── Result ──────────────────────────────────────────────── -->
    <section v-if="result" class="nest-section nest-converter-result">
      <h2 class="nest-h2">{{ t('converter.result.title') }}</h2>
      <p class="nest-subtitle mb-3">
        {{ t('converter.result.meta', {
          tokens_in: result.job.tokens_in,
          tokens_out: result.job.tokens_out,
          expires_at: new Date(result.job.expires_at).toLocaleString(),
        }) }}
      </p>

      <!-- Converter notes — a human-readable log of what the LLM
           changed ("Replaced .mes with .nest-msg in 12 places"). If
           absent, we just omit the list silently rather than showing
           an empty bullet wasteland. -->
      <ul v-if="converterNotes.length" class="nest-converter-notes">
        <li v-for="(note, i) in converterNotes" :key="i">{{ note }}</li>
      </ul>

      <!-- Preview block (M51 Sprint 1 wave 3). Two pieces of cheap
           insight before the user clicks Apply:
             - Selector-rewrite count: how many `.nest-*` selectors the
               converter actually produced. Zero = suspicious (model
               likely echoed the ST input back).
             - CSS snippet: first 30 lines so the user can eyeball
               whether the model wrote sensible code or hallucinated.
           Cheap to render — just two computeds and a <pre>. -->
      <div class="nest-converter-preview mt-3">
        <div class="nest-eyebrow">{{ t('converter.result.previewTitle') }}</div>
        <div class="nest-converter-preview-stat nest-mono">
          {{ t('converter.result.previewSelectors', { count: selectorRewriteCount }) }}
        </div>
        <p v-if="!cssPreview" class="nest-subtitle nest-hint--sm mt-2">
          {{ t('converter.result.previewEmpty') }}
        </p>
        <template v-else>
          <p class="nest-subtitle nest-hint--sm mt-2 mb-1">
            {{ t('converter.result.previewSnippet', { n: 30 }) }}
          </p>
          <pre class="nest-converter-preview-code"><code>{{ cssPreview }}</code></pre>
        </template>
      </div>

      <!-- Validation alert (M51 Sprint 1 wave 3). Surfaces only when
           the runtime guard rejects an Apply attempt; the user can
           still Download and inspect the file. -->
      <v-alert
        v-if="validationError"
        type="warning"
        variant="tonal"
        density="compact"
        class="mt-3"
      >
        {{ validationError }}
      </v-alert>

      <div class="nest-converter-result-actions mt-3">
        <v-btn
          color="primary"
          variant="flat"
          @click="downloadResult"
        >
          <v-icon size="18" class="mr-2">mdi-download</v-icon>
          {{ t('converter.result.download') }}
        </v-btn>
        <v-btn
          variant="outlined"
          :loading="applying"
          @click="applyToMe"
        >
          <v-icon size="18" class="mr-2">mdi-palette-outline</v-icon>
          {{ applied ? t('converter.result.applied') : t('converter.result.apply') }}
        </v-btn>
        <!-- M51 Sprint 2 wave 2 — re-run with another model on the
             same input. Opens the picker dialog pre-filled with the
             current job's model + source. -->
        <v-btn
          variant="text"
          :disabled="converting"
          @click="openRetryDialog(result.job)"
        >
          <v-icon size="18" class="mr-2">mdi-refresh</v-icon>
          {{ t('converter.result.retry') }}
        </v-btn>
      </div>
    </section>

    <!-- ── Recent jobs ─────────────────────────────────────────── -->
    <section v-if="recentJobs.length" class="nest-section">
      <h2 class="nest-h2">{{ t('converter.recent.title') }}</h2>
      <p class="nest-subtitle mb-3">{{ t('converter.recent.tagline') }}</p>
      <div class="nest-converter-recent">
        <div v-for="j in recentJobs" :key="j.id" class="nest-converter-recent-row">
          <v-icon
            size="16"
            :color="j.status === 'done' ? 'success' : j.status === 'error' ? 'error' : 'warning'"
          >
            {{ j.status === 'done' ? 'mdi-check-circle' : j.status === 'error' ? 'mdi-alert-circle' : 'mdi-progress-clock' }}
          </v-icon>
          <div class="nest-converter-recent-meta">
            <span class="nest-mono">{{ j.model }}</span>
            <span class="nest-converter-recent-age">{{ jobAgeLabel(j.created_at) }} ago</span>
          </div>
          <a
            v-if="j.status === 'done'"
            :href="`/api/convert/${j.id}/download`"
            class="nest-converter-recent-download"
          >
            <v-icon size="16">mdi-download</v-icon>
          </a>
          <!-- M51 Sprint 2 wave 2 — retry button on each history row.
               Hidden for non-terminal states (running/pending) since
               there's nothing finished to compare against yet.
               Opens the model-picker dialog pre-filled with this row's
               model + source. -->
          <button
            v-if="j.status === 'done' || j.status === 'error'"
            type="button"
            class="nest-converter-recent-retry"
            :title="t('converter.result.retry')"
            @click="openRetryDialog(j)"
          >
            <v-icon size="16">mdi-refresh</v-icon>
          </button>
        </div>
      </div>
    </section>

    <!-- ── Retry dialog (M51 Sprint 2 wave 2) ──────────────────────
         Light-weight model-picker reuse. We don't duplicate the
         BYOK/wuapi radio set here — it's already on the page above,
         and openRetryDialog() pre-syncs `source` to the row's source.
         Dialog just shows the model dropdown for the active source
         and a confirm. -->
    <v-dialog v-model="retryDialogOpen" max-width="480" class="nest-admin">
      <v-card>
        <v-card-title>{{ t('converter.retry.title') }}</v-card-title>
        <v-card-text>
          <p class="nest-subtitle mb-3">{{ t('converter.retry.body') }}</p>
          <v-select
            :items="modelItems"
            :model-value="selectedModel"
            :loading="modelsLoading"
            density="compact"
            hide-details
            :label="t('converter.step1.modelLabel')"
            @update:model-value="(id: string) => models.select(id)"
          />
          <p class="nest-converter-hint nest-mono mt-2">
            {{ t('converter.retry.sourceHint', { source: sourceLabel }) }}
          </p>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="retryDialogOpen = false">{{ t('common.cancel') }}</v-btn>
          <v-btn
            color="primary"
            variant="flat"
            :disabled="!selectedModel || converting"
            :loading="converting"
            @click="doRetry"
          >
            {{ t('converter.retry.run') }}
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- ── Back to Settings ────────────────────────────────────── -->
    <section class="nest-section nest-converter-footer">
      <v-btn variant="text" size="small" @click="goHome">
        <v-icon size="16" class="mr-1">mdi-arrow-left</v-icon>
        {{ t('common.close') }}
      </v-btn>
    </section>
  </v-container>
</template>

<style lang="scss" scoped>
.nest-converter {
  max-width: 720px;
  padding: 32px 24px;
}
.nest-converter-lead {
  max-width: 640px;
}
.nest-converter-disclosure {
  border-left: 3px solid var(--nest-accent);
}
.nest-converter-disclosure-list {
  margin: 6px 0 0;
  padding-left: 18px;
  font-size: 13px;
  line-height: 1.5;
}

.nest-section {
  margin-top: 40px;
  padding-top: 24px;
  border-top: 1px solid var(--nest-border);
}
.nest-section:first-of-type {
  border-top: none;
  padding-top: 0;
  margin-top: 24px;
}

.nest-converter-hint {
  color: var(--nest-text-secondary);
  font-size: 13px;
  margin-left: 6px;
}
.nest-converter-select {
  max-width: 420px;
}

// ── Tabs (file / paste) ─────────────────────────────────────
// M51 Sprint 2 wave 2. Slim tabs above the input area. Inherit
// Vuetify styling, just nudge the underline color so it tracks
// the active preset's accent (Vuetify primary bridge).
.nest-converter-tabs {
  border-bottom: 1px solid var(--nest-border);
}

// ── Paste textarea ──────────────────────────────────────────
.nest-converter-paste {
  :deep(textarea) {
    font-family: var(--nest-font-mono);
    font-size: 12.5px;
    line-height: 1.5;
  }
}

// ── Retry icon in history rows ──────────────────────────────
.nest-converter-recent-retry {
  background: transparent;
  border: 1px solid transparent;
  color: var(--nest-text-muted);
  border-radius: var(--nest-radius-sm);
  padding: 4px 6px;
  cursor: pointer;
  transition: color var(--nest-transition-fast),
              border-color var(--nest-transition-fast),
              background var(--nest-transition-fast);
  &:hover {
    color: var(--nest-accent);
    border-color: var(--nest-border);
    background: var(--nest-bg-elevated);
  }
}

// ── Dropzone ─────────────────────────────────────────────────
.nest-converter-dropzone {
  border: 2px dashed var(--nest-border);
  border-radius: var(--nest-radius);
  padding: 32px 16px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  transition: border-color var(--nest-transition-fast), background var(--nest-transition-fast);
  &:hover {
    border-color: var(--nest-accent);
    background: var(--nest-bg-elevated);
  }
  &.is-dragging {
    border-color: var(--nest-accent);
    background: var(--nest-bg-elevated);
  }
  &.is-selected {
    border-style: solid;
  }
  &.is-invalid {
    border-color: rgb(var(--v-theme-error));
  }
}
.nest-converter-file-input {
  display: none;
}
.nest-converter-filename {
  font-size: 14px;
  color: var(--nest-text);
  word-break: break-all;
}
.nest-converter-filesize {
  font-size: 12px;
  color: var(--nest-text-muted);
}
.nest-converter-drop-label {
  font-size: 14px;
  color: var(--nest-text);
}
.nest-converter-drop-hint {
  font-size: 11px;
  color: var(--nest-text-muted);
  letter-spacing: 0.04em;
}

// ── Action row ──────────────────────────────────────────────
.nest-converter-action {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
}
.nest-converter-costhint {
  color: var(--nest-text-muted);
  font-size: 11px;
}

// ── Result ──────────────────────────────────────────────────
.nest-converter-result {
  // Subtle highlight that something new just appeared.
  border-top-color: var(--nest-accent);
}
.nest-converter-notes {
  list-style: disc inside;
  padding-left: 0;
  margin: 0 0 0 4px;
  font-size: 13px;
  color: var(--nest-text-secondary);
  line-height: 1.6;
  li { margin-bottom: 2px; }
}
.nest-converter-result-actions {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}

// ── Preview block (M51 Sprint 1 wave 3) ─────────────────────
// What the LLM produced, at a glance, before user clicks Apply.
.nest-converter-preview {
  margin-top: 16px;
  padding: 12px 14px;
  border: 1px dashed var(--nest-border);
  border-radius: var(--nest-radius-sm);
  background: var(--nest-bg-elevated);
}
.nest-converter-preview-stat {
  font-size: 13px;
  color: var(--nest-text-secondary);
  margin-top: 2px;
}
.nest-converter-preview-code {
  margin: 0;
  padding: 10px 12px;
  background: var(--nest-bg);
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
  font-family: var(--nest-font-mono);
  font-size: 11.5px;
  line-height: 1.45;
  color: var(--nest-text);
  // Tall enough to read 30 lines without scrolling, but capped so the
  // preview doesn't dominate the panel for verbose themes.
  max-height: 280px;
  overflow: auto;
  white-space: pre;
}

// ── Recent strip ────────────────────────────────────────────
.nest-converter-recent {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.nest-converter-recent-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 12px;
  border: 1px solid var(--nest-border-subtle);
  border-radius: 8px;
  background: var(--nest-surface);
  font-size: 13px;
}
.nest-converter-recent-meta {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
  span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
}
.nest-converter-recent-age {
  color: var(--nest-text-muted);
  font-size: 11px;
  letter-spacing: 0.04em;
}
.nest-converter-recent-download {
  color: var(--nest-text-secondary);
  &:hover { color: var(--nest-accent); }
}

// Footer back-link
.nest-converter-footer {
  border-top: 1px dashed var(--nest-border-subtle);
}

@media (max-width: 640px) {
  .nest-converter { padding: 24px 16px; }
  .nest-converter-action { flex-direction: column; align-items: stretch; }
  .nest-converter-select { max-width: 100%; }
}
</style>
