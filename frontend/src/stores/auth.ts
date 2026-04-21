import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiFetch } from '@/api/client'

export interface UserProfile {
  wuapi_user_id: number
  username: string
  first_name: string
  tier: string
  gold_balance_nano: number
  daily_limit: number
  used_today: number
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

  function redirectToLogin() {
    if (loginUrl.value) window.location.href = loginUrl.value
  }

  return { authenticated, loading, loginUrl, profile, check, redirectToLogin }
})
