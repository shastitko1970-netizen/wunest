<script setup lang="ts">
import { onMounted, computed, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useAuthStore } from '@/stores/auth'
import { useAppearanceStore } from '@/stores/appearance'
import AppShell from '@/layout/AppShell.vue'
import LoginGate from '@/views/LoginGate.vue'
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

const showShell = computed(() => !loading.value && authenticated.value)
const showLogin = computed(() => !loading.value && !authenticated.value)
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
    <AppShell v-else-if="showShell" />
    <LoginGate v-else-if="showLogin" />
  </v-app>
</template>

<style lang="scss">
.nest-boot {
  min-height: 100vh;
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
