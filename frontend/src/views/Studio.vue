<script setup lang="ts">
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
</script>

<template>
  <v-container class="nest-stub text-center">
    <v-icon size="48" color="surface-variant">mdi-wrench-outline</v-icon>
    <h1 class="nest-h1 mt-4">{{ t('studio.title') }}</h1>
    <p class="nest-subtitle mt-2">
      {{ t('studio.tagline') }}<br />
      {{ t('studio.coming') }}
    </p>
  </v-container>
</template>

<style lang="scss" scoped>
.nest-stub {
  max-width: 640px;
  padding: 80px 24px;
}
</style>
