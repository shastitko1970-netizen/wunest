import { apiFetch } from '@/api/client'

// ─── Profile (GET /api/me) ────────────────────────────────────────────

export interface UserProfile {
  wuapi_user_id: number
  username: string
  first_name: string
  tier: string
  tier_expires_at?: string | null
  gold_balance_nano: number
  referral_count: number
  daily_limit: number
  used_today: number
  created_at: string
}

// ─── Stats (GET /api/me/stats) — WuApi pass-through ──────────────────

/**
 * Payload shape mirrors WuApi's auth.go HandleStats response. We keep it
 * loosely typed because the upstream may add fields we want to surface
 * without a WuNest release.
 */
export interface UsageStats {
  tokens: {
    day: number
    week: number
    month: number
    total: number
  }
  models?: ModelUsage[]
  referralCount?: number
}

export interface ModelUsage {
  model: string
  day: number
  week: number
  total: number
}

// ─── Gold transactions (GET /api/me/gold/transactions) ───────────────

export interface GoldTransaction {
  id: number
  deltaNano: number
  balanceAfterNano: number
  kind: string            // purchase | spend | refund | admin_grant | promo | dedupe_replay
  refID: string
  model: string
  inputTokens: number
  outputTokens: number
  costUSD: number
  orCostUSD: number
  description: string
  createdAt: string
}

// ─── API methods ─────────────────────────────────────────────────────

export const accountApi = {
  me: () => apiFetch<UserProfile>('/api/me'),
  stats: () => apiFetch<UsageStats>('/api/me/stats'),
  goldTransactions: (limit = 50, offset = 0) =>
    apiFetch<GoldTransaction[]>(
      `/api/me/gold/transactions?limit=${limit}&offset=${offset}`,
    ),
}
