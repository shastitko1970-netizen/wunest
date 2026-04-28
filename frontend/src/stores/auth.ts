import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { apiFetch } from '@/api/client'

export interface UserProfile {
  wuapi_user_id: number
  username: string
  first_name: string
  tier: string
  gold_balance_nano: number
  daily_limit: number
  used_today: number
  /** Beta gate — TRUE once the user has redeemed a WuNest access code on
   *  WuApi. Chat/Library/Settings stay locked until this flips. */
  nest_access_granted: boolean
  /** M30: which preset is globally active per type (map: type → preset id).
   *  Missing key = no active preset for that type. Updated via the presets
   *  store (which calls PUT /api/me/defaults, shared endpoint). */
  active_presets: Record<string, string>
}

interface AuthCheckResponse {
  authenticated: boolean
  login_url?: string
}

export const useAuthStore = defineStore('auth', () => {
  const authenticated = ref(false)
  const loading = ref(true)
  const loginUrl = ref<string | null>(null)
  const profile = ref<UserProfile | null>(null)

  // Derived helper: convenience accessor for the closed-beta gate.
  // Disabled 2026-04-25 — public access is on, so this always reports
  // `true` for any authenticated user. Originally this read
  // `profile.value?.nest_access_granted === true` and gated chat /
  // library / generation endpoints; the original logic is kept in a
  // comment so re-enabling a closed beta is a single-line revert.
  //
  // Real access: `nest_access_granted` is still surfaced from /api/me
  // (it stays useful as user state for UI hints if we ever want them);
  // we just don't gate features on it any more.
  const nestAccessGranted = computed(() => authenticated.value)

  async function check() {
    loading.value = true
    try {
      const status = await apiFetch<AuthCheckResponse>('/api/auth/check')
      authenticated.value = status.authenticated
      loginUrl.value = status.login_url ?? null
      if (status.authenticated) {
        profile.value = await apiFetch<UserProfile>('/api/me')
      }
    } catch (err) {
      console.warn('auth check failed', err)
      authenticated.value = false
      profile.value = null
    } finally {
      loading.value = false
    }
  }

  /** Redeem a WuNest access code. On success the server returns the
   *  updated profile which we swap in. On failure the upstream status is
   *  preserved so the UI can distinguish "not found" (404) from
   *  "already used" (409) from unexpected errors. */
  async function redeemAccessCode(code: string): Promise<void> {
    const trimmed = code.trim()
    if (!trimmed) throw new Error('empty code')
    // apiFetch throws on non-2xx with the response body as the error message.
    await apiFetch<void>('/api/me/nest-access/redeem', {
      method: 'POST',
      body: JSON.stringify({ code: trimmed }),
    })
    // Re-fetch the profile so nest_access_granted flips locally without
    // a full page reload.
    profile.value = await apiFetch<UserProfile>('/api/me')
  }

  function redirectToLogin() {
    if (loginUrl.value) window.location.href = loginUrl.value
  }

  return {
    authenticated, loading, loginUrl, profile, nestAccessGranted,
    check, redeemAccessCode, redirectToLogin,
  }
})
