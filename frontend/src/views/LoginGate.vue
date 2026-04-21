<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

// WuApi base URL. Kept as a constant because WuNest is always deployed as
// a sibling of api.wusphere.ru in production. For local dev you'd point
// this at `http://localhost:8080`, but then host-only cookies won't bridge
// anyway — so local dev uses a different (mocked) auth path.
const WUAPI_BASE = 'https://api.wusphere.ru'

// Where WuApi should send the user after a successful login.
// window.location.origin on nest.wusphere.ru = "https://nest.wusphere.ru".
const returnTo = computed(() => encodeURIComponent(window.location.origin + '/'))

const refreshUrl = computed(() => `${WUAPI_BASE}/auth/refresh?return_to=${returnTo.value}`)

// Provider button catalogue. Order = visual order. Disabled items render
// but don't deep-link — useful for "coming soon" affordances later.
const providers = computed(() => [
  {
    id: 'google',
    label: 'Google',
    icon: 'mdi-google',
    color: '#ea4335',
    href: `${WUAPI_BASE}/auth/google?return_to=${returnTo.value}`,
  },
  {
    id: 'github',
    label: 'GitHub',
    icon: 'mdi-github',
    color: '#f0f6fc',
    href: `${WUAPI_BASE}/auth/github?return_to=${returnTo.value}`,
  },
  {
    id: 'discord',
    label: 'Discord',
    icon: 'mdi-discord',
    color: '#5865f2',
    href: `${WUAPI_BASE}/auth/discord?return_to=${returnTo.value}`,
  },
  {
    id: 'gitlab',
    label: 'GitLab',
    icon: 'mdi-gitlab',
    color: '#fc6d26',
    href: `${WUAPI_BASE}/auth/gitlab?return_to=${returnTo.value}`,
  },
])

// Telegram uses the Login Widget which lives on wusphere.ru. We just
// redirect there; after login it'll drop the user back here via its own
// post-login redirect (once that page learns about return_to).
const telegramHref = `https://wusphere.ru/login?return_to=${encodeURIComponent(window.location.origin + '/')}`
</script>

<template>
  <v-main>
    <v-container class="nest-login py-16">
      <div class="d-flex flex-column align-center ga-6">
        <div class="nest-logo-xl">▲</div>
        <h1 class="nest-h1 text-center">{{ t('login.headline') }}</h1>
        <p class="nest-subtitle text-center" style="max-width: 480px">
          {{ t('login.tagline') }}
        </p>

        <!-- Already-logged-in affordance.
             If the user already has a WuApi session (on a host-only cookie
             from the pre-domain-cookie era) this refreshes it so it becomes
             visible to all .wusphere.ru subdomains. Harmless if no session. -->
        <div class="nest-already">
          <span class="nest-already-text">{{ t('login.alreadyLogged') }}</span>
          <a class="nest-already-link" :href="refreshUrl">
            {{ t('login.continueSession') }}
            <v-icon size="14" class="ml-1">mdi-arrow-right</v-icon>
          </a>
        </div>

        <div class="nest-divider">
          <span>{{ t('login.or') }}</span>
        </div>

        <!-- Fresh-login provider buttons. Top-level navigation — no fetch,
             no CORS issues, no XHR. Browser carries any existing cookies
             naturally, so WuApi gets to pick LOGIN vs LINK mode. -->
        <div class="nest-providers">
          <a
            v-for="p in providers"
            :key="p.id"
            :href="p.href"
            class="nest-provider-btn"
            :style="{ '--prov-accent': p.color }"
          >
            <v-icon :icon="p.icon" size="20" />
            <span>{{ t('login.signInWith', { provider: p.label }) }}</span>
          </a>
          <a :href="telegramHref" class="nest-provider-btn">
            <v-icon icon="mdi-send" size="20" />
            <span>{{ t('login.signInWith', { provider: 'Telegram' }) }}</span>
          </a>
        </div>

        <div class="nest-caption mt-4">
          {{ t('login.noAccountHint') }}
        </div>
      </div>
    </v-container>
  </v-main>
</template>

<style lang="scss" scoped>
.nest-login {
  min-height: 80vh;
  display: grid;
  place-items: center;
}
.nest-logo-xl {
  width: 64px;
  height: 64px;
  display: grid;
  place-items: center;
  color: var(--nest-accent);
  font-size: 40px;
  font-weight: 600;
  border: 2px solid var(--nest-accent);
  border-radius: 12px;
  font-family: var(--nest-font-mono);
}

.nest-already {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 16px;
  background: var(--nest-bg-elevated);
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-pill);
  font-size: 13px;
  color: var(--nest-text-secondary);
}
.nest-already-text {
  font-size: 12.5px;
}
.nest-already-link {
  display: inline-flex;
  align-items: center;
  color: var(--nest-accent);
  font-weight: 500;
  text-decoration: none;
  transition: color var(--nest-transition-fast);

  &:hover { color: var(--nest-text); }
}

.nest-divider {
  display: flex;
  align-items: center;
  width: 100%;
  max-width: 360px;
  color: var(--nest-text-muted);
  font-family: var(--nest-font-mono);
  font-size: 10.5px;
  letter-spacing: 0.08em;
  text-transform: uppercase;

  &::before, &::after {
    content: '';
    flex: 1;
    border-top: 1px solid var(--nest-border);
    margin: 0 12px;
  }
}

.nest-providers {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
  max-width: 360px;
}

.nest-provider-btn {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  border: 1px solid var(--nest-border);
  border-radius: var(--nest-radius);
  background: var(--nest-surface);
  color: var(--nest-text);
  text-decoration: none;
  font-size: 14px;
  font-weight: 500;
  transition: border-color var(--nest-transition-fast), transform var(--nest-transition-fast), background var(--nest-transition-fast);

  &:hover {
    border-color: var(--prov-accent, var(--nest-accent));
    background: var(--nest-bg-elevated);
  }
  &:active {
    transform: translateY(1px);
  }

  .v-icon { color: var(--prov-accent, var(--nest-text)); }
}

.nest-caption {
  font-family: var(--nest-font-mono);
  font-size: 11px;
  letter-spacing: 0.08em;
  color: var(--nest-text-muted);
  text-transform: uppercase;
}
</style>
