<script setup lang="ts">
import { onMounted, computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useAuthStore } from '@/stores/auth'
import AppShell from '@/layout/AppShell.vue'
import LoginGate from '@/views/LoginGate.vue'

const auth = useAuthStore()
const { authenticated, loading } = storeToRefs(auth)

// Boot: check session once.
onMounted(() => {
  auth.check()
})

const showShell = computed(() => !loading.value && authenticated.value)
const showLogin = computed(() => !loading.value && !authenticated.value)
</script>

<template>
  <v-app>
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
