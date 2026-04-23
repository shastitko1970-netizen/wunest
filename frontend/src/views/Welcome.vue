<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { useAuthStore } from '@/stores/auth'

// Public landing. Readable without a session — /welcome. Describes what
// WuNest is, what it does, who it's for, then funnels to either the
// login flow (unauthed) or the chat (authed).
//
// Not translated deeply yet — copy lives in i18n under `welcome.*`.
// Keep this page lightweight: no heavy assets, no API calls.

const { t } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const { authenticated } = storeToRefs(auth)

// Sign-in goes through /auth/start (WuNest) → /auth/refresh (WuApi) so
// we get a server-side log of every login attempt.
const loginUrl = computed(() => {
  const returnTo = encodeURIComponent(window.location.origin + '/chat')
  return `/auth/start?return_to=${returnTo}`
})

// Primary CTA routes by auth state. Authed users click "Open chat", anons
// click "Sign in with WuApi".
function onPrimary() {
  if (authenticated.value) {
    router.push('/chat')
  } else {
    window.location.href = loginUrl.value
  }
}

function toDocs() {
  router.push('/docs')
}

// Feature cards — a flat list is easier to scan on mobile than tabs.
interface Feature {
  icon: string
  titleKey: string
  bodyKey: string
}
const features: Feature[] = [
  { icon: 'mdi-card-account-details-outline', titleKey: 'welcome.features.characters.title',  bodyKey: 'welcome.features.characters.body' },
  { icon: 'mdi-book-open-variant',             titleKey: 'welcome.features.lorebooks.title',   bodyKey: 'welcome.features.lorebooks.body' },
  { icon: 'mdi-drama-masks',                   titleKey: 'welcome.features.personas.title',    bodyKey: 'welcome.features.personas.body' },
  { icon: 'mdi-tune-variant',                  titleKey: 'welcome.features.presets.title',     bodyKey: 'welcome.features.presets.body' },
  { icon: 'mdi-key-variant',                   titleKey: 'welcome.features.byok.title',        bodyKey: 'welcome.features.byok.body' },
  { icon: 'mdi-palette-outline',               titleKey: 'welcome.features.theming.title',     bodyKey: 'welcome.features.theming.body' },
  { icon: 'mdi-cellphone-link',                titleKey: 'welcome.features.mobile.title',      bodyKey: 'welcome.features.mobile.body' },
  { icon: 'mdi-file-download-outline',         titleKey: 'welcome.features.interop.title',     bodyKey: 'welcome.features.interop.body' },
]
</script>

<template>
  <div class="nest-welcome">
    <!-- Hero -->
    <section class="nest-welcome-hero">
      <div class="nest-welcome-logo">▲</div>
      <div class="nest-eyebrow">{{ t('welcome.eyebrow') }}</div>
      <h1 class="nest-h1 nest-welcome-title">{{ t('welcome.title') }}</h1>
      <p class="nest-subtitle nest-welcome-lead">{{ t('welcome.lead') }}</p>

      <div class="nest-welcome-ctas">
        <button class="nest-welcome-cta-primary" @click="onPrimary">
          <v-icon size="18" class="mr-2">
            {{ authenticated ? 'mdi-forum-outline' : 'mdi-login' }}
          </v-icon>
          {{ authenticated ? t('welcome.ctaOpen') : t('welcome.ctaLogin') }}
        </button>
        <button class="nest-welcome-cta-secondary" @click="toDocs">
          <v-icon size="18" class="mr-2">mdi-book-open-variant</v-icon>
          {{ t('welcome.ctaDocs') }}
        </button>
      </div>
    </section>

    <!-- Feature grid -->
    <section class="nest-welcome-features">
      <h2 class="nest-h2">{{ t('welcome.featuresTitle') }}</h2>
      <div class="nest-welcome-grid">
        <div
          v-for="f in features"
          :key="f.titleKey"
          class="nest-welcome-card"
        >
          <v-icon size="24" color="primary" class="nest-welcome-card-icon">
            {{ f.icon }}
          </v-icon>
          <div class="nest-welcome-card-title">{{ t(f.titleKey) }}</div>
          <p class="nest-welcome-card-body">{{ t(f.bodyKey) }}</p>
        </div>
      </div>
    </section>

    <!-- How it works -->
    <section class="nest-welcome-how">
      <h2 class="nest-h2">{{ t('welcome.howTitle') }}</h2>
      <ol class="nest-welcome-steps">
        <li>
          <span class="nest-welcome-step-num nest-mono">01</span>
          <div>
            <strong>{{ t('welcome.howStep1.title') }}</strong>
            <p class="nest-subtitle">{{ t('welcome.howStep1.body') }}</p>
          </div>
        </li>
        <li>
          <span class="nest-welcome-step-num nest-mono">02</span>
          <div>
            <strong>{{ t('welcome.howStep2.title') }}</strong>
            <p class="nest-subtitle">{{ t('welcome.howStep2.body') }}</p>
          </div>
        </li>
        <li>
          <span class="nest-welcome-step-num nest-mono">03</span>
          <div>
            <strong>{{ t('welcome.howStep3.title') }}</strong>
            <p class="nest-subtitle">{{ t('welcome.howStep3.body') }}</p>
          </div>
        </li>
      </ol>
    </section>

    <!-- Billing -->
    <section class="nest-welcome-pricing">
      <h2 class="nest-h2">{{ t('welcome.pricingTitle') }}</h2>
      <div class="nest-welcome-pricing-row">
        <div class="nest-welcome-price-card">
          <div class="nest-eyebrow">{{ t('welcome.pricing.wuapiEyebrow') }}</div>
          <div class="nest-kpi">wu-gold</div>
          <p>{{ t('welcome.pricing.wuapiBody') }}</p>
        </div>
        <div class="nest-welcome-price-card">
          <div class="nest-eyebrow">{{ t('welcome.pricing.byokEyebrow') }}</div>
          <div class="nest-kpi">BYOK</div>
          <p>{{ t('welcome.pricing.byokBody') }}</p>
        </div>
      </div>
    </section>

    <!-- Footer CTA -->
    <section class="nest-welcome-footer-cta">
      <h2 class="nest-h2">{{ t('welcome.footerTitle') }}</h2>
      <div class="nest-welcome-ctas">
        <button class="nest-welcome-cta-primary" @click="onPrimary">
          {{ authenticated ? t('welcome.ctaOpen') : t('welcome.ctaLogin') }}
        </button>
        <a
          href="https://github.com/shastitko1970-netizen/wunest"
          target="_blank"
          rel="noopener"
          class="nest-welcome-cta-secondary"
        >
          <v-icon size="18" class="mr-2">mdi-github</v-icon>
          GitHub
        </a>
      </div>
      <div class="nest-caption mt-6">{{ t('welcome.byWusphere') }}</div>
    </section>
  </div>
</template>

<style lang="scss" scoped>
.nest-welcome {
  max-width: 960px;
  margin: 0 auto;
  padding: 60px 24px 80px;
  display: flex;
  flex-direction: column;
  gap: 80px;
}

// ── Hero ─────────────────────────────────────────────────────
.nest-welcome-hero {
  text-align: center;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
}
.nest-welcome-logo {
  width: 72px;
  height: 72px;
  display: grid;
  place-items: center;
  color: var(--nest-accent);
  font-size: 44px;
  font-weight: 600;
  border: 2px solid var(--nest-accent);
  border-radius: 14px;
  font-family: var(--nest-font-mono);
  margin-bottom: 8px;
}
.nest-welcome-title {
  margin: 0;
  max-width: 720px;
}
.nest-welcome-lead {
  max-width: 640px;
  margin: 0;
  font-size: 1.15rem;
  line-height: 1.55;
}

.nest-welcome-ctas {
  display: flex;
  gap: 12px;
  margin-top: 16px;
  flex-wrap: wrap;
  justify-content: center;
}
.nest-welcome-cta-primary,
.nest-welcome-cta-secondary {
  all: unset;
  display: inline-flex;
  align-items: center;
  padding: 12px 22px;
  font-size: 14px;
  font-weight: 600;
  letter-spacing: 0.01em;
  border-radius: var(--nest-radius);
  cursor: pointer;
  transition: transform var(--nest-transition-fast), filter var(--nest-transition-fast);
  &:hover { transform: translateY(-1px); }
}
.nest-welcome-cta-primary {
  background: var(--nest-accent);
  color: #fff;
  &:hover { filter: brightness(1.1); }
  .v-icon { color: #fff; }
}
.nest-welcome-cta-secondary {
  border: 1px solid var(--nest-border);
  color: var(--nest-text);
  text-decoration: none;
  &:hover { border-color: var(--nest-accent); color: var(--nest-text); }
}

// ── Feature grid ─────────────────────────────────────────────
.nest-welcome-features h2 { margin-bottom: 20px; }
.nest-welcome-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 14px;
}
.nest-welcome-card {
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius);
  background: var(--nest-surface);
  padding: 18px 16px;
  transition: border-color var(--nest-transition-fast), transform var(--nest-transition-fast);
  &:hover {
    border-color: var(--nest-accent);
    transform: translateY(-2px);
  }
}
.nest-welcome-card-icon { margin-bottom: 10px; }
.nest-welcome-card-title {
  font-family: var(--nest-font-display);
  font-size: 16px;
  font-weight: 500;
  color: var(--nest-text);
  margin-bottom: 4px;
}
.nest-welcome-card-body {
  font-size: 13px;
  color: var(--nest-text-secondary);
  line-height: 1.5;
  margin: 0;
}

// ── How ──────────────────────────────────────────────────────
.nest-welcome-how h2 { margin-bottom: 20px; }
.nest-welcome-steps {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 14px;

  li {
    display: flex;
    gap: 16px;
    padding: 16px;
    border-left: 2px solid var(--nest-accent);
    background: var(--nest-bg-elevated);
    border-radius: 0 var(--nest-radius) var(--nest-radius) 0;

    p { margin: 4px 0 0; }
  }
}
.nest-welcome-step-num {
  font-size: 20px;
  color: var(--nest-accent);
  letter-spacing: 0.05em;
  flex-shrink: 0;
  min-width: 36px;
}

// ── Pricing ──────────────────────────────────────────────────
.nest-welcome-pricing h2 { margin-bottom: 20px; }
.nest-welcome-pricing-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
}
.nest-welcome-price-card {
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius);
  background: var(--nest-surface);
  padding: 22px 20px;
  .nest-kpi { margin: 8px 0 12px; }
  p { font-size: 13.5px; color: var(--nest-text-secondary); margin: 0; line-height: 1.55; }
}

// ── Footer CTA ───────────────────────────────────────────────
.nest-welcome-footer-cta {
  text-align: center;
  padding-top: 40px;
  border-top: 1px dashed var(--nest-border-subtle);

  h2 { margin-bottom: 20px; }
  .nest-welcome-ctas { justify-content: center; }
}
.nest-caption {
  font-family: var(--nest-font-mono);
  font-size: 11px;
  letter-spacing: 0.08em;
  color: var(--nest-text-muted);
  text-transform: uppercase;
}

// ── Mobile ───────────────────────────────────────────────────
@media (max-width: 600px) {
  .nest-welcome { padding: 40px 16px 60px; gap: 56px; }
  .nest-welcome-lead { font-size: 1rem; }
  .nest-welcome-grid { grid-template-columns: 1fr; }
  .nest-welcome-pricing-row { grid-template-columns: 1fr; }
  .nest-welcome-ctas { flex-direction: column; width: 100%; }
  .nest-welcome-cta-primary,
  .nest-welcome-cta-secondary { width: 100%; justify-content: center; }
}
</style>
