<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useSubscriptionStore } from '@/stores/subscription'
import PlanCardsGrid from '@/components/PlanCardsGrid.vue'

// Subscription page (M54.3).
//
// The 3 plan cards live in a shared component (PlanCardsGrid) so the
// inline section on /account stays in sync — same loading / error /
// purchase logic, no duplication. This page wraps the grid with the
// "active subscription" banner, monthly discount-cap usage hint, and
// the regulatory footnote.

const { t } = useI18n()
const sub = useSubscriptionStore()
const { state, buyError } = storeToRefs(sub)

onMounted(() => {
  void sub.fetchState()
})

const expiresLabel = computed(() => {
  const exp = state.value?.expires_at
  if (!exp) return ''
  return new Date(exp).toLocaleDateString()
})

const discountHintShown = computed<boolean>(() => {
  if (!state.value) return false
  return state.value.gold_discount_pct > 0 && state.value.gold_discount_cap_nano > 0
})

const discountHint = computed(() => {
  const s = state.value
  if (!s) return ''
  const used = s.gold_discount_used_nano / 1_000_000_000
  const remaining = s.gold_discount_remaining_nano / 1_000_000_000
  return t('subscription.discount.usage', {
    used: used.toFixed(2),
    remaining: remaining.toFixed(2),
    cap: Math.round(s.gold_discount_cap_nano / 1_000_000_000),
  })
})
</script>

<template>
  <div class="nest-subscription nest-admin">
    <header class="nest-subscription-head">
      <h1 class="nest-h1">{{ t('subscription.title') }}</h1>
      <p class="nest-subscription-tagline">{{ t('subscription.tagline') }}</p>

      <!-- Active subscription summary — shown only when on plus/pro. -->
      <div
        v-if="state && state.level"
        class="nest-subscription-active"
      >
        <v-icon size="18" class="mr-2">mdi-check-circle-outline</v-icon>
        <span>
          {{ t('subscription.active', { level: state.level.toUpperCase() }) }}
          <template v-if="expiresLabel">
            · {{ t('subscription.activeUntil', { date: expiresLabel }) }}
          </template>
        </span>
      </div>

      <!-- Discount cap usage hint — only relevant for active subs. -->
      <div v-if="discountHintShown" class="nest-subscription-discount-hint">
        <v-icon size="14" class="mr-1">mdi-piggy-bank-outline</v-icon>
        {{ discountHint }}
      </div>
    </header>

    <div class="nest-subscription-grid-wrap">
      <PlanCardsGrid />
    </div>

    <v-alert
      v-if="buyError"
      type="error"
      variant="tonal"
      density="compact"
      class="nest-subscription-error mt-6"
      closable
      @click:close="sub.dismissBuyError()"
    >
      {{ buyError }}
    </v-alert>

    <p class="nest-subscription-footnote">
      {{ t('subscription.footnote') }}
    </p>
  </div>
</template>

<style lang="scss" scoped>
.nest-subscription {
  min-height: 100dvh;
  background: var(--nest-bg);
  color: var(--nest-text);
  padding: 32px 24px 80px;
}

.nest-subscription-head {
  max-width: 1100px;
  margin: 0 auto 32px;
  text-align: center;
}
.nest-subscription-tagline {
  margin: 12px 0 0;
  color: var(--nest-text-secondary);
  font-size: 1rem;
  line-height: 1.55;
  max-width: 640px;
  margin-left: auto;
  margin-right: auto;
}
.nest-subscription-active {
  display: inline-flex;
  align-items: center;
  margin-top: 18px;
  padding: 6px 14px;
  border: 1px solid rgba(46, 197, 122, 0.4);
  border-radius: var(--nest-radius-pill);
  background: rgba(46, 197, 122, 0.08);
  font-size: 13px;
  color: var(--nest-text);
}
.nest-subscription-discount-hint {
  display: inline-flex;
  align-items: center;
  margin-top: 10px;
  font-size: 12.5px;
  color: var(--nest-text-secondary);
}

.nest-subscription-grid-wrap {
  max-width: 1100px;
  margin: 0 auto;
}

@media (max-width: 720px) {
  .nest-subscription { padding: 20px 16px 60px; }
}

.nest-subscription-footnote {
  max-width: 720px;
  margin: 32px auto 0;
  text-align: center;
  font-size: 12px;
  color: var(--nest-text-muted);
  line-height: 1.5;
}
</style>
