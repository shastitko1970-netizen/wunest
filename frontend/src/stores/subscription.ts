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
  level: 'plus' | 'pro' | null
  expires_at: string | null
  slot_limit: number
  gold_discount_pct: number
  gold_discount_cap_nano: number
  gold_discount_used_nano: number
  gold_discount_remaining_nano: number
  period_start: string
}

/** A single tier row from WuApi's /api/pay/prices endpoint
 *  (`nest_subscriptions` array). Free is included as a synthetic row so
 *  the Subscription page can render a uniform 3-card comparison grid. */
export interface NestPlan {
  level: 'free' | 'plus' | 'pro'
  amount_rub: number
  label: string
  slot_limit: number       // -1 = unlimited
  gold_discount_pct: number
  gold_discount_cap: number // nano-gold per month
}

// Same-origin proxy to WuApi's /api/pay/prices (see internal/server/
// router.go:handlePricing). Hitting api.wusphere.ru directly fails CORS
// in the browser; the WuNest backend just forwards the request and the
// SPA stays on one origin.
const PRICING_ENDPOINT = '/api/pricing'

export const useSubscriptionStore = defineStore('subscription', () => {
  const state = ref<SubscriptionState | null>(null)
  const plans = ref<NestPlan[]>([])
  const loading = ref(false)
  const plansLoading = ref(false)
  const limitReached = ref<LimitReachedDetail | null>(null)
  // Purchase flow state, shared between Subscription page and Account
  // page so both surfaces show the same loading/error feedback when a
  // user clicks Buy from either place.
  const buyingLevel = ref<'plus' | 'pro' | null>(null)
  const buyError = ref<string | null>(null)

  /** Active level — null when free, "plus" or "pro" when subscribed. */
  const level = computed<'free' | 'plus' | 'pro'>(() => {
    return state.value?.level ?? 'free'
  })

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
      // Non-fatal — the SPA falls back to "free" defaults (slot_limit=3
      // shows 3-cap hints, level=null). Real CRUD operations still go
      // through server-side enforcement which is the truth.
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
      // Same-origin via WuNest's pricing proxy — bypasses cross-origin
      // CORS against api.wusphere.ru. apiFetch handles error envelopes
      // but the catalog is plain JSON so we just .json() the response.
      const res = await fetch(PRICING_ENDPOINT, { credentials: 'include' })
      if (!res.ok) throw new Error(`catalog ${res.status}`)
      const data = await res.json()
      const list = (data?.nest_subscriptions ?? []) as NestPlan[]
      // Order: free → plus → pro for stable card layout.
      const order: Record<string, number> = { free: 0, plus: 1, pro: 2 }
      list.sort((a, b) => (order[a.level] ?? 99) - (order[b.level] ?? 99))
      plans.value = list
      return list
    } catch (e) {
      console.warn('subscription.fetchPlans', e)
      return []
    } finally {
      plansLoading.value = false
    }
  }

  /**
   * purchase — start a Yookassa checkout for the given level (M54.4).
   *
   * Lives in the store (not in a single component) so multiple surfaces
   * — `/subscription` and the inline plans block on `/account` — can
   * share buyingLevel + buyError state. The user shouldn't see a stuck
   * spinner on one page just because they clicked Buy on the other.
   *
   * On success: hard-redirects via `window.location.assign(payment_url)`.
   * On failure: leaves `buyError` set, clears `buyingLevel`, resolves
   * (no throw) so callers can stay simple.
   */
  async function purchase(level: 'plus' | 'pro') {
    if (buyingLevel.value) return
    buyError.value = null
    buyingLevel.value = level
    try {
      const res = await fetch('/api/pay/create', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ type: 'nest_subscription', nest_level: level }),
      })
      const data = await res.json().catch(() => ({})) as { payment_url?: string; error?: string }
      if (!res.ok || !data.payment_url) {
        buyError.value = data.error || `payment ${res.status}`
        buyingLevel.value = null
        return
      }
      // Hard redirect — Yookassa hosted checkout. After payment user
      // lands back on /subscription via the configured return URL.
      window.location.assign(data.payment_url)
    } catch (e) {
      console.error('subscription.purchase', e)
      buyError.value = (e as Error).message || 'payment failed'
      buyingLevel.value = null
    }
  }

  function dismissBuyError() {
    buyError.value = null
  }

  /**
   * showLimitReached — open the global "you hit your slot cap" dialog.
   * Called by any handler that caught an ApiError(402, kind=limit_reached).
   * Use isLimitReached() from @/api/client to type-narrow the error.
   */
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
