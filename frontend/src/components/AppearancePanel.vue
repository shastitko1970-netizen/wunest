<script setup lang="ts">
import { computed, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useAppearanceStore, resolveScope } from '@/stores/appearance'
import { useThemeStore, THEME_PRESETS, type ThemePreset } from '@/stores/theme'
import { fromST, toST, type AvatarStyle, type ChatDisplay, type STTheme } from '@/api/appearance'
import { uploadBackground } from '@/api/uploads'
import { auditDangerousSelectors, supportsCSSScope, validateCss } from '@/lib/cssScope'

// Detailed appearance controls. Lives as a section of /settings so users
// see everything in one place: theme presets, density, custom colors,
// background image, custom CSS, and SillyTavern theme import/export.
//
// All mutations route through the store's update() which debounces the
// server PUT, so dragging a slider doesn't hammer the backend.
const { t } = useI18n()
const store = useAppearanceStore()
const { appearance, saving } = storeToRefs(store)

// M42.2 — Theme preset picker. Surface the five built-in themes from the
// design-system kit as cards; on click the theme store pulls the preset's
// CSS chunk and swaps the live <style id="nest-theme"> tag. Custom CSS
// (below) layers on top.
const themeStore = useThemeStore()
const { currentId: currentThemeId, loading: themeLoading } = storeToRefs(themeStore)
async function pickTheme(id: ThemePreset) {
  await themeStore.apply(id)
}

// Bind sliders to writable refs so we can patch the store on change without
// fighting v-model two-way rules.
const fontScale = computed<number>({
  get: () => appearance.value.fontScale ?? 1,
  set: v => store.update({ fontScale: round2(v) }),
})
const chatWidth = computed<number>({
  get: () => appearance.value.chatWidth ?? 60,
  set: v => store.update({ chatWidth: Math.round(v) }),
})
const blurStrength = computed<number>({
  get: () => appearance.value.blurStrength ?? 0,
  set: v => store.update({ blurStrength: Math.round(v) }),
})
const accent = computed<string>({
  get: () => appearance.value.accent ?? '',
  set: v => store.update({ accent: v || undefined }),
})
const bgImage = computed<string>({
  get: () => appearance.value.bgImageUrl ?? '',
  set: v => store.update({ bgImageUrl: v || undefined }),
})
const avatarStyle = computed<AvatarStyle | ''>({
  get: () => appearance.value.avatarStyle ?? '',
  set: v => store.update({ avatarStyle: (v || undefined) as AvatarStyle | undefined }),
})
const chatDisplay = computed<ChatDisplay | ''>({
  get: () => appearance.value.chatDisplay ?? '',
  set: v => store.update({ chatDisplay: (v || undefined) as ChatDisplay | undefined }),
})
const shadows = computed<boolean>({
  get: () => appearance.value.shadows !== false,
  set: v => store.update({ shadows: v }),
})
const reducedMotion = computed<boolean>({
  get: () => appearance.value.reducedMotion === true,
  set: v => store.update({ reducedMotion: v }),
})
const customCss = computed<string>({
  get: () => appearance.value.customCss ?? '',
  set: v => store.update({ customCss: v || undefined }),
})
const htmlRendering = computed<boolean>({
  get: () => appearance.value.htmlRendering !== false,
  set: v => store.update({ htmlRendering: v }),
})

// 'chat' — CSS wrapped in @scope(#chat) so ST themes don't paint over
// settings/library/menu. 'global' — the whole UI inherits the CSS.
// Getter uses resolveScope() so pre-M26 users who already had CSS see
// 'global' selected (matches how it was applied before the field
// existed); fresh users see 'chat'. Writing the toggle always stores
// an explicit value.
const customCssScope = computed<'chat' | 'global'>({
  get: () => resolveScope(appearance.value),
  set: v => store.update({ customCssScope: v }),
})

// Tally of broad-element selectors ("body", "textarea", ...) so the
// "scope toggle" can warn users that flipping to 'global' would hit
// them. Recomputes on every CSS edit; cheap since themes are ≤500 lines.
const dangerousAudit = computed(() => auditDangerousSelectors(customCss.value))

// Live CSS syntax check — surfaces a single red-underline-style error
// under the editor if the parser rejects the input. Avoids the
// "I pasted this and nothing happened" silent fail mode. Debounce
// not needed — validateCss is ~0.2ms on 500-line stylesheets.
const cssValidationError = computed(() => validateCss(customCss.value))

// Theming guide disclosure — default closed so the Appearance page
// doesn't become a giant cheat-sheet scroll for users who just want
// to flip a toggle.
const guideOpen = ref(false)

// Custom CSS disclosure — default closed once the field has content so
// the long textarea doesn't dominate the page. Clicking the header
// summary expands the editor. Summary shows the detected theme name
// (pulled from the first /* Name: ... */ comment or a leading @title
// comment) plus line count for orientation.
const customCssOpen = ref(false)
const customCssSummary = computed(() => {
  const css = customCss.value.trim()
  if (!css) return { name: '', lines: 0, empty: true }
  const lines = css.split('\n').length

  // Try a few common "theme label" conventions for ST/DokWCS themes:
  //   /* Name: Purple Tavern */
  //   /* Theme: Purple Tavern */
  //   /*! Purple Tavern — v1 */   (banner comment style)
  //   /* Purple Tavern */
  const labelled = /\/\*\s*(?:name|theme|title)\s*:\s*([^\n*]{1,60}?)\s*\*\//i.exec(css)
  if (labelled?.[1]) return { name: labelled[1].trim(), lines, empty: false }

  const banner = /\/\*!\s*([^\n*]{3,60}?)\s*(?:[-—–]|\n|\*\/)/.exec(css)
  if (banner?.[1]) return { name: banner[1].trim(), lines, empty: false }

  const firstBlock = /\/\*\s*([^\n*]{3,60}?)\s*\*\//.exec(css)
  if (firstBlock?.[1] && !/\d{3,}/.test(firstBlock[1])) {
    // Reject obvious non-titles like date-y "2024 11 17".
    return { name: firstBlock[1].trim(), lines, empty: false }
  }

  return { name: '', lines, empty: false }
})

const fileInput = ref<HTMLInputElement | null>(null)
const importError = ref<string | null>(null)
const importNotice = ref<string | null>(null)

// ─── Background upload ────────────────────────────────────────────
// User can upload their own chat background image instead of supplying
// a URL. Upload flows through POST /api/uploads/background → MinIO,
// then we set bgImageUrl to the returned URL and let the existing
// debounced save handle the rest.
const bgFileInput = ref<HTMLInputElement | null>(null)
const bgUploading = ref(false)
const bgUploadError = ref<string | null>(null)

async function onBackgroundFilePicked(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  try {
    bgUploading.value = true
    bgUploadError.value = null
    const res = await uploadBackground(file)
    // Use the computed's setter → store.update(debounced) → server save.
    bgImage.value = res.url
  } catch (err: any) {
    bgUploadError.value = err?.message || String(err)
  } finally {
    bgUploading.value = false
    if (input) input.value = ''
  }
}

function pickBackgroundFile() {
  bgFileInput.value?.click()
}

async function onImportFile(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0]
  if (!f) return
  importError.value = null
  importNotice.value = null
  try {
    const text = await f.text()
    const parsed = JSON.parse(text) as STTheme
    const mapped = fromST(parsed)
    // ST themes regularly target `body`/`input`/`textarea` — in WuNest
    // those rules would paint over settings/library/menu. Default the
    // scope to 'chat' on every ST import so imported themes are safe
    // out of the box. Users can flip to 'global' from the scope toggle
    // if they want the classic paste-everywhere behaviour.
    mapped.customCssScope = 'chat'
    store.update(mapped)
    // If the theme contains broad selectors, show a one-liner so users
    // understand why some menu bits didn't change color.
    if (mapped.customCss) {
      const audit = auditDangerousSelectors(mapped.customCss)
      if (audit.length > 0) {
        importNotice.value = t('appearance.cssScope.importNotice', {
          count: audit.length,
        })
      }
    }
  } catch (err) {
    importError.value = (err as Error).message
  } finally {
    // Reset input so re-uploading the same file re-triggers the event.
    if (fileInput.value) fileInput.value.value = ''
  }
}

function downloadExport() {
  const blob = toST(appearance.value)
  const name = (blob.name || 'wunest-theme').replace(/[^a-z0-9-_]+/gi, '_')
  const data = new Blob([JSON.stringify(blob, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(data)
  const a = document.createElement('a')
  a.href = url
  a.download = `${name}.json`
  a.click()
  URL.revokeObjectURL(url)
}

// Export the raw custom CSS as a standalone .css file — useful when
// a user wants to share their theme on Discord / forums without the
// surrounding ST JSON envelope. Name derived from `/* Name: ... */`
// comment in the CSS itself when possible.
function downloadCssExport() {
  const css = appearance.value.customCss?.trim() ?? ''
  if (!css) return
  const match = css.match(/\/\*\s*(?:Name|@title)\s*:\s*(.+?)\s*\*\//i)
  const name = (match?.[1] ?? 'wunest-theme').replace(/[^a-z0-9-_]+/gi, '_')
  const data = new Blob([css], { type: 'text/css' })
  const url = URL.createObjectURL(data)
  const a = document.createElement('a')
  a.href = url
  a.download = `${name}.css`
  a.click()
  URL.revokeObjectURL(url)
}

// Raw .css file import — dropping a theme CSS file straight into the
// custom-CSS editor. Complements the ST JSON importer: a lot of
// community themes travel as bare .css snippets these days.
async function onImportCssFile(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0]
  if (!f) return
  importError.value = null
  importNotice.value = null
  try {
    const text = await f.text()
    if (!text.trim()) {
      throw new Error(t('appearance.customCssImportEmpty'))
    }
    // Default new raw-CSS imports to chat scope — same protection ST
    // imports get. Authors can flip to global from the toggle.
    store.update({ customCss: text, customCssScope: 'chat' })
    const audit = auditDangerousSelectors(text)
    if (audit.length > 0) {
      importNotice.value = t('appearance.cssScope.importNotice', {
        count: audit.length,
      })
    }
    customCssOpen.value = true
  } catch (err) {
    importError.value = (err as Error).message
  } finally {
    if (cssFileInput.value) cssFileInput.value.value = ''
  }
}

const cssFileInput = ref<HTMLInputElement | null>(null)
function pickCssFile() {
  cssFileInput.value?.click()
}

function resetAll() {
  store.reset()
}

function round2(n: number): number {
  return Math.round(n * 100) / 100
}

const savingHint = computed(() => saving.value ? t('appearance.savingHint') : '')
</script>

<template>
  <section class="nest-appearance">
    <div class="nest-app-head">
      <div>
        <div class="nest-eyebrow">{{ t('settings.appearanceTitle') }}</div>
        <h2 class="nest-h2 mt-1">{{ t('appearance.headline') }}</h2>
        <p class="nest-subtitle mt-1">{{ t('appearance.tagline') }}</p>
      </div>
      <div class="nest-app-head-right">
        <span v-if="savingHint" class="nest-mono nest-saving">{{ savingHint }}</span>
        <v-btn
          variant="text"
          size="small"
          prepend-icon="mdi-restore"
          @click="resetAll"
        >
          {{ t('appearance.reset') }}
        </v-btn>
      </div>
    </div>

    <!-- ─── Theme preset picker (M42.2) ─────────────────────────────────
         Five design-system-compliant themes. Each card swaps
         <style id="nest-theme"> atomically. Custom CSS (further down)
         layers on top, so users can start from "Cyber neon" and add
         their own tweaks without rebuilding the palette from scratch. -->
    <div class="nest-theme-section nest-md-hint">
      <div class="nest-eyebrow">{{ t('appearance.themes.eyebrow') }}</div>
      <p class="nest-subtitle mt-1 mb-3">{{ t('appearance.themes.tagline') }}</p>

      <div class="nest-theme-grid">
        <button
          v-for="p in THEME_PRESETS"
          :key="p.id"
          type="button"
          class="nest-theme-card"
          :class="{
            'is-active': currentThemeId === p.id,
            'is-loading': themeLoading && currentThemeId !== p.id,
            [`is-kind-${p.kind}`]: true,
          }"
          :disabled="themeLoading"
          @click="pickTheme(p.id)"
        >
          <!-- Mini-preview: accent stripe + background + two bubble chips.
               Uses CSS variables the theme defines, so the preview
               updates automatically if tokens change. -->
          <div class="nest-theme-preview" :data-theme-id="p.id">
            <div class="nest-theme-preview-bg" />
            <div class="nest-theme-preview-msg" />
            <div class="nest-theme-preview-msg user" />
            <div class="nest-theme-preview-accent" />
          </div>
          <div class="nest-theme-card-body">
            <div class="nest-theme-card-title">
              {{ p.label }}
              <span class="nest-mono nest-theme-kind">{{ p.kind }}</span>
            </div>
            <div class="nest-theme-card-desc">{{ p.description }}</div>
          </div>
          <v-icon
            v-if="currentThemeId === p.id"
            size="18"
            color="primary"
            class="nest-theme-check"
          >mdi-check-circle</v-icon>
        </button>
      </div>
    </div>

    <!-- Size / density -->
    <div class="nest-grid">
      <div class="nest-field">
        <label class="nest-field-label">
          {{ t('appearance.fontScale') }}
          <span class="nest-mono">×{{ (fontScale ?? 1).toFixed(2) }}</span>
        </label>
        <v-slider
          v-model="fontScale"
          :min="0.75"
          :max="1.4"
          :step="0.05"
          hide-details
          color="primary"
          thumb-label
        />
      </div>
      <div class="nest-field">
        <label class="nest-field-label">
          {{ t('appearance.chatWidth') }}
          <span class="nest-mono">{{ chatWidth ?? 60 }}%</span>
        </label>
        <v-slider
          v-model="chatWidth"
          :min="40"
          :max="100"
          :step="1"
          hide-details
          color="primary"
          thumb-label
        />
      </div>
    </div>

    <!-- Style choices -->
    <div class="nest-grid">
      <div class="nest-field">
        <label class="nest-field-label">{{ t('appearance.avatarStyle') }}</label>
        <v-btn-toggle
          v-model="avatarStyle"
          color="primary"
          mandatory="force"
          density="compact"
          variant="outlined"
        >
          <v-btn value="round">{{ t('appearance.avatar.round') }}</v-btn>
          <v-btn value="square">{{ t('appearance.avatar.square') }}</v-btn>
          <v-btn value="portrait">{{ t('appearance.avatar.portrait') }}</v-btn>
        </v-btn-toggle>
        <div class="nest-hint">{{ t('appearance.avatar.hint') }}</div>
      </div>
      <div class="nest-field">
        <label class="nest-field-label">{{ t('appearance.chatDisplay') }}</label>
        <v-btn-toggle
          v-model="chatDisplay"
          color="primary"
          mandatory="force"
          density="compact"
          variant="outlined"
        >
          <v-btn value="bubbles">{{ t('appearance.display.bubbles') }}</v-btn>
          <v-btn value="flat">{{ t('appearance.display.flat') }}</v-btn>
          <v-btn value="document">{{ t('appearance.display.document') }}</v-btn>
        </v-btn-toggle>
      </div>
    </div>

    <!-- Colors -->
    <div class="nest-grid">
      <div class="nest-field">
        <label class="nest-field-label">{{ t('appearance.accent') }}</label>
        <div class="d-flex align-center ga-2">
          <input
            type="color"
            class="nest-color-swatch"
            :value="accent || '#ef4444'"
            @input="e => accent = (e.target as HTMLInputElement).value"
          />
          <v-text-field
            v-model="accent"
            placeholder="#ef4444"
            density="compact"
            hide-details
            style="flex: 1"
          />
        </div>
      </div>
      <div class="nest-field">
        <label class="nest-field-label">{{ t('appearance.bgImage') }}</label>
        <div class="nest-bg-row">
          <v-text-field
            v-model="bgImage"
            :placeholder="t('appearance.bgImagePlaceholder')"
            density="compact"
            hide-details
            style="flex: 1"
          />
          <input
            ref="bgFileInput"
            type="file"
            accept="image/*"
            style="display:none"
            @change="onBackgroundFilePicked"
          />
          <v-btn
            size="small"
            variant="tonal"
            prepend-icon="mdi-upload"
            :loading="bgUploading"
            @click="pickBackgroundFile"
          >
            {{ t('appearance.bgUpload') }}
          </v-btn>
          <v-btn
            v-if="bgImage"
            size="small"
            variant="text"
            color="error"
            icon="mdi-close"
            :title="t('appearance.bgClear')"
            @click="bgImage = ''"
          />
        </div>
        <v-alert
          v-if="bgUploadError"
          type="error"
          density="compact"
          variant="tonal"
          closable
          class="mt-2"
          @click:close="bgUploadError = null"
        >
          {{ bgUploadError }}
        </v-alert>
      </div>
    </div>

    <!-- Blur (only meaningful with bg image) -->
    <div v-if="bgImage" class="nest-field">
      <label class="nest-field-label">
        {{ t('appearance.blur') }}
        <span class="nest-mono">{{ blurStrength ?? 0 }}px</span>
      </label>
      <v-slider
        v-model="blurStrength"
        :min="0"
        :max="20"
        :step="1"
        hide-details
        color="primary"
        thumb-label
      />
    </div>

    <!-- Toggles -->
    <div class="nest-field-row">
      <v-switch
        v-model="shadows"
        :label="t('appearance.shadows')"
        hide-details
        color="primary"
        inset
      />
      <v-switch
        v-model="reducedMotion"
        :label="t('appearance.reducedMotion')"
        hide-details
        color="primary"
        inset
      />
      <v-switch
        v-model="htmlRendering"
        :label="t('appearance.htmlRendering')"
        hide-details
        color="primary"
        inset
      />
    </div>
    <p class="nest-subtitle nest-html-hint">{{ t('appearance.htmlRenderingHint') }}</p>

    <!-- Custom CSS — collapsed by default, summary shows theme name + line count -->
    <div class="nest-field nest-css-block">
      <button
        class="nest-css-header"
        :aria-expanded="customCssOpen"
        @click="customCssOpen = !customCssOpen"
      >
        <v-icon size="18" class="nest-css-caret">
          {{ customCssOpen ? 'mdi-chevron-down' : 'mdi-chevron-right' }}
        </v-icon>
        <span class="nest-css-header-title">{{ t('appearance.customCss') }}</span>
        <span v-if="customCssSummary.empty" class="nest-css-summary-empty">
          {{ t('appearance.customCssEmpty') }}
        </span>
        <template v-else>
          <span v-if="customCssSummary.name" class="nest-css-summary-name">
            {{ customCssSummary.name }}
          </span>
          <span class="nest-css-summary-lines nest-mono">
            {{ t('appearance.customCssLines', { n: customCssSummary.lines }) }}
          </span>
        </template>
        <span class="nest-css-hint nest-mono text-medium-emphasis">
          {{ t('appearance.customCssHint') }}
        </span>
      </button>
      <v-textarea
        v-show="customCssOpen"
        v-model="customCss"
        :placeholder="t('appearance.customCssPlaceholder')"
        rows="8"
        auto-grow
        density="compact"
        hide-details
        variant="outlined"
        class="nest-mono-textarea mt-2"
        spellcheck="false"
        wrap="off"
      />
      <!-- CSS parse-error feedback (M42.4). Shows a red plate under the
           editor whenever the browser's CSS parser chokes on user input.
           Silent-accept was the old default — users pasted broken themes
           and saw no apparent change, never discovering why. -->
      <v-alert
        v-if="customCssOpen && cssValidationError"
        type="error"
        variant="tonal"
        density="compact"
        class="mt-2 nest-hint"
      >
        {{ t('appearance.customCssParseError') }}: {{ cssValidationError.message }}
      </v-alert>

      <!-- Scope picker — appears once there's CSS to scope. Default is
           chat-only. -->
      <div v-if="customCss && customCssOpen" class="nest-css-scope mt-3">
        <div class="nest-eyebrow mb-2">{{ t('appearance.cssScope.label') }}</div>
        <v-btn-toggle
          :model-value="customCssScope"
          color="primary"
          density="compact"
          variant="outlined"
          mandatory="force"
          @update:model-value="(v: string) => customCssScope = (v as 'chat' | 'global')"
        >
          <v-btn value="chat">
            <v-icon size="16" class="mr-1">mdi-message-text-outline</v-icon>
            {{ t('appearance.cssScope.chat') }}
          </v-btn>
          <v-btn value="global">
            <v-icon size="16" class="mr-1">mdi-application-outline</v-icon>
            {{ t('appearance.cssScope.global') }}
          </v-btn>
        </v-btn-toggle>
        <p class="nest-hint nest-hint--sm mt-2">
          {{ customCssScope === 'chat'
            ? t('appearance.cssScope.chatHint')
            : t('appearance.cssScope.globalHint') }}
        </p>

        <!-- If user has picked 'global' AND the CSS contains broad
             element selectors, warn that it'll repaint the whole app. -->
        <v-alert
          v-if="customCssScope === 'global' && dangerousAudit.length"
          type="warning"
          variant="tonal"
          density="compact"
          class="mt-2 nest-hint"
        >
          <div>
            {{ t('appearance.cssScope.globalWarning') }}
          </div>
          <div class="mt-1 nest-mono nest-hint--xs nest-audit-selectors">
            {{ dangerousAudit.slice(0, 6).map(a => a.selector).join(', ') }}{{ dangerousAudit.length > 6 ? '…' : '' }}
          </div>
        </v-alert>

        <!-- Browser can't scope natively AND we're in 'chat' mode —
             we'll fall back to manual prefixing. Inform the user; the
             fallback works but uses simpler parsing than native @scope. -->
        <v-alert
          v-if="customCssScope === 'chat' && !supportsCSSScope"
          type="info"
          variant="tonal"
          density="compact"
          class="mt-2 nest-hint"
        >
          {{ t('appearance.cssScope.scopeFallback') }}
        </v-alert>
      </div>

      <!-- Theming guide — collapsible cheat sheet of WuNest-native
           classes + ST aliases + copy-paste snippets. Inline because
           users are mid-edit when they want to look up a selector. -->
      <details class="nest-theme-guide mt-3" :open="guideOpen" @toggle="guideOpen = ($event.target as HTMLDetailsElement).open">
        <summary class="nest-theme-guide-summary">
          <v-icon size="16" class="mr-1">mdi-book-open-variant</v-icon>
          <span>{{ t('appearance.guide.title') }}</span>
        </summary>
        <div class="nest-theme-guide-body">
          <p class="nest-subtitle">{{ t('appearance.guide.intro') }}</p>

          <h4 class="nest-h4 mt-3">{{ t('appearance.guide.varsTitle') }}</h4>
          <p class="nest-hint nest-hint--md">{{ t('appearance.guide.varsIntro') }}</p>
          <pre class="nest-guide-snippet">:root {
  --SmartThemeBodyColor: #f0f0f0;
  --SmartThemeBorderColor: #3c1e50;
  --SmartThemeQuoteColor: #ef4444;    /* accent */
  --SmartThemeBlurTintColor: #0b0818;  /* main bg */
  --SmartThemeChatTintColor: #130d15;  /* panels */
  --SmartThemeBodyFont: 'Andika';
}</pre>

          <h4 class="nest-h4 mt-3">{{ t('appearance.guide.classesTitle') }}</h4>
          <p class="nest-hint nest-hint--md">{{ t('appearance.guide.classesIntro') }}</p>
          <ul class="nest-guide-list nest-mono">
            <li><code>.nest-msg</code> / <code>.mes</code> — <span>{{ t('appearance.guide.selMsg') }}</span></li>
            <li><code>.nest-msg-body</code> / <code>.mes_block</code> — <span>{{ t('appearance.guide.selMsgBody') }}</span></li>
            <li><code>.nest-msg-content</code> / <code>.mes_text</code> — <span>{{ t('appearance.guide.selMsgText') }}</span></li>
            <li><code>.nest-msg-name</code> / <code>.mes_name</code> — <span>{{ t('appearance.guide.selName') }}</span></li>
            <li><code>#chat</code> — <span>{{ t('appearance.guide.selChat') }}</span></li>
            <li><code>#send_form</code>, <code>#send_textarea</code>, <code>#send_but</code> — <span>{{ t('appearance.guide.selComposer') }}</span></li>
            <li><code>#top-bar</code>, <code>.topbar</code> — <span>{{ t('appearance.guide.selTop') }}</span></li>
          </ul>

          <h4 class="nest-h4 mt-3">{{ t('appearance.guide.exampleTitle') }}</h4>
          <p class="nest-hint nest-hint--md">{{ t('appearance.guide.exampleIntro') }}</p>
          <pre class="nest-guide-snippet">/* Message bubble with soft glow */
.mes {
  background: rgba(30, 15, 45, 0.65);
  border: 1px solid #3c1e50;
  border-radius: 14px;
  box-shadow: 0 2px 10px rgba(120, 60, 200, 0.15);
}

/* Accent for your name line */
.mes_name { color: #c485ff; letter-spacing: 0.02em; }

/* Your typed message input */
#send_textarea { color: #f1d1ff; }</pre>

          <p class="nest-hint mt-3">
            {{ t('appearance.guide.stImportNote') }}
          </p>
        </div>
      </details>
    </div>

    <!-- ── Import & Export (renamed from "SillyTavern compatibility") ──
         M46: re-shaped to stop implying «ST themes just work». Big
         warning banner up front + explicit CTA к Конвертеру + ссылка
         на doc-страничку для WuNest-нативных тем. Кнопки ниже без
         промиса ST-compat в label'ах. -->
    <div class="nest-field nest-io">
      <label class="nest-field-label">{{ t('appearance.io.title') }}</label>

      <!-- ST WARNING — самое заметное что на странице -->
      <div class="nest-st-warning">
        <div class="nest-st-warning-headline">
          <v-icon size="22" color="warning" class="mr-2">mdi-alert-octagram</v-icon>
          <span>{{ t('appearance.io.stWarning.title') }}</span>
        </div>
        <p class="nest-st-warning-body">
          {{ t('appearance.io.stWarning.body') }}
        </p>
        <div class="nest-st-warning-ctas">
          <v-btn
            color="primary"
            variant="flat"
            prepend-icon="mdi-auto-fix"
            size="small"
            :to="'/convert'"
          >
            {{ t('appearance.io.stWarning.convert') }}
          </v-btn>
          <v-btn
            variant="outlined"
            prepend-icon="mdi-book-open-variant"
            size="small"
            :to="'/docs/theming'"
          >
            {{ t('appearance.io.stWarning.howToWrite') }}
          </v-btn>
          <!-- «Галерея пресетов» (/themes) removed по просьбе тестера:
               кнопка обещала функцию которой user ожидал но /themes
               сейчас показывает только эстетический preview без
               actionable фичи. Вернём когда галерея будет install-able
               (M48 roadmap — theme marketplace). -->
        </div>
      </div>

      <p class="nest-subtitle mt-4">{{ t('appearance.io.hint') }}</p>
      <div class="d-flex ga-2 flex-wrap mt-2 nest-io-buttons">
        <v-btn
          variant="outlined"
          prepend-icon="mdi-upload"
          @click="fileInput?.click()"
        >
          {{ t('appearance.io.import') }}
        </v-btn>
        <input
          ref="fileInput"
          type="file"
          accept="application/json,.json"
          hidden
          @change="onImportFile"
        />
        <v-btn
          variant="outlined"
          prepend-icon="mdi-file-code-outline"
          @click="pickCssFile"
        >
          {{ t('appearance.io.importCss') }}
        </v-btn>
        <input
          ref="cssFileInput"
          type="file"
          accept="text/css,.css"
          hidden
          @change="onImportCssFile"
        />
        <v-btn
          variant="outlined"
          prepend-icon="mdi-download"
          @click="downloadExport"
        >
          {{ t('appearance.io.export') }}
        </v-btn>
        <v-btn
          v-if="customCss.trim()"
          variant="outlined"
          prepend-icon="mdi-download-outline"
          @click="downloadCssExport"
        >
          {{ t('appearance.io.exportCss') }}
        </v-btn>
      </div>
      <v-alert
        v-if="importError"
        type="error"
        variant="tonal"
        density="compact"
        class="mt-3"
      >
        {{ importError }}
      </v-alert>
      <v-alert
        v-if="importNotice"
        type="info"
        variant="tonal"
        density="compact"
        class="mt-3"
        closable
        @click:close="importNotice = null"
      >
        {{ importNotice }}
      </v-alert>
    </div>
  </section>
</template>

<style lang="scss" scoped>
.nest-appearance { display: flex; flex-direction: column; gap: 24px; }

// Audit-selector preview inside the css-scope warning card — was
// inline style="font-size: 10.5px; opacity: 0.85". Lifted into class
// so the muted look is consistent with the .nest-hint--xs utility
// plus its own opacity override.
.nest-audit-selectors { opacity: 0.85; }

// ─── Theme preset picker (M42.2) ───────────────────────────────────
// Grid of theme cards with live mini-previews. Each card is a button;
// :disabled while another theme is loading so two-finger tap doesn't
// start parallel loads. Active state is a tinted border + check icon.
.nest-theme-section {
  display: flex;
  flex-direction: column;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--nest-border-subtle);
}
.nest-theme-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(230px, 1fr));
  gap: 12px;
}
.nest-theme-card {
  all: unset;
  position: relative;
  display: flex;
  align-items: stretch;
  gap: 10px;
  padding: 10px;
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius);
  background: var(--nest-surface);
  cursor: pointer;
  transition:
    border-color var(--nest-transition-fast),
    transform var(--nest-transition-fast),
    box-shadow var(--nest-transition-fast);

  &:hover:not(:disabled):not(.is-active) {
    border-color: var(--nest-accent);
    transform: translateY(-1px);
    box-shadow: var(--nest-shadow-sm);
  }
  &.is-active {
    border-color: var(--nest-accent);
    box-shadow: 0 0 0 1px var(--nest-accent);
  }
  &:disabled { cursor: progress; opacity: 0.7; }
  &.is-loading { opacity: 0.5; }
}
.nest-theme-check {
  position: absolute;
  top: 6px;
  right: 6px;
}
.nest-theme-preview {
  position: relative;
  flex: 0 0 72px;
  aspect-ratio: 1 / 1;
  border-radius: var(--nest-radius-sm);
  overflow: hidden;
  // Per-preview theme colours. Uses the `data-theme-id` attribute so we
  // can show a faithful mini of each preset even before the user picks
  // it — no live CSS var inheritance from global (which would show the
  // SAME palette for every card).
  &[data-theme-id="nest-default-dark"]    { background: #080808; --sw-accent: #ef4444; --sw-msg: #141414; --sw-user: #2a1818; }
  &[data-theme-id="nest-default-light"]   { background: #fafaf7; --sw-accent: #ef4444; --sw-msg: #ffffff; --sw-user: #fff0f0; }
  &[data-theme-id="cyber-neon"]           { background: #0a0612; --sw-accent: #c485ff; --sw-msg: #150a20; --sw-user: #2a1740; }
  &[data-theme-id="minimal-reader"]       { background: #fbfaf5; --sw-accent: #6b5c4a; --sw-msg: transparent; --sw-user: #f0ece0; }
  &[data-theme-id="tavern-warm"]          { background: #1a0f07; --sw-accent: #e0a96d; --sw-msg: #2a1a10; --sw-user: #3a2418; }
}
.nest-theme-preview-bg {
  position: absolute; inset: 0;
}
.nest-theme-preview-msg {
  position: absolute;
  left: 8px; right: 32px;
  top: 16px;
  height: 10px;
  border-radius: 3px;
  background: var(--sw-msg);
  border: 1px solid rgba(255, 255, 255, 0.08);

  &.user {
    top: 34px;
    left: 20px; right: 8px;
    background: var(--sw-user);
  }
}
.nest-theme-preview-accent {
  position: absolute;
  bottom: 8px;
  left: 8px;
  width: 36px;
  height: 4px;
  border-radius: 2px;
  background: var(--sw-accent);
  opacity: 0.9;
}
.nest-theme-card-body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 3px;
}
.nest-theme-card-title {
  font-size: 13.5px;
  font-weight: 500;
  color: var(--nest-text);
  display: flex;
  align-items: baseline;
  gap: 8px;
}
.nest-theme-kind {
  font-size: 9.5px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--nest-text-muted);
  padding: 1px 5px;
  border-radius: var(--nest-radius-pill);
  background: var(--nest-bg-elevated);
}
.nest-theme-card-desc {
  font-size: 11px;
  color: var(--nest-text-secondary);
  line-height: 1.4;
}
@media (max-width: 640px) {
  .nest-theme-grid { grid-template-columns: 1fr; }
}

// Grid instead of flex+wrap+space-between. The old layout broke when the
// user cranked fontScale up: the left block grew taller and wider, `wrap`
// pushed the right block onto a new line, and `space-between` then
// parked it at the container's right edge — which on wide desktops is far
// from the text column the user was reading. "Reset" button looked like
// it had "teleported to Magadan".
//
// Grid pins the right block to the `auto` track regardless of left-block
// height. On mobile we collapse to a single column so the reset button
// still appears below the title instead of squeezing width.
.nest-app-head {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 16px;
  align-items: center;
}
.nest-app-head-right {
  display: flex;
  align-items: center;
  gap: 8px;
  justify-self: end;
}
@media (max-width: 560px) {
  .nest-app-head {
    grid-template-columns: 1fr;
  }
  .nest-app-head-right {
    justify-self: start;
  }
}

.nest-saving {
  font-size: 11px;
  color: var(--nest-text-muted);
  letter-spacing: 0.05em;
}

.nest-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 18px;
}
.nest-field { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
.nest-field-row {
  display: flex;
  flex-wrap: wrap;
  gap: 24px;
  padding: 4px 0;
}
.nest-bg-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
.nest-field-label {
  display: flex;
  justify-content: space-between;
  font-size: 12.5px;
  color: var(--nest-text-secondary);
  font-weight: 500;
}
.nest-hint {
  font-size: 11.5px;
  line-height: 1.4;
  color: var(--nest-text-muted);
  margin-top: 4px;
}

.nest-color-swatch {
  width: 38px;
  height: 38px;
  padding: 0;
  border: 1px solid var(--nest-border);
  border-radius: var(--nest-radius-sm);
  cursor: pointer;
  background: transparent;
}

.nest-mono-textarea :deep(textarea) {
  font-family: var(--nest-font-mono);
  font-size: 12.5px;
  line-height: 1.5;
  // Preserve whitespace + line breaks AS TYPED; scroll horizontally when a
  // line is too long rather than soft-wrapping mid-token. Makes pasted
  // ST theme CSS readable — default word-break chopped selectors like
  // `.mes_reasoning_header` mid-identifier, which is unreadable and hides
  // the actual rule boundaries.
  white-space: pre;
  overflow-x: auto;
  // Same for tabs so indentation survives paste.
  tab-size: 2;
}

.nest-css-block { gap: 0; }
.nest-css-header {
  all: unset;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
  background: var(--nest-bg-elevated);
  cursor: pointer;
  transition: border-color var(--nest-transition-fast), background var(--nest-transition-fast);
  flex-wrap: wrap;

  &:hover { border-color: var(--nest-border); }
}
.nest-css-caret { color: var(--nest-text-muted); flex-shrink: 0; }
.nest-css-header-title {
  font-size: 12.5px;
  color: var(--nest-text);
  font-weight: 500;
  flex-shrink: 0;
}
.nest-css-summary-name {
  font-size: 12px;
  color: var(--nest-text);
  padding: 2px 8px;
  // Token for the pill shape; was hardcoded 999px.
  border-radius: var(--nest-radius-pill);
  background: var(--nest-surface);
  border: 1px solid var(--nest-border-subtle);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 220px;
}
.nest-css-summary-lines {
  font-size: 10.5px;
  color: var(--nest-text-muted);
  letter-spacing: 0.04em;
}
.nest-css-summary-empty {
  font-size: 11.5px;
  color: var(--nest-text-muted);
  font-style: italic;
}
.nest-css-hint {
  margin-left: auto;
  font-size: 10px;
  letter-spacing: 0.05em;
}

@media (max-width: 520px) {
  .nest-css-hint { display: none; }
}

.nest-css-scope {
  padding: 10px 12px;
  border: 1px dashed var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
  background: var(--nest-bg-elevated);
}

.nest-theme-guide {
  border-top: 1px dashed var(--nest-border-subtle);
  padding-top: 10px;
}
.nest-theme-guide-summary {
  font-family: var(--nest-font-mono);
  font-size: 11px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--nest-text-muted);
  cursor: pointer;
  display: flex;
  align-items: center;
  padding: 4px 2px;
  list-style: none;
  &::-webkit-details-marker { display: none; }
}
details[open] > .nest-theme-guide-summary { color: var(--nest-text); }
.nest-theme-guide-body {
  padding: 8px 2px 4px;
}
.nest-guide-snippet {
  font-family: var(--nest-font-mono);
  font-size: 11.5px;
  line-height: 1.5;
  padding: 10px 12px;
  background: var(--nest-bg);
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
  overflow-x: auto;
  color: var(--nest-text-secondary);
  white-space: pre;
  margin-top: 4px;
}
.nest-guide-list {
  margin: 6px 0 0;
  padding-left: 16px;
  line-height: 1.7;
  font-size: 11.5px;
  color: var(--nest-text-secondary);

  li { margin: 2px 0; }
  code {
    font-size: 11px;
    padding: 1px 4px;
    background: var(--nest-bg);
    border-radius: 3px;
    color: var(--nest-text);
  }
  li > span { opacity: 0.8; font-family: var(--nest-font-body); }
}

.nest-io {
  padding-top: 12px;
  border-top: 1px dashed var(--nest-border-subtle);
}

// M46 — ST-import warning banner. Prominent без быть пугающим —
// сплошная yellow-amber заливка с левой полосой + CTA-кнопками.
// Задача: тот кто открыл Import section и думает «сейчас залью свою
// ST-тему» СРАЗУ видит что так не сработает и ему надо пойти в
// Конвертер. Цвета через color-mix от accent → theme-aware.
.nest-st-warning {
  margin-top: 12px;
  padding: 16px 18px;
  border-left: 4px solid rgb(var(--v-theme-warning));
  border-radius: var(--nest-radius);
  background: color-mix(in srgb, rgb(var(--v-theme-warning)) 12%, var(--nest-surface) 88%);
}
.nest-st-warning-headline {
  display: flex;
  align-items: center;
  font-size: 16px;
  font-weight: 600;
  line-height: 1.3;
  color: var(--nest-text);
  margin-bottom: 8px;
}
.nest-st-warning-body {
  font-size: 13.5px;
  line-height: 1.55;
  color: var(--nest-text-secondary);
  margin: 0 0 12px;
}
.nest-st-warning-ctas {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}

.nest-io-buttons {
  // Плотнее на mobile — чтобы 4 кнопки не становились колонкой.
  @media (max-width: 520px) {
    gap: 6px;
  }
}

.nest-html-hint {
  font-size: 11.5px;
  margin: -6px 0 0;
  color: var(--nest-text-muted);
}

@media (max-width: 640px) {
  .nest-grid { grid-template-columns: 1fr; }
  .nest-st-warning { padding: 14px; }
  .nest-st-warning-headline { font-size: 15px; }
  .nest-st-warning-ctas { flex-direction: column; align-items: stretch; }
}
</style>
