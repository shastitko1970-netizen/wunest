<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore } from '@/stores/theme'
import type { ThemePreset, ThemePresetMeta } from '@/stores/theme'

// Themes gallery — public page (`/themes`, `meta.public: true`).
//
// Purpose:
//   - Give mod-authors a public reference of the 5 built-in presets:
//     swatch strip, sample "bubble", and the pair partner for
//     dark↔light flip UX.
//   - Link straight into `docs/theming` (the how-to for custom CSS
//     authoring) so the gallery doubles as a funnel for the
//     mod ecosystem we want to seed.
//   - Let authed users apply a preset straight from the card, so
//     newcomers can try vibes without hunting for Settings.
//
// Why it's a standalone route vs. just a section in /docs:
//   - The docs viewer is text-first; a theme grid with colour swatches
//     and interactive "Apply" controls would fight that layout.
//   - Public route (no auth) means the link can be shared in Discord
//     without making the visitor sign up just to see what the themes
//     look like.
//
// We deliberately do NOT hotwire the page to mutate the live shell on
// hover — previews are static swatches/samples. An "Apply" click does
// the full theme switch (same path as AppearancePanel). Anonymous
// visitors see a `loginToApply` CTA instead of the apply button.

const { t } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const { authenticated } = storeToRefs(auth)
const themeStore = useThemeStore()
const { currentId, presets } = storeToRefs(themeStore)

// M51 Sprint 3 wave 1 — swatches now live in the per-preset
// `*.theme.json` manifest (sibling of the CSS file). Single source
// of truth: `meta.swatches` is the same data the AppearancePanel
// uses for its mini-previews. Adding a 6th theme is one manifest +
// one CSS file, no edit here.
const cards = computed(() => presets.value.map((p: ThemePresetMeta) => ({
  meta: p,
  swatch: p.swatches,
  pairMeta: p.pair ? presets.value.find(x => x.id === p.pair) : undefined,
})))

async function apply(id: ThemePreset) {
  if (!authenticated.value) {
    const returnTo = encodeURIComponent(window.location.origin + '/themes')
    window.location.href = `/auth/start?return_to=${returnTo}`
    return
  }
  // M51 Sprint 2 wave 3 — gallery click is a manual pick → userPick
  // path which also disables follow-system. Otherwise the auto-flip
  // listener would override the user's gallery selection on next
  // OS theme change.
  await themeStore.userPick(id)
}

function toDocs() {
  router.push('/docs/theming')
}

function kindLabel(kind: 'dark' | 'light') {
  return kind === 'dark' ? t('themes.kindDark') : t('themes.kindLight')
}
</script>

<template>
  <div class="nest-themes nest-admin">
    <!-- Hero -->
    <section class="nest-themes-hero">
      <div class="nest-eyebrow">{{ t('themes.eyebrow') }}</div>
      <h1 class="nest-h1 nest-themes-title">{{ t('themes.title') }}</h1>
      <p class="nest-subtitle nest-themes-lead">{{ t('themes.lead') }}</p>
    </section>

    <!-- Preset cards -->
    <section class="nest-themes-grid">
      <article
        v-for="card in cards"
        :key="card.meta.id"
        class="nest-themes-card"
        :class="{ 'is-active': currentId === card.meta.id }"
      >
        <!-- Preview strip: swatch + tiny chat bubble sample painted
             with the preset's own colours (hard-coded from the css
             files, not live DOM). Keeps each card self-contained. -->
        <div
          class="nest-themes-preview"
          :style="{
            background: card.swatch.bg,
            borderColor: card.swatch.border,
          }"
        >
          <div
            class="nest-themes-bubble"
            :style="{
              background: card.swatch.surface,
              borderColor: card.swatch.border,
              color: card.swatch.text,
            }"
          >
            <div
              class="nest-themes-bubble-name"
              :style="{ color: card.swatch.accent }"
            >
              {{ card.meta.id === 'tavern-warm' ? 'Bard' : 'Rin' }}
            </div>
            <div class="nest-themes-bubble-body">
              {{ t('themes.preview.sampleBody') }}
            </div>
          </div>

          <!-- Swatch dots row: bg / surface / border / text / accent.
               Border explicit colour in case accent≈bg for very low
               contrast presets. -->
          <div class="nest-themes-swatches">
            <span
              v-for="(c, i) in [
                card.swatch.bg,
                card.swatch.surface,
                card.swatch.border,
                card.swatch.text,
                card.swatch.accent,
              ]"
              :key="i"
              class="nest-themes-swatch-dot"
              :style="{ background: c, borderColor: card.swatch.border }"
            />
          </div>
        </div>

        <!-- Card body: name, description, meta chips, actions -->
        <div class="nest-themes-body">
          <div class="nest-themes-card-head">
            <h3 class="nest-themes-name">{{ card.meta.label }}</h3>
            <span
              class="nest-themes-kind"
              :class="`nest-themes-kind--${card.meta.kind}`"
            >
              {{ kindLabel(card.meta.kind) }}
            </span>
          </div>
          <p class="nest-themes-desc">{{ card.meta.description }}</p>

          <div class="nest-themes-meta">
            <div class="nest-mono nest-themes-meta-label">
              {{ t('themes.pair') }}
            </div>
            <div class="nest-themes-meta-val">
              <template v-if="card.pairMeta">
                {{ card.pairMeta.label }}
                <span class="nest-themes-pair-kind">
                  · {{ kindLabel(card.pairMeta.kind) }}
                </span>
              </template>
              <template v-else>
                <span class="nest-themes-meta-muted">
                  {{ t('themes.pairNone') }}
                </span>
              </template>
            </div>
          </div>

          <!-- Apply button. Anon → redirects to WuApi sign-in, the
               return_to brings them back here for a retry. Active
               preset shows a read-only "Active" chip. -->
          <div class="nest-themes-actions">
            <button
              v-if="currentId === card.meta.id"
              class="nest-themes-applied"
              disabled
            >
              <v-icon size="16" color="success" class="mr-1">mdi-check-circle</v-icon>
              {{ t('themes.applied') }}
            </button>
            <button
              v-else
              class="nest-themes-apply"
              :style="{
                background: card.swatch.accent,
                color: card.swatch.accentOn,
              }"
              @click="apply(card.meta.id)"
            >
              {{ authenticated ? t('themes.apply') : t('themes.loginToApply') }}
            </button>
          </div>
        </div>
      </article>
    </section>

    <!-- Rolling-your-own funnel -->
    <section class="nest-themes-how">
      <h2 class="nest-h2">{{ t('themes.howToMod') }}</h2>
      <p class="nest-subtitle nest-themes-how-body">
        {{ t('themes.howToModBody') }}
      </p>
      <button class="nest-themes-how-cta" @click="toDocs">
        <v-icon size="18" class="mr-2">mdi-book-open-variant</v-icon>
        {{ t('themes.howToModCta') }}
      </button>
    </section>

    <div class="nest-caption nest-themes-footer">
      {{ t('themes.byWusphere') }}
    </div>
  </div>
</template>

<style lang="scss" scoped>
.nest-themes {
  max-width: 960px;
  margin: 0 auto;
  padding: 60px 24px 80px;
  display: flex;
  flex-direction: column;
  gap: 64px;
}

// ── Hero ─────────────────────────────────────────────────────
.nest-themes-hero {
  text-align: center;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
}
.nest-themes-title {
  margin: 0;
  max-width: 720px;
}
.nest-themes-lead {
  max-width: 640px;
  margin: 0;
  font-size: 1.1rem;
  line-height: 1.55;
}

// ── Grid ─────────────────────────────────────────────────────
.nest-themes-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 18px;
}
.nest-themes-card {
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius);
  background: var(--nest-surface);
  overflow: hidden;
  display: flex;
  flex-direction: column;
  transition: border-color var(--nest-transition-fast), transform var(--nest-transition-fast);
  &:hover {
    border-color: var(--nest-accent);
    transform: translateY(-2px);
  }
  &.is-active {
    border-color: var(--nest-accent);
    box-shadow: 0 0 0 1px var(--nest-accent);
  }
}

// ── Preview ──────────────────────────────────────────────────
// Preview paints itself with the preset's own colours so you can
// see what the theme looks like without applying it.
.nest-themes-preview {
  padding: 18px 16px;
  display: flex;
  flex-direction: column;
  gap: 14px;
  border-bottom: 1px solid var(--nest-border-subtle);
  min-height: 160px;
}
.nest-themes-bubble {
  border: 1px solid;
  border-radius: 12px;
  padding: 12px 14px;
  font-size: 13px;
  line-height: 1.5;
}
.nest-themes-bubble-name {
  font-weight: 600;
  font-size: 13px;
  margin-bottom: 4px;
  letter-spacing: 0.01em;
}
.nest-themes-bubble-body {
  opacity: 0.9;
}
.nest-themes-swatches {
  display: flex;
  gap: 8px;
}
.nest-themes-swatch-dot {
  width: 18px;
  height: 18px;
  border-radius: 50%;
  border: 1px solid;
}

// ── Body ─────────────────────────────────────────────────────
.nest-themes-body {
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  flex: 1;
}
.nest-themes-card-head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 8px;
}
.nest-themes-name {
  font-family: var(--nest-font-display);
  font-size: 17px;
  font-weight: 500;
  color: var(--nest-text);
  margin: 0;
}
.nest-themes-kind {
  font-family: var(--nest-font-mono);
  font-size: 10px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  padding: 2px 8px;
  border-radius: 999px;
  // Kind-specific chip: muted so it doesn't fight the preview swatch
  &--dark {
    background: rgba(255, 255, 255, 0.06);
    color: var(--nest-text-secondary);
    border: 1px solid var(--nest-border-subtle);
  }
  &--light {
    background: rgba(0, 0, 0, 0.04);
    color: var(--nest-text-secondary);
    border: 1px solid var(--nest-border-subtle);
  }
}
.nest-themes-desc {
  font-size: 13px;
  color: var(--nest-text-secondary);
  line-height: 1.5;
  margin: 0;
  flex: 1;
}
.nest-themes-meta {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px 10px;
  background: var(--nest-bg-elevated);
  border-radius: 8px;
  border: 1px solid var(--nest-border-subtle);
}
.nest-themes-meta-label {
  font-size: 10px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--nest-text-muted);
}
.nest-themes-meta-val {
  font-size: 13px;
  color: var(--nest-text);
}
.nest-themes-meta-muted {
  color: var(--nest-text-muted);
  font-style: italic;
}
.nest-themes-pair-kind {
  color: var(--nest-text-secondary);
  font-size: 12px;
}

// ── Actions ─────────────────────────────────────────────────
.nest-themes-actions {
  margin-top: 4px;
}
.nest-themes-apply,
.nest-themes-applied {
  all: unset;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 10px 16px;
  font-size: 13px;
  font-weight: 600;
  letter-spacing: 0.01em;
  border-radius: var(--nest-radius);
  cursor: pointer;
  width: 100%;
  box-sizing: border-box;
  transition: filter var(--nest-transition-fast);
  &:hover { filter: brightness(1.1); }
}
.nest-themes-apply {
  // Background/colour come from the preset swatch inline. Fallback
  // to accent if somehow inline style is stripped.
  background: var(--nest-accent);
  color: #fff;
}
.nest-themes-applied {
  background: var(--nest-bg-elevated);
  color: var(--nest-text-secondary);
  border: 1px solid var(--nest-border-subtle);
  cursor: default;
  &:hover { filter: none; }
  // Icon colour comes from Vuetify's `color="success"` (theme palette),
  // which follows the active preset's kind via our global.scss bridge.
}

// ── "Roll your own" funnel ──────────────────────────────────
.nest-themes-how {
  text-align: center;
  padding: 40px 24px;
  border: 1px dashed var(--nest-border-subtle);
  border-radius: var(--nest-radius);
  background: var(--nest-bg-elevated);
}
.nest-themes-how-body {
  max-width: 560px;
  margin: 8px auto 20px;
  font-size: 14px;
}
.nest-themes-how-cta {
  all: unset;
  display: inline-flex;
  align-items: center;
  padding: 10px 18px;
  font-size: 14px;
  font-weight: 600;
  border: 1px solid var(--nest-accent);
  color: var(--nest-text);
  border-radius: var(--nest-radius);
  cursor: pointer;
  transition: background var(--nest-transition-fast);
  &:hover { background: rgba(255, 255, 255, 0.04); }
}

.nest-themes-footer {
  text-align: center;
  padding-top: 16px;
  border-top: 1px dashed var(--nest-border-subtle);
}
.nest-caption {
  font-family: var(--nest-font-mono);
  font-size: 11px;
  letter-spacing: 0.08em;
  color: var(--nest-text-muted);
  text-transform: uppercase;
}

// ── Mobile ──────────────────────────────────────────────────
// DS-canonical 640px: two-col → one-col.
@media (max-width: 640px) {
  .nest-themes { padding: 40px 16px 60px; gap: 48px; }
  .nest-themes-grid { grid-template-columns: 1fr; }
  .nest-themes-lead { font-size: 1rem; }
}
</style>
