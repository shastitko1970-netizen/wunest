<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useSubscriptionStore, type NestPlan } from '@/stores/subscription'

// Reusable 3-card grid for WuNest plans (M54.5).
//
// Used on:
//   - /subscription (full page with footnote and active-sub banner)
//   - /account inline section (compact, no banner — Account already
//     surfaces the active plan in its KPI row)
//
// All purchase state lives in the subscription store so both surfaces
// share `buyingLevel` and `buyError` — a click on Account doesn't
// collide with a half-open redirect from /subscription.

const { t } = useI18n()
const sub = useSubscriptionStore()
const { plans, plansLoading, level } = storeToRefs(sub)

onMounted(() => {
  void sub.fetchPlans()
})

function isCurrent(plan: NestPlan): boolean {
  if (plan.level === 'free') return false
  return plan.level === level.value
}

function formatSlots(limit: number): string {
  if (limit < 0) return '∞'
  return String(limit)
}

function formatPrice(amountRub: number): string {
  if (amountRub === 0) return t('subscription.price.free')
  return `${amountRub} ₽`
}

function formatGoldCap(capNano: number): string {
  if (capNano <= 0) return ''
  return Math.round(capNano / 1_000_000_000).toString()
}

function onSubscribe(plan: NestPlan) {
  if (plan.level === 'free' || isCurrent(plan)) return
  void sub.purchase(plan.level)
}

const showLoading = computed(() => plansLoading.value && plans.value.length === 0)
</script>

<template>
  <div class="nest-plan-cards-wrap">
    <div v-if="showLoading" class="nest-plan-cards-loading">
      <v-progress-circular indeterminate size="28" />
    </div>

    <div v-else-if="plans.length === 0" class="nest-plan-cards-empty">
      {{ t('subscription.fetchFailed') }}
    </div>

    <div v-else class="nest-plan-cards-grid">
      <article
        v-for="plan in plans"
        :key="plan.level"
        class="nest-plan-card"
        :class="{
          'is-current': isCurrent(plan),
          'is-pro': plan.level === 'max' || plan.level === 'max_plus',
        }"
      >
        <div class="nest-plan-head">
          <span class="nest-plan-name">{{ plan.name }}</span>
          <v-chip
            v-if="isCurrent(plan)"
            size="x-small"
            color="primary"
            variant="flat"
          >
            {{ t('subscription.current') }}
          </v-chip>
        </div>

        <div class="nest-plan-price">
          <span class="nest-plan-price-amount">{{ formatPrice(plan.amount_rub) }}</span>
          <span v-if="plan.amount_rub > 0" class="nest-plan-price-period">
            / {{ t('subscription.period.month') }}
          </span>
        </div>

        <p class="nest-plan-tagline">
          {{ plan.level === 'free' ? t('subscription.plan.free.tagline') : t('subscription.wuapiTagline') }}
        </p>

        <ul class="nest-plan-features">
          <li>
            <v-icon size="14" class="nest-plan-feature-icon">mdi-account-multiple-outline</v-icon>
            {{ t('subscription.feature.slots', { count: formatSlots(plan.slot_limit) }) }}
          </li>
          <li v-if="plan.gold_discount_pct > 0">
            <v-icon size="14" class="nest-plan-feature-icon">mdi-piggy-bank-outline</v-icon>
            <template v-if="plan.gold_discount_unlimited">
              {{ t('subscription.feature.discountUnlimited', { percent: plan.gold_discount_pct }) }}
            </template>
            <template v-else>
              {{ t('subscription.feature.discount', {
                percent: plan.gold_discount_pct,
                cap: formatGoldCap(plan.gold_discount_cap),
              }) }}
            </template>
          </li>
          <li v-else-if="plan.level === 'free'">
            <v-icon size="14" class="nest-plan-feature-icon">mdi-information-outline</v-icon>
            {{ t('subscription.feature.noDiscount') }}
          </li>
          <li v-if="plan.level !== 'free'">
            <v-icon size="14" class="nest-plan-feature-icon">mdi-open-in-new</v-icon>
            {{ t('subscription.wuapiCtaHint') }}
          </li>
        </ul>

        <div class="nest-plan-cta">
          <v-btn
            v-if="isCurrent(plan)"
            block
            variant="tonal"
            disabled
          >
            {{ t('subscription.cta.current') }}
          </v-btn>
          <v-btn
            v-else-if="plan.level === 'free'"
            block
            variant="tonal"
            disabled
          >
            {{ t('subscription.cta.freeUnavailable') }}
          </v-btn>
          <v-btn
            v-else
            block
            color="primary"
            :variant="plan.level === 'max' || plan.level === 'max_plus' ? 'flat' : 'outlined'"
            :loading="false"
            :disabled="false"
            @click="onSubscribe(plan)"
          >
            {{ t('subscription.cta.openWuApi') }}
          </v-btn>
        </div>
      </article>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.nest-plan-cards-wrap { width: 100%; }

.nest-plan-cards-loading,
.nest-plan-cards-empty {
  display: grid;
  place-items: center;
  padding: 40px 20px;
  color: var(--nest-text-muted);
}

.nest-plan-cards-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
}

@media (max-width: 720px) {
  .nest-plan-cards-grid { grid-template-columns: 1fr; }
}

.nest-plan-card {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 22px 20px 20px;
  background: var(--nest-surface);
  border: 1px solid var(--nest-border);
  border-radius: var(--nest-radius);
  transition: border-color var(--nest-transition-fast), transform var(--nest-transition-fast);

  &:hover { transform: translateY(-2px); }
  &.is-current {
    border-color: var(--nest-accent);
    box-shadow: 0 0 0 2px rgba(var(--nest-accent-rgb, 37, 99, 235), 0.15);
  }
  &.is-pro {
    border-color: var(--nest-accent);
  }
}

.nest-plan-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.nest-plan-name {
  font-family: var(--nest-font-display);
  font-size: 1.2rem;
  letter-spacing: -0.005em;
}

.nest-plan-price {
  display: flex;
  align-items: baseline;
  gap: 6px;
}
.nest-plan-price-amount {
  font-family: var(--nest-font-display);
  font-size: 1.7rem;
  font-weight: 500;
  letter-spacing: -0.01em;
}
.nest-plan-price-period {
  font-size: 12.5px;
  color: var(--nest-text-muted);
}

.nest-plan-tagline {
  margin: 0;
  font-size: 13px;
  color: var(--nest-text-secondary);
  line-height: 1.5;
}

.nest-plan-features {
  list-style: none;
  padding: 0;
  margin: 4px 0 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
  border-top: 1px dashed var(--nest-border-subtle);
  padding-top: 12px;

  li {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    font-size: 13px;
    line-height: 1.5;
    color: var(--nest-text);
  }
}
.nest-plan-feature-icon {
  margin-top: 2px;
  color: var(--nest-text-muted);
  flex-shrink: 0;
}

.nest-plan-cta {
  margin-top: auto;
  padding-top: 6px;
}
</style>
