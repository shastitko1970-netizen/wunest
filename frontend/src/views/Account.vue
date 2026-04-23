<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useAccountStore } from '@/stores/account'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const account = useAccountStore()
const auth = useAuthStore()
const {
  profile, stats, transactions,
  loading, goldDisplay, memberSince, tierExpiresDisplay, quotaRemaining,
} = storeToRefs(account)
const { nestAccessGranted } = storeToRefs(auth)

onMounted(() => account.refreshAll())

// Redeem-code form state. Lives on Account because that's the only page
// an un-activated user can reach alongside /, /docs; they'll find the
// field on their own profile page.
const redeemCode = ref('')
const redeemBusy = ref(false)
const redeemError = ref<string | null>(null)

async function submitRedeem() {
  redeemError.value = null
  redeemBusy.value = true
  try {
    await auth.redeemAccessCode(redeemCode.value)
    redeemCode.value = ''
    // profile is refreshed inside the store; AppShell's gate flips
    // automatically on the next reactive read of nestAccessGranted.
  } catch (e) {
    // apiFetch surfaces the upstream body as the message. WuApi returns
    // {"error":"code not found"} / {"error":"code already used"}; try
    // to extract that, fall back to the raw message.
    const raw = (e as Error).message
    try {
      const parsed = JSON.parse(raw) as { error?: string }
      redeemError.value = parsed.error ?? raw
    } catch {
      redeemError.value = raw
    }
  } finally {
    redeemBusy.value = false
  }
}

const topModels = computed(() => {
  const m = stats.value?.models ?? []
  return [...m].sort((a, b) => b.total - a.total).slice(0, 6)
})

function formatNano(nano: number): string {
  const g = nano / 1_000_000_000
  if (g === 0) return '0'
  const sign = nano < 0 ? '−' : '+'
  return `${sign}${Math.abs(g).toFixed(4)}`
}

function formatCost(usd: number): string {
  if (!usd) return '—'
  return '$' + usd.toFixed(6)
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString([], {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function kindChipColor(kind: string): string {
  switch (kind) {
    case 'purchase':    return 'secondary'
    case 'spend':       return ''
    case 'refund':      return 'success'
    case 'admin_grant': return 'info'
    case 'promo':       return 'info'
    default:            return ''
  }
}
</script>

<template>
  <v-container class="nest-account" fluid>
    <!-- Header -->
    <div class="nest-account-head">
      <div>
        <div class="nest-eyebrow">{{ t('account.title') }}</div>
        <h1 class="nest-h1 mt-1">
          {{ profile?.first_name || profile?.username || t('account.titleFallback') }}
        </h1>
        <p v-if="memberSince" class="nest-subtitle mt-1">
          {{ t('account.memberSince', { date: memberSince }) }}
        </p>
      </div>
      <v-btn
        variant="text"
        :loading="loading.profile || loading.stats || loading.transactions"
        prepend-icon="mdi-refresh"
        @click="account.refreshAll()"
      >
        {{ t('account.refresh') }}
      </v-btn>
    </div>

    <!-- Access code section. Shown prominently when the user hasn't
         activated yet; collapses to a tiny "access granted" badge once
         they have so the Account page isn't cluttered. -->
    <section v-if="!nestAccessGranted" class="nest-access-card">
      <div class="nest-eyebrow">{{ t('account.access.title') }}</div>
      <p class="nest-subtitle mt-2">{{ t('account.access.body') }}</p>
      <div class="nest-access-form mt-3">
        <v-text-field
          v-model="redeemCode"
          :placeholder="t('account.access.placeholder')"
          :disabled="redeemBusy"
          density="compact"
          hide-details
          spellcheck="false"
          class="nest-access-input"
          @keyup.enter="submitRedeem"
        />
        <v-btn
          color="primary"
          variant="flat"
          :loading="redeemBusy"
          :disabled="!redeemCode.trim()"
          @click="submitRedeem"
        >
          {{ t('account.access.submit') }}
        </v-btn>
      </div>
      <v-alert
        v-if="redeemError"
        type="error"
        variant="tonal"
        density="compact"
        class="mt-3"
      >
        {{ redeemError }}
      </v-alert>
    </section>
    <section v-else class="nest-access-activated">
      <v-icon size="18" color="success">mdi-check-circle</v-icon>
      <span>{{ t('account.access.activatedBadge') }}</span>
    </section>

    <!-- Primary KPI row -->
    <div class="nest-kpi-grid">
      <!-- Gold -->
      <v-card class="nest-kpi-card nest-kpi-card--gold pa-5">
        <div class="nest-kpi-label">{{ t('account.kpi.gold') }}</div>
        <div class="nest-kpi-value nest-mono">{{ goldDisplay }}</div>
        <div class="nest-kpi-unit">{{ t('account.kpi.goldSub') }}</div>
        <a
          class="nest-kpi-action"
          href="https://wusphere.ru/dashboard"
          target="_blank"
          rel="noopener"
        >
          {{ t('account.kpi.topUp') }}
        </a>
      </v-card>

      <!-- Tier -->
      <v-card class="nest-kpi-card pa-5">
        <div class="nest-kpi-label">{{ t('account.kpi.tier') }}</div>
        <div class="nest-kpi-value">{{ profile?.tier ?? '—' }}</div>
        <div v-if="tierExpiresDisplay" class="nest-kpi-unit">
          {{ t('account.kpi.tierExpires', { date: tierExpiresDisplay }) }}
        </div>
        <div v-else class="nest-kpi-unit">{{ t('account.kpi.tierNoExpiration') }}</div>
        <a
          class="nest-kpi-action"
          href="https://wusphere.ru/dashboard"
          target="_blank"
          rel="noopener"
        >
          {{ t('account.kpi.manage') }}
        </a>
      </v-card>

      <!-- Today quota -->
      <v-card class="nest-kpi-card pa-5">
        <div class="nest-kpi-label">{{ t('account.kpi.today') }}</div>
        <div class="nest-kpi-value nest-mono">
          <template v-if="profile && profile.daily_limit">
            {{ profile.used_today }} <span class="nest-kpi-unit-inline">/ {{ profile.daily_limit }}</span>
          </template>
          <template v-else>—</template>
        </div>
        <div v-if="quotaRemaining !== null" class="nest-kpi-unit">
          {{ t('account.kpi.todayLeft', { n: quotaRemaining }) }}
        </div>
      </v-card>

      <!-- Referrals -->
      <v-card class="nest-kpi-card pa-5">
        <div class="nest-kpi-label">{{ t('account.kpi.referrals') }}</div>
        <div class="nest-kpi-value nest-mono">{{ profile?.referral_count ?? 0 }}</div>
        <div class="nest-kpi-unit">{{ t('account.kpi.referralsUnit') }}</div>
      </v-card>
    </div>

    <!-- Token usage -->
    <section v-if="stats?.tokens" class="nest-section">
      <h2 class="nest-h2">{{ t('account.sections.tokenUsage') }}</h2>
      <div class="nest-token-grid">
        <div class="nest-token-cell">
          <div class="nest-token-label">{{ t('account.periods.today') }}</div>
          <div class="nest-token-value nest-mono">{{ stats.tokens.day.toLocaleString() }}</div>
        </div>
        <div class="nest-token-cell">
          <div class="nest-token-label">{{ t('account.periods.week') }}</div>
          <div class="nest-token-value nest-mono">{{ stats.tokens.week.toLocaleString() }}</div>
        </div>
        <div class="nest-token-cell">
          <div class="nest-token-label">{{ t('account.periods.month') }}</div>
          <div class="nest-token-value nest-mono">{{ stats.tokens.month.toLocaleString() }}</div>
        </div>
        <div class="nest-token-cell">
          <div class="nest-token-label">{{ t('account.periods.total') }}</div>
          <div class="nest-token-value nest-mono">{{ stats.tokens.total.toLocaleString() }}</div>
        </div>
      </div>
    </section>

    <!-- Top models -->
    <section v-if="topModels.length" class="nest-section">
      <h2 class="nest-h2">{{ t('account.sections.topModels') }}</h2>
      <div class="nest-models-list">
        <div v-for="m in topModels" :key="m.model" class="nest-models-row">
          <div class="nest-models-name nest-mono">{{ m.model }}</div>
          <div class="nest-models-metrics nest-mono">
            <span>{{ t('account.models.today', { n: m.day }) }}</span>
            <span>·</span>
            <span>{{ t('account.models.week', { n: m.week }) }}</span>
            <span>·</span>
            <span>{{ t('account.models.total', { n: m.total }) }}</span>
          </div>
        </div>
      </div>
    </section>

    <!-- Gold transactions -->
    <section v-if="transactions.length" class="nest-section">
      <h2 class="nest-h2">{{ t('account.sections.transactions') }}</h2>
      <div class="nest-txn-list">
        <div v-for="t2 in transactions" :key="t2.id" class="nest-txn-row">
          <div class="nest-txn-when nest-mono">{{ formatDate(t2.createdAt) }}</div>
          <div class="nest-txn-kind">
            <v-chip
              size="x-small"
              :color="kindChipColor(t2.kind)"
              variant="tonal"
              class="nest-mono"
            >
              {{ t2.kind }}
            </v-chip>
          </div>
          <div class="nest-txn-model nest-mono text-truncate">
            {{ t2.model || t2.description || '—' }}
          </div>
          <div class="nest-txn-tokens nest-mono">
            <template v-if="t2.inputTokens || t2.outputTokens">
              {{ t2.inputTokens }}/{{ t2.outputTokens }}
            </template>
            <template v-else>—</template>
          </div>
          <div class="nest-txn-cost nest-mono">{{ formatCost(t2.costUSD) }}</div>
          <div
            class="nest-txn-delta nest-mono"
            :class="{ 'is-negative': t2.deltaNano < 0, 'is-positive': t2.deltaNano > 0 }"
          >
            {{ formatNano(t2.deltaNano) }}
          </div>
        </div>
      </div>
    </section>

    <!-- External link -->
    <section class="nest-section">
      <h2 class="nest-h2">{{ t('account.sections.manageTitle') }}</h2>
      <p class="nest-subtitle">{{ t('account.sections.manageTagline') }}</p>
      <v-btn
        class="mt-3"
        variant="outlined"
        append-icon="mdi-open-in-new"
        href="https://wusphere.ru/dashboard"
        target="_blank"
        rel="noopener"
      >
        {{ t('account.sections.openWuApi') }}
      </v-btn>
    </section>
  </v-container>
</template>

<style lang="scss" scoped>
.nest-account {
  max-width: 960px;
  padding: 32px 24px 80px;
}

.nest-account-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  flex-wrap: wrap;
  gap: 16px;
  margin-bottom: 28px;
}

// ─── Access-code card ───────────────────────────────────────
// Prominent amber-outlined card while the user hasn't redeemed yet;
// collapses to a small success pill once activated. Lives at the top of
// Account so un-activated users don't miss it.
.nest-access-card {
  border: 1px dashed var(--nest-amber, #c9882a);
  background: rgba(201, 136, 42, 0.08);
  padding: 16px 18px;
  border-radius: var(--nest-radius);
  margin-bottom: 24px;
}
.nest-access-form {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.nest-access-input { flex: 1 1 240px; min-width: 0; }
.nest-access-activated {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border-radius: var(--nest-radius-pill);
  background: var(--nest-bg-elevated);
  border: 1px solid var(--nest-border-subtle);
  font-size: 12px;
  color: var(--nest-text-secondary);
  margin-bottom: 24px;
}

// ─── KPI cards ─────────────────────────────────────────────
.nest-kpi-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 12px;
  margin-bottom: 40px;
}

.nest-kpi-card {
  background: var(--nest-surface) !important;
  border: 1px solid var(--nest-border);
  border-radius: var(--nest-radius) !important;
  min-height: 140px;
  display: flex;
  flex-direction: column;
  justify-content: flex-start;
  position: relative;

  &--gold {
    background: linear-gradient(135deg, var(--nest-bg-elevated), var(--nest-surface)) !important;
    border-color: rgba(247, 201, 72, 0.3);
  }
  &--gold .nest-kpi-value { color: var(--nest-gold); }
}

.nest-kpi-label {
  font-family: var(--nest-font-mono);
  font-size: 10.5px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--nest-text-muted);
}

.nest-kpi-value {
  font-family: var(--nest-font-display);
  font-size: 2.2rem;
  font-weight: 400;
  line-height: 1.1;
  letter-spacing: -0.02em;
  color: var(--nest-text);
  margin: 8px 0 4px;
  font-feature-settings: 'tnum' 1;
}

.nest-kpi-unit {
  font-size: 11.5px;
  color: var(--nest-text-muted);
}
.nest-kpi-unit-inline {
  font-size: 16px;
  color: var(--nest-text-muted);
}

.nest-kpi-action {
  position: absolute;
  bottom: 14px;
  right: 16px;
  font-family: var(--nest-font-mono);
  font-size: 11px;
  color: var(--nest-accent);
  text-decoration: none;
  letter-spacing: 0.04em;

  &:hover { color: var(--nest-text); }
}

// ─── Sections ──────────────────────────────────────────────
.nest-section {
  margin-top: 40px;
  padding-top: 24px;
  border-top: 1px solid var(--nest-border);
}

// ─── Token usage row ───────────────────────────────────────
.nest-token-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 12px;
  margin-top: 16px;
}

.nest-token-cell {
  padding: 14px 16px;
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius);
  background: var(--nest-surface);
}

.nest-token-label {
  font-family: var(--nest-font-mono);
  font-size: 10.5px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--nest-text-muted);
}

.nest-token-value {
  font-size: 1.5rem;
  font-weight: 500;
  margin-top: 4px;
  color: var(--nest-text);
  font-feature-settings: 'tnum' 1;
}

// ─── Top models list ───────────────────────────────────────
.nest-models-list {
  margin-top: 12px;
  display: flex;
  flex-direction: column;
}

.nest-models-row {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  padding: 10px 0;
  border-bottom: 1px dashed var(--nest-border-subtle);
  gap: 12px;
}
.nest-models-row:last-child { border-bottom: 0; }

.nest-models-name {
  color: var(--nest-text);
  font-size: 13px;
}

.nest-models-metrics {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  font-size: 11.5px;
  color: var(--nest-text-muted);
}

// ─── Transaction list ──────────────────────────────────────
.nest-txn-list {
  margin-top: 12px;
  display: flex;
  flex-direction: column;
}

.nest-txn-row {
  display: grid;
  grid-template-columns: 120px 90px 1fr 90px 90px 100px;
  gap: 8px;
  padding: 8px 0;
  border-bottom: 1px solid var(--nest-border-subtle);
  font-size: 12px;
  align-items: baseline;
}
.nest-txn-row:last-child { border-bottom: 0; }

.nest-txn-when     { color: var(--nest-text-muted); }
.nest-txn-model    { color: var(--nest-text-secondary); overflow: hidden; white-space: nowrap; }
.nest-txn-tokens   { color: var(--nest-text-muted); text-align: right; }
.nest-txn-cost     { color: var(--nest-text-muted); text-align: right; }
.nest-txn-delta    { text-align: right; }

.is-positive { color: var(--nest-green); }
.is-negative { color: var(--nest-accent); }

@media (max-width: 720px) {
  .nest-account { padding: 20px 12px 60px; }

  .nest-txn-row {
    grid-template-columns: 1fr auto;
    grid-template-areas:
      "model delta"
      "when cost"
      "kind tokens";
  }
  .nest-txn-model   { grid-area: model; }
  .nest-txn-delta   { grid-area: delta; }
  .nest-txn-when    { grid-area: when; }
  .nest-txn-cost    { grid-area: cost; }
  .nest-txn-kind    { grid-area: kind; }
  .nest-txn-tokens  { grid-area: tokens; text-align: left; }
}
</style>
