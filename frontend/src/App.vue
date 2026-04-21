<script setup lang="ts">
import { onMounted } from 'vue'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()

onMounted(() => {
  // Check session on app boot. If the user has a valid wu_session cookie,
  // the store will be populated; otherwise the "login" action on the page
  // will redirect to wusphere.ru/login.
  auth.check()
})
</script>

<template>
  <v-app>
    <router-view />
  </v-app>
</template>
