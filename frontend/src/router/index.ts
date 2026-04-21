import { createRouter, createWebHistory } from 'vue-router'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/chat' },
    { path: '/chat', name: 'chat', component: () => import('@/views/Chat.vue') },
    { path: '/chat/:id', name: 'chat-detail', component: () => import('@/views/Chat.vue') },
    { path: '/library', name: 'library', component: () => import('@/views/Library.vue') },
    { path: '/settings', name: 'settings', component: () => import('@/views/Settings.vue') },
    { path: '/studio', name: 'studio', component: () => import('@/views/Studio.vue') },
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
