<script setup lang="ts">
import { computed, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useAppearanceStore } from '@/stores/appearance'
import { fromST, toST, type AvatarStyle, type ChatDisplay, type STTheme } from '@/api/appearance'
import { auditDangerousSelectors, supportsCSSScope } from '@/lib/cssScope'

// Detailed appearance controls. Lives as a section of /settings so users
// see everything in one place: theme presets, density, custom colors,
// background image, custom CSS, and SillyTavern theme import/export.
//
// All mutations route through the store's update() which debounces the
// server PUT, so dragging a slider doesn't hammer the backend.
const { t } = useI18n()
const store = useAppearanceStore()
const { appearance, saving } = storeToRefs(store)

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

// 'chat' (default) — CSS wrapped in @scope(#chat) so ST themes don't
// paint over settings/library/menu. 'global' — legacy behaviour; the
// whole UI inherits the CSS. Default 'chat' for safety.
const customCssScope = computed<'chat' | 'global'>({
  get: () => appearance.value.customCssScope ?? 'chat',
  set: v => store.update({ customCssScope: v }),
})

// Tally of broad-element selectors ("body", "textarea", ...) so the
// "scope toggle" can warn users that flipping to 'global' would hit
// them. Recomputes on every CSS edit; cheap since themes are ≤500 lines.
const dangerousAudit = computed(() => auditDangerousSelectors(customCss.value))

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
        </v-btn-toggle>
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
        <v-text-field
          v-model="bgImage"
          :placeholder="t('appearance.bgImagePlaceholder')"
          density="compact"
          hide-details
        />
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
        <p class="nest-subtitle mt-2" style="font-size: 12px">
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
          class="mt-2"
          style="font-size: 11.5px"
        >
          <div>
            {{ t('appearance.cssScope.globalWarning') }}
          </div>
          <div class="mt-1 nest-mono" style="font-size: 10.5px; opacity: 0.85">
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
          class="mt-2"
          style="font-size: 11.5px"
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
          <p class="nest-subtitle" style="font-size: 12.5px">{{ t('appearance.guide.varsIntro') }}</p>
          <pre class="nest-guide-snippet">:root {
  --SmartThemeBodyColor: #f0f0f0;
  --SmartThemeBorderColor: #3c1e50;
  --SmartThemeQuoteColor: #ef4444;    /* accent */
  --SmartThemeBlurTintColor: #0b0818;  /* main bg */
  --SmartThemeChatTintColor: #130d15;  /* panels */
  --SmartThemeBodyFont: 'Andika';
}</pre>

          <h4 class="nest-h4 mt-3">{{ t('appearance.guide.classesTitle') }}</h4>
          <p class="nest-subtitle" style="font-size: 12.5px">{{ t('appearance.guide.classesIntro') }}</p>
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
          <p class="nest-subtitle" style="font-size: 12.5px">{{ t('appearance.guide.exampleIntro') }}</p>
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

          <p class="nest-subtitle mt-3" style="font-size: 11.5px">
            {{ t('appearance.guide.stImportNote') }}
          </p>
        </div>
      </details>
    </div>

    <!-- Import / Export -->
    <div class="nest-field nest-io">
      <label class="nest-field-label">{{ t('appearance.io.title') }}</label>
      <p class="nest-subtitle">{{ t('appearance.io.hint') }}</p>
      <div class="d-flex ga-2 flex-wrap mt-2">
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
          prepend-icon="mdi-download"
          @click="downloadExport"
        >
          {{ t('appearance.io.export') }}
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

.nest-app-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  flex-wrap: wrap;
}
.nest-app-head-right {
  display: flex;
  align-items: center;
  gap: 8px;
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
.nest-field-label {
  display: flex;
  justify-content: space-between;
  font-size: 12.5px;
  color: var(--nest-text-secondary);
  font-weight: 500;
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
  border-radius: 999px;
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

.nest-html-hint {
  font-size: 11.5px;
  margin: -6px 0 0;
  color: var(--nest-text-muted);
}

@media (max-width: 640px) {
  .nest-grid { grid-template-columns: 1fr; }
}
</style>
