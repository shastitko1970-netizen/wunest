import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  accountApi,
  type GoldTransaction,
  type UsageStats,
  type UserProfile,
} from '@/api/account'

// Account store is distinct from the auth store: auth only knows "am I
// logged in?" while this one holds the full profile + live stats + gold
// history for the Cabinet view. Everything here re-fetches on demand.

export const useAccountStore = defineStore('account', () => {
  const profile = ref<UserProfile | null>(null)
  const stats = ref<UsageStats | null>(null)
  const transactions = ref<GoldTransaction[]>([])

  const loading = ref({ profile: false, stats: false, transactions: false })
  const errors = ref({ profile: null as string | null, stats: null as string | null, transactions: null as string | null })

  const goldDisplay = computed(() =>
    profile.value ? (profile.value.gold_balance_nano / 1_000_000_000).toFixed(4) : '—',
  )

  const memberSince = computed(() => {
    if (!profile.value?.created_at) return ''
    return new Date(profile.value.created_at).toLocaleDateString()
  })

  const tierExpiresDisplay = computed(() => {
    const e = profile.value?.tier_expires_at
    if (!e) return null
    return new Date(e).toLocaleDateString()
  })

  const quotaRemaining = computed(() => {
    if (!profile.value) return null
    const limit = profile.value.daily_limit || 0
    const used = profile.value.used_today || 0
    return Math.max(0, limit - used)
  })

  async function fetchProfile() {
    loading.value.profile = true
    errors.value.profile = null
    try {
      profile.value = await accountApi.me()
    } catch (e) {
      errors.value.profile = (e as Error).message
    } finally {
      loading.value.profile = false
    }
  }

  async function fetchStats() {
    loading.value.stats = true
    errors.value.stats = null
    try {
      stats.value = await accountApi.stats()
    } catch (e) {
      errors.value.stats = (e as Error).message
    } finally {
      loading.value.stats = false
    }
  }

  async function fetchTransactions(limit = 20) {
    loading.value.transactions = true
    errors.value.transactions = null
    try {
      transactions.value = await accountApi.goldTransactions(limit, 0)
    } catch (e) {
      errors.value.transactions = (e as Error).message
    } finally {
      loading.value.transactions = false
    }
  }

  /** Refresh everything in parallel. Used by the Cabinet page's reload button. */
  async function refreshAll() {
    await Promise.all([fetchProfile(), fetchStats(), fetchTransactions()])
  }

  return {
    profile, stats, transactions, loading, errors,
    goldDisplay, memberSince, tierExpiresDisplay, quotaRemaining,
    fetchProfile, fetchStats, fetchTransactions, refreshAll,
  }
})
