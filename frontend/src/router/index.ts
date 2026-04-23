import { watch } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

/**
 * Route table.
 *
 * `meta.public: true` — route is reachable without a session. App.vue's
 * auth gate renders these directly instead of falling through to
 * LoginGate, letting anonymous visitors read the landing + docs.
 */
export const router = createRouter({
  history: createWebHistory(),
  routes: [
    // Root: always public landing. Authed users use topbar to navigate to
    // /chat; the login flow returns directly to /chat so the landing
    // doesn't interrupt the "just signed in" path.
    {
      path: '/',
      name: 'home',
      component: () => import('@/views/Welcome.vue'),
      meta: { public: true },
    },

    // ── Public ──────────────────────────────────────────────────────
    {
      path: '/welcome',
      redirect: '/',
    },
    {
      path: '/docs',
      name: 'docs-index',
      component: () => import('@/views/Docs.vue'),
      meta: { public: true },
    },
    {
      path: '/docs/:slug',
      name: 'docs-page',
      component: () => import('@/views/Docs.vue'),
      meta: { public: true },
    },

    // ── Authed ──────────────────────────────────────────────────────
    { path: '/chat', name: 'chat', component: () => import('@/views/Chat.vue') },
    { path: '/chat/:id', name: 'chat-detail', component: () => import('@/views/Chat.vue') },
    { path: '/library', name: 'library', component: () => import('@/views/Library.vue') },
    { path: '/account', name: 'account', component: () => import('@/views/Account.vue') },
    { path: '/presets', name: 'presets', component: () => import('@/views/Presets.vue') },
    { path: '/settings', name: 'settings', component: () => import('@/views/Settings.vue') },
    {
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      component: () => import('@/views/NotFound.vue'),
    },
  ],
  scrollBehavior() {
    return { top: 0 }
  },
})

// ── Beta-gate navigation guard ───────────────────────────────────────
//
// Chat and Library require an activated WuNest access code. The disabled
// nav buttons cover the UI path, but without a router-level guard a user
// can still reach a chat by direct URL (bookmark, history, deeplink from
// another tab). Here we redirect gated paths to /account, where the
// redeem form lives, until nestAccessGranted flips to true.
//
// This is still client-side — real hard enforcement has to live in the
// chat/library backend handlers. But it stops the honest bypass case.
//
// Auth boot race: App.vue fires auth.check() on mount; for a cold hit on
// /chat/:id the router can fire before /api/me resolves. We wait out
// loading so the gate has ground truth before it decides to redirect.
const GATED_PREFIXES = ['/chat', '/library']
const isGated = (path: string) =>
  GATED_PREFIXES.some(p => path === p || path.startsWith(p + '/'))

router.beforeEach(async (to) => {
  const auth = useAuthStore()

  if (auth.loading) {
    await new Promise<void>((resolve) => {
      const unwatch = watch(() => auth.loading, (v) => {
        if (!v) { unwatch(); resolve() }
      })
    })
  }

  if (!auth.authenticated) return true         // anon → AppShell handles
  if (auth.nestAccessGranted) return true      // activated → free pass
  if (!isGated(to.path)) return true           // /, /docs, /account, /settings
  return { path: '/account' }
})
