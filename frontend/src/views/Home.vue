<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useAuthStore } from '@/stores/auth'
import { useI18n } from 'vue-i18n'

const auth = useAuthStore()
const { loading, authenticated, profile } = storeToRefs(auth)
const { t } = useI18n()

const goldDisplay = (nano: number) => (nano / 1_000_000_000).toFixed(2)
</script>

<template>
  <v-main>
    <!-- 820px matches --nest-content-max from global.scss; wrapping the
         container width in a class keeps layout tokens consistent. -->
    <v-container class="py-16 nest-home-container">
      <div v-if="loading" class="text-center text-medium-emphasis">
        {{ t('app.loading') }}
      </div>

      <template v-else-if="!authenticated">
        <div class="d-flex flex-column align-center ga-6">
          <h1 class="nest-h1 text-center">{{ t('home.anonymous') }}</h1>
          <p class="nest-subtitle text-center">
            Современный клиент для ролевой переписки с моделями.
            <br />
            Залогинься через WuSphere — ключи и подписка подтянутся сами.
          </p>
          <v-btn
            size="x-large"
            color="primary"
            class="mt-4"
            @click="auth.redirectToLogin()"
          >
            {{ t('home.loginCta') }}
          </v-btn>
        </div>
      </template>

      <template v-else-if="profile">
        <h1 class="nest-h1">{{ t('home.greeting', { name: profile.first_name || profile.username }) }}</h1>

        <v-row class="mt-6" dense>
          <v-col cols="12" sm="4">
            <v-card class="pa-4 h-100" color="surface">
              <div class="nest-eyebrow">{{ t('home.tierLabel') }}</div>
              <div class="nest-kpi mt-2">{{ profile.tier }}</div>
            </v-card>
          </v-col>
          <v-col cols="12" sm="4">
            <v-card class="pa-4 h-100" color="surface">
              <div class="nest-eyebrow">{{ t('home.goldLabel') }}</div>
              <div class="nest-kpi nest-kpi--gold mt-2">{{ goldDisplay(profile.gold_balance_nano) }}</div>
            </v-card>
          </v-col>
          <v-col cols="12" sm="4">
            <v-card class="pa-4 h-100" color="surface">
              <div class="nest-eyebrow">{{ t('home.quotaLabel') }}</div>
              <div class="nest-kpi mt-2">
                {{ profile.used_today }} / {{ profile.daily_limit }}
              </div>
            </v-card>
          </v-col>
        </v-row>

        <v-alert type="info" variant="tonal" class="mt-8">
          Скелет запущен. Чат, персонажи и миры появятся в следующих итерациях.
        </v-alert>
      </template>
    </v-container>
  </v-main>
</template>

<style lang="scss" scoped>
.nest-home-container {
  // 820px matches --nest-content-max; keeping the constraint in CSS
  // (not inline) lets mod authors override via:
  //   .nest-home-container { max-width: var(--nest-content-max) }
  max-width: var(--nest-content-max);
}
</style>
