<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import AppearancePanel from '@/components/AppearancePanel.vue'
import BYOKPanel from '@/components/BYOKPanel.vue'
import { useModelsStore } from '@/stores/models'
import { usePreferencesStore } from '@/stores/preferences'
import { apiFetch } from '@/api/client'

// M46 — Settings page rework.
//
// Previously a flat vertical stack without visible hierarchy; tester
// complained that after a season of changes it felt scattered. This
// rework:
//
//   - Adds a desktop-only side nav (TOC) так что user может прыгнуть
//     в нужную секцию без скролла. На mobile nav скрыт, секции
//     просто идут подряд.
//   - Groups related controls into labelled sections с eyebrow +
//     subtitle — общие, внешний вид, ключи.
//   - Adds "Where it moved" stub cards pointing from old spots to
//     new places (chat-specific settings → drawer icon, generation
//     sampler → /presets).
//   - AppearancePanel теперь несёт главный warning про ST-темы + CTA
//     к Конвертеру; здесь в Settings просто оборачиваем его в ясную
//     section.

const { t, locale, availableLocales } = useI18n()
const models = useModelsStore()
const prefs = usePreferencesStore()
const { disableStreaming } = storeToRefs(prefs)
const { items: modelOptions, loading: modelsLoading } = storeToRefs(models)

const currentLocale = computed({
  get: () => locale.value,
  set: (v: string) => {
    locale.value = v
    localStorage.setItem('nest:locale', v)
  },
})

const localeLabel = (code: string) => {
  switch (code) {
    case 'ru': return 'Русский'
    case 'en': return 'English'
    default:   return code.toUpperCase()
  }
}

// ─── Default generation model ────────────────────────────────────────
// Server-side на nest_users.settings.default_model. Применяется when
// chat send не задаёт модель и нет chat-level override.
const defaultModel = ref<string>('')
const defaultModelSaving = ref(false)
const defaultModelSaved = ref(false)

onMounted(async () => {
  try {
    const r = await apiFetch<{ default_model: string }>('/api/me/default-model')
    defaultModel.value = r.default_model ?? ''
  } catch { /* non-fatal */ }
  // Settings не привязан к чату, показываем WuApi-пул. Если юзеру
  // нужен BYOK-only default, он его пинит per-chat.
  await models.setActiveSource('wuapi')
})

const defaultModelOptions = computed(() => [
  { id: '', title: t('settings.defaultModel.serverFallback') },
  ...modelOptions.value.map((m: { id: string }) => ({ id: m.id, title: m.id })),
])

async function saveDefaultModel(v: string) {
  defaultModel.value = v
  defaultModelSaving.value = true
  try {
    await apiFetch('/api/me/default-model', {
      method: 'PUT',
      body: JSON.stringify({ default_model: v }),
    })
    defaultModelSaved.value = true
    setTimeout(() => (defaultModelSaved.value = false), 1500)
  } finally {
    defaultModelSaving.value = false
  }
}

// ─── TOC / desktop section nav ──────────────────────────────────────
// Anchor-click jumps via native hash navigation (+ CSS scroll-behavior
// smooth в html). Active state tracks which section is in viewport
// via IntersectionObserver.
interface TocEntry {
  id: string
  icon: string
  labelKey: string
}
const TOC: TocEntry[] = [
  { id: 'general',    icon: 'mdi-tune-variant',         labelKey: 'settings.nav.general' },
  { id: 'appearance', icon: 'mdi-palette-outline',      labelKey: 'settings.nav.appearance' },
  { id: 'byok',       icon: 'mdi-key-variant',          labelKey: 'settings.nav.byok' },
]
const activeTocId = ref<string>('general')

onMounted(() => {
  const io = new IntersectionObserver(
    (entries) => {
      // Pick the topmost entry currently intersecting (rootMargin
      // offsets so "active" flips when the heading reaches ~25% from
      // top of viewport — feels natural when scrolling).
      const visible = entries
        .filter((e) => e.isIntersecting)
        .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top)
      if (visible.length > 0 && visible[0].target.id) {
        activeTocId.value = visible[0].target.id
      }
    },
    { rootMargin: '-25% 0px -60% 0px', threshold: 0 },
  )
  for (const t2 of TOC) {
    const el = document.getElementById(t2.id)
    if (el) io.observe(el)
  }
})
</script>

<template>
  <v-container class="nest-settings nest-admin" fluid>
    <div class="nest-settings-layout">
      <!-- Desktop side TOC — hidden on mobile via CSS -->
      <aside class="nest-settings-toc" aria-label="Settings sections">
        <div class="nest-eyebrow nest-settings-toc-title">
          {{ t('nav.settings') }}
        </div>
        <a
          v-for="entry in TOC"
          :key="entry.id"
          :href="'#' + entry.id"
          class="nest-settings-toc-link"
          :class="{ 'is-active': activeTocId === entry.id }"
        >
          <v-icon size="16" class="mr-2">{{ entry.icon }}</v-icon>
          <span>{{ t(entry.labelKey) }}</span>
        </a>
      </aside>

      <!-- Main column -->
      <div class="nest-settings-main">
        <div class="nest-eyebrow">{{ t('nav.settings') }}</div>
        <h1 class="nest-h1 mt-1">{{ t('settings.title') }}</h1>
        <p class="nest-subtitle mt-2 nest-settings-lead">
          {{ t('settings.lead') }}
        </p>

        <!-- ───── §1 Общие ────────────────────────────────────── -->
        <section id="general" class="nest-section nest-section--first">
          <div class="nest-section-head">
            <v-icon size="20" class="mr-2">mdi-tune-variant</v-icon>
            <h2 class="nest-h2 mb-0">{{ t('settings.sections.general') }}</h2>
          </div>
          <p class="nest-subtitle nest-section-sub">
            {{ t('settings.sections.generalSub') }}
          </p>

          <div class="nest-subsection">
            <div class="nest-subsection-head">{{ t('settings.language') }}</div>
            <v-radio-group v-model="currentLocale" hide-details density="compact">
              <v-radio
                v-for="code in availableLocales"
                :key="code"
                :label="localeLabel(code)"
                :value="code"
              />
            </v-radio-group>
          </div>

          <div class="nest-subsection">
            <div class="nest-subsection-head">
              {{ t('settings.defaultModel.title') }}
              <v-fade-transition>
                <span v-if="defaultModelSaved" class="nest-saved-chip nest-mono">
                  <v-icon size="12" color="success" class="mr-1">mdi-check</v-icon>
                  {{ t('settings.defaultModel.saved') }}
                </span>
              </v-fade-transition>
            </div>
            <p class="nest-hint mb-3">{{ t('settings.defaultModel.tagline') }}</p>
            <v-select
              :model-value="defaultModel"
              :items="defaultModelOptions"
              item-title="title"
              item-value="id"
              :loading="modelsLoading || defaultModelSaving"
              density="compact"
              hide-details
              class="nest-settings-select"
              @update:model-value="saveDefaultModel"
            />
          </div>

          <div class="nest-subsection">
            <div class="nest-subsection-head">{{ t('settings.streaming.title') }}</div>
            <p class="nest-hint mb-3">{{ t('settings.streaming.tagline') }}</p>
            <v-switch
              v-model="disableStreaming"
              :label="t('settings.streaming.disableLabel')"
              color="primary"
              inset
              hide-details
              density="compact"
            />
            <p class="nest-hint mt-2">
              {{ t('settings.streaming.disableHint') }}
            </p>
          </div>
        </section>

        <!-- ───── §2 Внешний вид ─────────────────────────────── -->
        <section id="appearance" class="nest-section">
          <div class="nest-section-head">
            <v-icon size="20" class="mr-2">mdi-palette-outline</v-icon>
            <h2 class="nest-h2 mb-0">{{ t('settings.sections.appearance') }}</h2>
          </div>
          <p class="nest-subtitle nest-section-sub">
            {{ t('settings.sections.appearanceSub') }}
          </p>
          <AppearancePanel />
        </section>

        <!-- ───── §3 Свои ключи ──────────────────────────────── -->
        <section id="byok" class="nest-section">
          <div class="nest-section-head">
            <v-icon size="20" class="mr-2">mdi-key-variant</v-icon>
            <h2 class="nest-h2 mb-0">{{ t('settings.sections.byok') }}</h2>
          </div>
          <p class="nest-subtitle nest-section-sub">
            {{ t('settings.sections.byokSub') }}
          </p>
          <BYOKPanel />
        </section>

        <!-- ───── «Где это теперь?» ──────────────────────────── -->
        <!-- Pointer card for users looking for settings that moved.
             Prevents confusion после M42-M44 rework'ов. -->
        <section class="nest-section nest-moved">
          <div class="nest-section-head">
            <v-icon size="20" class="mr-2" color="info">mdi-map-marker-outline</v-icon>
            <h2 class="nest-h2 mb-0">{{ t('settings.moved.title') }}</h2>
          </div>
          <p class="nest-subtitle nest-section-sub">
            {{ t('settings.moved.lead') }}
          </p>
          <ul class="nest-moved-list">
            <li>
              <strong>{{ t('settings.moved.sampler') }}</strong>
              <span>{{ t('settings.moved.samplerWhere') }}</span>
            </li>
            <li>
              <strong>{{ t('settings.moved.memory') }}</strong>
              <span>{{ t('settings.moved.memoryWhere') }}</span>
            </li>
            <li>
              <strong>{{ t('settings.moved.presets') }}</strong>
              <a href="/presets">{{ t('settings.moved.presetsWhere') }}</a>
            </li>
            <li>
              <strong>{{ t('settings.moved.themes') }}</strong>
              <a href="/themes">{{ t('settings.moved.themesWhere') }}</a>
            </li>
            <li>
              <strong>{{ t('settings.moved.convert') }}</strong>
              <a href="/convert">{{ t('settings.moved.convertWhere') }}</a>
            </li>
          </ul>
        </section>
      </div>
    </div>
  </v-container>
</template>

<style lang="scss" scoped>
// M46 layout — two-column on desktop (TOC + content), single column on
// mobile (TOC collapses away). Container is `fluid` because the TOC
// sits outside the 720px content cap.
.nest-settings {
  padding: 32px 24px;
}
.nest-settings-layout {
  display: grid;
  grid-template-columns: 200px minmax(0, 720px);
  gap: 48px;
  max-width: 1000px;
  margin: 0 auto;
}
.nest-settings-main {
  min-width: 0; // prevent children from blowing out the grid cell
}
.nest-settings-lead {
  max-width: 640px;
  margin-bottom: 32px;
}

// ─── TOC ─────────────────────────────────────────────────────
.nest-settings-toc {
  position: sticky;
  top: calc(var(--nest-header-height, 56px) + 24px);
  align-self: start;
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding-top: 48px; // aligns with h1 line
}
.nest-settings-toc-title {
  opacity: 0.7;
  margin-bottom: 8px;
}
.nest-settings-toc-link {
  all: unset;
  display: flex;
  align-items: center;
  padding: 6px 10px;
  font-size: 13px;
  color: var(--nest-text-secondary);
  border-left: 2px solid transparent;
  cursor: pointer;
  transition: color var(--nest-transition-fast), border-color var(--nest-transition-fast);
  &:hover {
    color: var(--nest-text);
  }
  &.is-active {
    color: var(--nest-accent);
    border-left-color: var(--nest-accent);
  }
}

// ─── Section scaffolding ─────────────────────────────────────
.nest-section {
  margin-top: 48px;
  padding-top: 32px;
  border-top: 1px solid var(--nest-border-subtle);
  scroll-margin-top: calc(var(--nest-header-height, 56px) + 16px);
}
.nest-section--first {
  border-top: none;
  padding-top: 0;
  margin-top: 8px;
}
.nest-section-head {
  display: flex;
  align-items: center;
  margin-bottom: 4px;
  .nest-h2 { line-height: 1.2; }
}
.nest-section-sub {
  max-width: 600px;
  margin: 0 0 24px;
  font-size: 13.5px;
  line-height: 1.55;
}

// ─── Sub-section (inside a section) ──────────────────────────
// Softer heading than h2, used for Language/DefaultModel/Streaming
// inside the "General" group.
.nest-subsection {
  margin-top: 28px;
  &:first-child { margin-top: 0; }
}
.nest-subsection-head {
  display: flex;
  align-items: baseline;
  gap: 10px;
  font-family: var(--nest-font-display);
  font-size: 16px;
  font-weight: 500;
  color: var(--nest-text);
  margin-bottom: 6px;
}
.nest-hint {
  font-size: 12.5px;
  color: var(--nest-text-muted);
  line-height: 1.5;
}
.nest-saved-chip {
  display: inline-flex;
  align-items: center;
  font-size: 11px;
  letter-spacing: 0.04em;
  color: var(--nest-text-secondary);
}

.nest-settings-select {
  max-width: 420px;
}

// ─── «Где это теперь?» card ─────────────────────────────────
.nest-moved-list {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;

  li {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
    padding: 10px 14px;
    border: 1px solid var(--nest-border-subtle);
    border-radius: var(--nest-radius);
    background: var(--nest-bg-elevated);
    font-size: 13.5px;

    strong {
      font-weight: 600;
      color: var(--nest-text);
      min-width: 160px;
    }
    span, a {
      color: var(--nest-text-secondary);
      flex: 1;
    }
    a {
      color: var(--nest-accent);
      text-decoration: none;
      &:hover { text-decoration: underline; }
    }
  }
}

// ─── Mobile ──────────────────────────────────────────────────
@media (max-width: 960px) {
  .nest-settings-layout {
    grid-template-columns: 1fr;
    gap: 0;
  }
  .nest-settings-toc {
    display: none;
  }
  .nest-moved-list li strong {
    min-width: 0;
    flex-basis: 100%;
  }
}
</style>
