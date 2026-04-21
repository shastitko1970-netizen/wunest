<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

// WuApi base URL. Deployed as a sibling of nest.wusphere.ru in production.
const WUAPI_BASE = 'https://api.wusphere.ru'

// Where WuApi should send the user after a successful login.
const returnTo = computed(() => encodeURIComponent(window.location.origin + '/'))

// Single entry point: /auth/refresh is the "smart" endpoint on WuApi.
//   - Logged in already on WuApi → refresh cookie, 302 back here.
//   - Not logged in            → 302 to /login (WuApi's login page, with
//                                 all OAuth buttons + Telegram widget).
// User sees exactly one button on WuNest — WuApi decides the rest.
const loginUrl = computed(() => `${WUAPI_BASE}/auth/refresh?return_to=${returnTo.value}`)
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

        <a :href="loginUrl" class="nest-login-cta">
          <v-icon icon="mdi-login" size="18" class="mr-2" />
          {{ t('login.cta') }}
        </a>

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

// Single CTA — styled as the dominant call-to-action. Uses accent color
// solid so it stands out more than a secondary outlined button would.
.nest-login-cta {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 14px 28px;
  margin-top: 8px;
  min-width: 280px;

  background: var(--nest-accent);
  color: #fff;
  font-size: 15px;
  font-weight: 600;
  letter-spacing: 0.01em;
  text-decoration: none;
  border-radius: var(--nest-radius);

  transition: transform var(--nest-transition-fast), box-shadow var(--nest-transition-fast), filter var(--nest-transition-fast);

  &:hover {
    filter: brightness(1.1);
    transform: translateY(-1px);
  }
  &:active {
    transform: translateY(0);
  }

  .v-icon { color: #fff; }
}

.nest-caption {
  font-family: var(--nest-font-mono);
  font-size: 11px;
  letter-spacing: 0.08em;
  color: var(--nest-text-muted);
  text-transform: uppercase;
}
</style>
