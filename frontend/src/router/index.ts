import { createRouter, createWebHistory } from 'vue-router'

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
