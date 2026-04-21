<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useAuthStore } from '@/stores/auth'
import { useTheme } from 'vuetify'
import { computed } from 'vue'

const auth = useAuthStore()
const { profile } = storeToRefs(auth)
const vTheme = useTheme()

const currentTheme = computed({
  get: () => vTheme.global.name.value,
  set: (v: string) => { vTheme.global.name.value = v; localStorage.setItem('nest:theme', v) },
})

const goldDisplay = computed(() =>
  profile.value ? (profile.value.gold_balance_nano / 1_000_000_000).toFixed(2) : '—',
)
</script>

<template>
  <v-container class="nest-settings">
    <div class="nest-eyebrow">Settings</div>
    <h1 class="nest-h1 mt-1">Preferences</h1>

    <section class="nest-section" v-if="profile">
      <h2 class="nest-h2">Account</h2>
      <div class="nest-field-row">
        <div class="nest-field-label">Signed in as</div>
        <div class="nest-field-value">{{ profile.first_name || profile.username }}</div>
      </div>
      <div class="nest-field-row">
        <div class="nest-field-label">Tier</div>
        <div class="nest-field-value nest-mono">{{ profile.tier }}</div>
      </div>
      <div class="nest-field-row">
        <div class="nest-field-label">Gold balance</div>
        <div class="nest-field-value nest-mono">{{ goldDisplay }}</div>
      </div>
      <div class="nest-field-row">
        <div class="nest-field-label">Today's usage</div>
        <div class="nest-field-value nest-mono">
          {{ profile.used_today }} / {{ profile.daily_limit || '∞' }}
        </div>
      </div>
      <v-btn
        variant="outlined"
        class="mt-2"
        append-icon="mdi-open-in-new"
        href="https://wusphere.ru/dashboard"
        target="_blank"
      >
        Manage on WuSphere
      </v-btn>
    </section>

    <section class="nest-section">
      <h2 class="nest-h2">Theme</h2>
      <v-radio-group v-model="currentTheme" hide-details>
        <v-radio label="WuNest Dark" value="nestDark" />
        <v-radio label="WuNest Light" value="nestLight" />
      </v-radio-group>
    </section>

    <section class="nest-section">
      <h2 class="nest-h2">BYOK (bring-your-own-key)</h2>
      <p class="nest-subtitle">
        Use your own provider keys instead of WuApi. Useful if you have an OpenAI or Claude plan.
      </p>
      <v-alert type="info" variant="tonal" density="compact" class="mt-3">
        BYOK management is coming in M5.
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
}
.nest-field-row {
  display: grid;
  grid-template-columns: 180px 1fr;
  padding: 8px 0;
  border-bottom: 1px solid var(--nest-border-subtle);
  align-items: baseline;
}
.nest-field-label {
  font-family: var(--nest-font-mono);
  font-size: 11px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--nest-text-muted);
}
.nest-field-value {
  color: var(--nest-text);
}
</style>
