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
    // Public theme gallery (M42.6). Shareable link for mod authors,
    // doubles as a vibe-check page so anon visitors can see what the
    // 5 built-in presets actually look like without signing up. Apply
    // button redirects through the login flow for anon users.
    {
      path: '/themes',
      name: 'themes',
      component: () => import('@/views/Themes.vue'),
      meta: { public: true },
    },

    // ── Authed ──────────────────────────────────────────────────────
    { path: '/chat', name: 'chat', component: () => import('@/views/Chat.vue') },
    { path: '/chat/:id', name: 'chat-detail', component: () => import('@/views/Chat.vue') },
    { path: '/library', name: 'library', component: () => import('@/views/Library.vue') },
    { path: '/account', name: 'account', component: () => import('@/views/Account.vue') },
    // M54.2 — placeholder for the future plans gallery. The limit-reached
    // dialog routes here on Upgrade click, so the URL must exist before
    // M54.3 wires up the real comparison cards.
    { path: '/subscription', name: 'subscription', component: () => import('@/views/Subscription.vue') },
    { path: '/presets', name: 'presets', component: () => import('@/views/Presets.vue') },
    { path: '/settings', name: 'settings', component: () => import('@/views/Settings.vue') },
    // M43 — theme converter. Auth-required so we can bill tokens to the
    // signed-in user. Not beta-gated (user can test ST→WuNest conversions
    // before fully activating).
    { path: '/convert', name: 'convert', component: () => import('@/views/Converter.vue') },
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

// ── Stale-bundle recovery ─────────────────────────────────────────────
//
// After a deploy, hashed chunk filenames change. Any tab that's still
// running the PRE-deploy index.html will try to dynamic-import chunk
// names that no longer exist (e.g. Account-B-lf6Oog.js → 404). Vue
// Router surfaces that as an onError with message "Failed to fetch
// dynamically imported module".
//
// The fix is simple: the user has an outdated entrypoint in memory, so
// do a hard navigation to the target path. The browser re-requests
// index.html (no-cache, so it gets the fresh one from the server),
// which pulls the fresh chunk map.
//
// We only reload for dynamic-import failures to avoid a reload loop on
// actually-broken routes — other errors should surface normally.
const STALE_CHUNK_PATTERNS = [
  /Failed to fetch dynamically imported module/i,
  /Importing a module script failed/i,
  /error loading dynamically imported module/i,
]

router.onError((error, to) => {
  const msg = (error as Error | undefined)?.message ?? ''
  if (STALE_CHUNK_PATTERNS.some(r => r.test(msg))) {
    // Hard-navigate so index.html is re-fetched fresh. Use the intended
    // destination's fullPath (or the current URL as a fallback) so the
    // user lands where they were trying to go, not on the root.
    const target = to?.fullPath || window.location.pathname + window.location.search
    console.warn('[router] stale chunk, reloading to', target)
    window.location.replace(target)
  }
})
