<script setup lang="ts">
import { useRouter } from 'vue-router'

const router = useRouter()
</script>

<template>
  <v-main>
    <v-container class="py-16 text-center">
      <h1 class="nest-h1">404</h1>
      <p class="nest-subtitle">Страница не найдена.</p>
      <v-btn variant="outlined" class="mt-6" @click="router.push('/')">
        На главную
      </v-btn>
    </v-container>
  </v-main>
</template>
