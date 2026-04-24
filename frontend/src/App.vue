<script setup lang="ts">
import { onMounted, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useAuthStore } from '@/stores/auth'
import { useAppearanceStore } from '@/stores/appearance'
import AppShell from '@/layout/AppShell.vue'
import SafeModeBanner from '@/components/SafeModeBanner.vue'

const auth = useAuthStore()
const appearance = useAppearanceStore()
const { authenticated, loading } = storeToRefs(auth)
const { safeMode } = storeToRefs(appearance)

// Boot: check session once.
onMounted(() => {
  auth.check()
})

// Once logged in, pull the saved appearance from the server. The store
// also reads localStorage eagerly so the first paint already reflects
// the last-used theme — this is the "catch up if we're out of sync"
// step after a cross-device login.
watch(authenticated, (ok) => {
  if (ok) void appearance.fetchFromServer()
}, { immediate: true })
</script>

<template>
  <v-app>
    <!-- Safe mode banner renders above everything when ?safe is in the URL.
         Custom CSS injection is already suppressed by the store; this UI
         just gives the user a way to purge the broken CSS or exit safe
         mode without typing a new URL. -->
    <SafeModeBanner v-if="safeMode" />

    <div v-if="loading" class="nest-boot">
      <div class="nest-boot-spinner">▲</div>
    </div>
    <!-- AppShell renders for everyone (authed or anon) so the main page
         looks consistent across login states. Inside, AppShell shows a
         "Sign in" CTA in the topbar when anon, and protected routes
         fall back to an inline prompt rather than stealing the whole
         viewport. The old standalone LoginGate is retained as a target
         when the user explicitly clicks Sign In. -->
    <AppShell v-else />
  </v-app>
</template>

<style lang="scss">
.nest-boot {
  // 100dvh — dynamic viewport height. On mobile Safari/Chrome 100vh
  // stays pegged to the "URL bar collapsed" height, so the boot screen
  // got clipped by 56px whenever the bar was visible. 100dvh reflows
  // correctly. DS rule: never 100vh, always 100dvh.
  min-height: 100dvh;
  display: grid;
  place-items: center;
  background: var(--nest-bg);
}
.nest-boot-spinner {
  color: var(--nest-accent);
  font-family: var(--nest-font-mono);
  font-size: 32px;
  animation: nest-pulse 1.2s ease-in-out infinite;
}
@keyframes nest-pulse {
  0%, 100% { opacity: 0.4; transform: scale(1); }
  50%      { opacity: 1;   transform: scale(1.1); }
}
</style>
