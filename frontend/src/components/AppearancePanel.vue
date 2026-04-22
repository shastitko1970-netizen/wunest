<script setup lang="ts">
import { computed, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useAppearanceStore } from '@/stores/appearance'
import { fromST, toST, type AvatarStyle, type ChatDisplay, type STTheme } from '@/api/appearance'

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

const fileInput = ref<HTMLInputElement | null>(null)
const importError = ref<string | null>(null)

async function onImportFile(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0]
  if (!f) return
  importError.value = null
  try {
    const text = await f.text()
    const parsed = JSON.parse(text) as STTheme
    const mapped = fromST(parsed)
    store.update(mapped)
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
    </div>

    <!-- Custom CSS -->
    <div class="nest-field">
      <label class="nest-field-label">
        {{ t('appearance.customCss') }}
        <span class="nest-mono text-medium-emphasis">{{ t('appearance.customCssHint') }}</span>
      </label>
      <v-textarea
        v-model="customCss"
        :placeholder="t('appearance.customCssPlaceholder')"
        rows="5"
        auto-grow
        density="compact"
        hide-details
        variant="outlined"
        class="nest-mono-textarea"
      />
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
}

.nest-io {
  padding-top: 12px;
  border-top: 1px dashed var(--nest-border-subtle);
}

@media (max-width: 640px) {
  .nest-grid { grid-template-columns: 1fr; }
}
</style>
