<script setup lang="ts">
import { computed } from 'vue'
import { useTheme } from 'vuetify'
import { useI18n } from 'vue-i18n'
import AppearancePanel from '@/components/AppearancePanel.vue'

const { t, locale, availableLocales } = useI18n()
const vTheme = useTheme()

const currentTheme = computed({
  get: () => vTheme.global.name.value,
  set: (v: string) => {
    vTheme.global.name.value = v
    localStorage.setItem('nest:theme', v)
  },
})

const currentLocale = computed({
  get: () => locale.value,
  set: (v: string) => {
    locale.value = v
    localStorage.setItem('nest:locale', v)
  },
})

const localeLabel = (code: string) => {
  switch (code) {
    case 'ru': return 'Русский'
    case 'en': return 'English'
    default:   return code.toUpperCase()
  }
}
</script>

<template>
  <v-container class="nest-settings">
    <div class="nest-eyebrow">{{ t('nav.settings') }}</div>
    <h1 class="nest-h1 mt-1">{{ t('settings.title') }}</h1>

    <section class="nest-section">
      <h2 class="nest-h2">{{ t('settings.theme') }}</h2>
      <v-radio-group v-model="currentTheme" hide-details>
        <v-radio label="WuNest Dark" value="nestDark" />
        <v-radio label="WuNest Light" value="nestLight" />
      </v-radio-group>
    </section>

    <section class="nest-section">
      <h2 class="nest-h2">{{ t('settings.language') }}</h2>
      <v-radio-group v-model="currentLocale" hide-details>
        <v-radio
          v-for="code in availableLocales"
          :key="code"
          :label="localeLabel(code)"
          :value="code"
        />
      </v-radio-group>
    </section>

    <section class="nest-section">
      <AppearancePanel />
    </section>

    <section class="nest-section">
      <h2 class="nest-h2">{{ t('settings.byok.title') }}</h2>
      <p class="nest-subtitle">{{ t('settings.byok.tagline') }}</p>
      <v-alert type="info" variant="tonal" density="compact" class="mt-3">
        {{ t('settings.byok.coming') }}
      </v-alert>
    </section>
  </v-container>
</template>

<style lang="scss" scoped>
.nest-settings {
  max-width: 720px;
  padding: 32px 24px;
}
.nest-section {
  margin-top: 40px;
  padding-top: 24px;
  border-top: 1px solid var(--nest-border);
}
.nest-section:first-of-type {
  border-top: none;
  padding-top: 0;
  margin-top: 24px;
}
</style>
