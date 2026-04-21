<script setup lang="ts">
import { useAuthStore } from '@/stores/auth'
import { useI18n } from 'vue-i18n'

const auth = useAuthStore()
const { t } = useI18n()
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
        <v-btn
          size="x-large"
          color="primary"
          class="mt-4"
          :prepend-icon="'mdi-login'"
          @click="auth.redirectToLogin()"
        >
          {{ t('login.cta') }}
        </v-btn>
        <div class="nest-caption mt-6">
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
.nest-caption {
  font-family: var(--nest-font-mono);
  font-size: 11px;
  letter-spacing: 0.08em;
  color: var(--nest-text-muted);
  text-transform: uppercase;
}
</style>
