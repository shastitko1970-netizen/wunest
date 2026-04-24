<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useModelsStore } from '@/stores/models'
import { byokApi, type BYOKKey } from '@/api/byok'
import { convertTheme, listJobs, type ConvertJob, type ConvertResponse } from '@/api/convert'
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

// ── File upload state ─────────────────────────────────────────────
// Single-file uploader. Can't drop multiple themes; conversion is
// expensive (LLM tokens) so we want deliberate one-at-a-time flow.
const fileInput = ref<HTMLInputElement | null>(null)
const selectedFile = ref<File | null>(null)
const dragOver = ref(false)

// Display bytes as KB with one decimal — themes are always well
// under 1MB so no MB/GB logic needed.
function formatKB(bytes: number): string {
  return (bytes / 1024).toFixed(1) + ' KB'
}

const MAX_BYTES = 500 * 1024
const MAX_KB_LABEL = '500 KB'

const fileError = computed<string | null>(() => {
  if (!selectedFile.value) return null
  if (selectedFile.value.size > MAX_BYTES) return t('converter.errors.tooLarge', { max: MAX_KB_LABEL })
  if (!/\.json$/i.test(selectedFile.value.name)) return t('converter.errors.notJson')
  return null
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

// ── Convert action ────────────────────────────────────────────────
const converting = ref(false)
const convertError = ref<string | null>(null)
const rateLimitUntil = ref<Date | null>(null)
const result = ref<ConvertResponse | null>(null)

async function doConvert() {
  if (!selectedFile.value || fileError.value || !selectedModel.value) return
  converting.value = true
  convertError.value = null
  rateLimitUntil.value = null
  result.value = null
  try {
    const byokId = source.value.kind === 'byok' ? source.value.id : undefined
    const res = await convertTheme({
      file: selectedFile.value,
      model: selectedModel.value,
      byokId,
    })
    result.value = res
    // Kick a refresh of the recent-jobs list so the chip strip updates.
    await fetchRecent()
  } catch (err) {
    if (err instanceof ApiError) {
      if (err.status === 429) {
        // Custom "rate_limited|ISO8601" encoding from api/convert.ts
        const [, when] = err.message.split('|')
        if (when) rateLimitUntil.value = new Date(when)
        convertError.value = t('converter.errors.rateLimited')
      } else if (err.status === 502) {
        convertError.value = t('converter.errors.modelFailed', { detail: err.message })
      } else {
        convertError.value = err.message || t('converter.errors.unknown')
      }
    } else {
      convertError.value = (err as Error).message || t('converter.errors.unknown')
    }
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

function applyToMe() {
  if (!result.value) return
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

    <!-- ── Step 2. Upload ──────────────────────────────────────── -->
    <section class="nest-section">
      <h2 class="nest-h2">{{ t('converter.step2.title') }}</h2>
      <p class="nest-subtitle mb-3">{{ t('converter.step2.tagline', { max: MAX_KB_LABEL }) }}</p>

      <div
        class="nest-converter-dropzone"
        :class="{ 'is-dragging': dragOver, 'is-selected': !!selectedFile, 'is-invalid': !!fileError }"
        @dragover.prevent="dragOver = true"
        @dragleave.prevent="dragOver = false"
        @drop="onDrop"
        @click="pickFile"
      >
        <input
          ref="fileInput"
          type="file"
          accept="application/json,.json"
          class="nest-converter-file-input"
          @change="onFileChange"
        />
        <template v-if="selectedFile">
          <v-icon size="28" :color="fileError ? 'error' : 'primary'">
            {{ fileError ? 'mdi-alert-circle-outline' : 'mdi-file-code-outline' }}
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
      <p v-if="fileError" class="nest-hint text-error mt-2">{{ fileError }}</p>
    </section>

    <!-- ── Action ──────────────────────────────────────────────── -->
    <section class="nest-section">
      <div class="nest-converter-action">
        <v-btn
          color="primary"
          size="large"
          :loading="converting"
          :disabled="!selectedFile || !!fileError || !selectedModel || converting"
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
        </div>
      </div>
    </section>

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
