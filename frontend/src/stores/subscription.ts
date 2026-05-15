import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { apiFetch, type LimitReachedDetail } from '@/api/client'

/**
 * Subscription store (M54).
 *
 * Owns two concerns:
 *
 *  1. The detailed subscription state — level, expiry, monthly gold-
 *     discount cap usage. Fetched lazily from WuApi (proxied through
 *     WuNest's own /api/me/subscription endpoint) when the user opens
 *     the /subscription page or the gold-purchase flow.
 *
 *  2. A globally-accessible "limit reached" dialog. Any handler that
 *     catches an ApiError 402 with kind=limit_reached can call
 *     `showLimitReached(detail)` and the dialog mounted at app root
 *     opens with the right copy. Keeps every CRUD page from carrying
 *     its own dialog markup.
 *
 * Source of truth for the level itself stays on WuApi — we just cache
 * the response here so UI doesn't re-fetch on every check.
 */

export interface SubscriptionState {
  level: string | null
  expires_at: string | null
  slot_limit: number
  gold_discount_pct: number
  gold_discount_cap_nano: number
  gold_discount_used_nano: number
  gold_discount_remaining_nano: number
  period_start: string
}

/** A single tier row from WuApi /api/catalog/pricing (`wuapi_tiers`). */
export interface NestPlan {
  level: string
  amount_rub: number
  name: string
  slot_limit: number
  gold_discount_pct: number
  gold_discount_cap: number
  gold_discount_unlimited?: boolean
}

// Same-origin proxy to WuApi catalog pricing (WuNest server forwards).
const PRICING_ENDPOINT = '/api/pricing'

const WUAPI_SUBSCRIBE_URL = (import.meta.env.VITE_WUAPI_URL as string) || 'https://api.wuproj.com/dashboard'

export const useSubscriptionStore = defineStore('subscription', () => {
  const state = ref<SubscriptionState | null>(null)
  const plans = ref<NestPlan[]>([])
  const loading = ref(false)
  const plansLoading = ref(false)
  const limitReached = ref<LimitReachedDetail | null>(null)
  const buyingLevel = ref<string | null>(null)
  const buyError = ref<string | null>(null)

  /** Active WuApi tier key from /api/me/subscription, or "free". */
  const level = computed(() => state.value?.level ?? 'free')

  /** Per-resource slot cap from current state. Always >= 0 (unlimited
   *  is exposed as Infinity). Used by the Library usage hints. */
  const slotLimit = computed<number>(() => {
    const n = state.value?.slot_limit ?? 3
    return n < 0 ? Infinity : n
  })

  async function fetchState(): Promise<SubscriptionState | null> {
    loading.value = true
    try {
      const data = await apiFetch<SubscriptionState>('/api/me/subscription')
      state.value = data
      return data
    } catch (e) {
      console.warn('subscription.fetchState', e)
      return null
    } finally {
      loading.value = false
    }
  }

  /**
   * fetchPlans — pull the public pricing catalog from WuApi. The free
   * row comes first so consumers don't have to special-case it. Cached
   * after first successful call (plans rarely change; SPA can refresh
   * on a hard reload if WuApi adds a new tier).
   */
  async function fetchPlans(): Promise<NestPlan[]> {
    if (plans.value.length > 0) return plans.value
    plansLoading.value = true
    try {
      const res = await fetch(PRICING_ENDPOINT, { credentials: 'include' })
      if (!res.ok) throw new Error(`catalog ${res.status}`)
      const data = await res.json() as {
        wuapi_tiers?: Array<{
          key: string
          name: string
          amount_rub_monthly: number
          slot_limit: number
          gold_discount_pct: number
          gold_discount_cap_nano: number
          gold_discount_unlimited?: boolean
        }>
      }
      const rows = data?.wuapi_tiers ?? []
      const mapped: NestPlan[] = rows
        .filter((r) => r.key && r.key !== 'free')
        .map((r) => ({
          level: r.key,
          amount_rub: r.amount_rub_monthly ?? 0,
          name: r.name || r.key,
          slot_limit: r.slot_limit ?? 3,
          gold_discount_pct: r.gold_discount_pct ?? 0,
          gold_discount_cap: r.gold_discount_cap_nano ?? 0,
          gold_discount_unlimited: !!r.gold_discount_unlimited,
        }))
      const free: NestPlan = {
        level: 'free',
        amount_rub: 0,
        name: 'Free',
        slot_limit: 3,
        gold_discount_pct: 0,
        gold_discount_cap: 0,
      }
      const order: Record<string, number> = {
        free: 0, start: 1, plus: 2, pro: 3, max: 4, max_plus: 5,
      }
      const list = [free, ...mapped].sort(
        (a, b) => (order[a.level] ?? 99) - (order[b.level] ?? 99),
      )
      plans.value = list
      return list
    } catch (e) {
      console.warn('subscription.fetchPlans', e)
      return []
    } finally {
      plansLoading.value = false
    }
  }

  /** Opens WuApi checkout in a new tab — Nest plans are no longer sold here. */
  async function purchase(_level?: string) {
    buyError.value = null
    buyingLevel.value = null
    window.open(WUAPI_SUBSCRIBE_URL, '_blank', 'noopener')
  }

  function dismissBuyError() {
    buyError.value = null
  }

  function showLimitReached(detail: LimitReachedDetail) {
    limitReached.value = detail
  }

  function dismissLimitReached() {
    limitReached.value = null
  }

  return {
    state,
    plans,
    loading,
    plansLoading,
    level,
    slotLimit,
    limitReached,
    buyingLevel,
    buyError,
    fetchState,
    fetchPlans,
    purchase,
    dismissBuyError,
    showLimitReached,
    dismissLimitReached,
  }
})
