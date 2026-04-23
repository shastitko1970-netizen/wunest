<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppearanceStore } from '@/stores/appearance'

// Top-of-app banner that only renders when the user navigated here with
// `?safe` in the URL (SAFE_MODE flag). Tells them custom CSS is off,
// lets them purge the stored custom CSS, and lets them exit safe mode.
//
// This is the escape hatch when a user-imported theme breaks WuNest's own
// shell. The banner styles inline with !important so even a truly hostile
// custom CSS can't hide it — though since SAFE_MODE suppresses the custom
// CSS entirely, that belt-and-braces is mostly belt.
const { t } = useI18n()
const appearance = useAppearanceStore()
const busy = ref(false)

async function resetCss() {
  busy.value = true
  try {
    appearance.update({ customCss: undefined })
  } finally {
    busy.value = false
  }
}

function exitSafeMode() {
  // Strip ?safe (and any compat ?nest-safe) from the URL and reload.
  const url = new URL(window.location.href)
  url.searchParams.delete('safe')
  url.searchParams.delete('nest-safe')
  window.location.replace(url.toString())
}
</script>

<template>
  <div class="nest-safe-banner">
    <span class="nest-safe-icon">⚠</span>
    <span class="nest-safe-text">{{ t('safe.banner') }}</span>
    <v-btn
      size="small"
      variant="text"
      color="white"
      :loading="busy"
      @click="resetCss"
    >
      {{ t('safe.reset') }}
    </v-btn>
    <v-btn
      size="small"
      variant="outlined"
      color="white"
      @click="exitSafeMode"
    >
      {{ t('safe.exit') }}
    </v-btn>
  </div>
</template>

<style lang="scss" scoped>
.nest-safe-banner {
  // Inline !important + high z-index so a rogue user CSS can't hide or
  // reorder this. Safe-mode also suppresses the custom CSS entirely, but
  // belt-and-braces costs nothing.
  position: fixed !important;
  top: 0 !important;
  left: 0 !important;
  right: 0 !important;
  z-index: 100000 !important;
  display: flex !important;
  align-items: center !important;
  gap: 10px !important;
  padding: 8px 14px !important;
  background: #d97706 !important; /* amber-600 — visible in both themes */
  color: #fff !important;
  font-family: sans-serif !important;
  font-size: 13px !important;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.25) !important;
  flex-wrap: wrap;
}
.nest-safe-icon { font-size: 16px; }
.nest-safe-text { flex: 1; min-width: 200px; }
</style>
