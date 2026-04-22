<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useTheme } from 'vuetify'
import { useI18n } from 'vue-i18n'
import AppearancePanel from '@/components/AppearancePanel.vue'
import BYOKPanel from '@/components/BYOKPanel.vue'
import { useModelsStore } from '@/stores/models'
import { apiFetch } from '@/api/client'

const { t, locale, availableLocales } = useI18n()
const vTheme = useTheme()
const models = useModelsStore()
const { options: modelOptions, loading: modelsLoading } = storeToRefs(models)

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

// ─── Default generation model ────────────────────────────────────────
// Stored server-side on nest_users.settings.default_model. Applied when
// a chat send doesn't specify a model and no chat-level override exists.
const defaultModel = ref<string>('')
const defaultModelSaving = ref(false)
const defaultModelSaved = ref(false)

onMounted(async () => {
  try {
    const r = await apiFetch<{ default_model: string }>('/api/me/default-model')
    defaultModel.value = r.default_model ?? ''
  } catch { /* non-fatal */ }
  if (!models.loaded) await models.fetchList()
})

// Options for the select: "— server default —" (empty string = clear the
// preference) plus every model from the catalogue. We don't try to merge
// the stored preference in if the catalogue doesn't have it; a stale id
// keeps working at generation time — the server just passes it through.
const defaultModelOptions = computed(() => [
  { id: '', title: t('settings.defaultModel.serverFallback') },
  ...modelOptions.value.map(m => ({ id: m.id, title: m.id })),
])

async function saveDefaultModel(v: string) {
  defaultModel.value = v
  defaultModelSaving.value = true
  try {
    await apiFetch('/api/me/default-model', {
      method: 'PUT',
      body: JSON.stringify({ default_model: v }),
    })
    defaultModelSaved.value = true
    setTimeout(() => (defaultModelSaved.value = false), 1500)
  } finally {
    defaultModelSaving.value = false
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
      <h2 class="nest-h2">{{ t('settings.defaultModel.title') }}</h2>
      <p class="nest-subtitle mb-3">{{ t('settings.defaultModel.tagline') }}</p>
      <v-select
        :model-value="defaultModel"
        :items="defaultModelOptions"
        item-title="title"
        item-value="id"
        :loading="modelsLoading || defaultModelSaving"
        density="compact"
        hide-details
        style="max-width: 360px"
        @update:model-value="saveDefaultModel"
      />
      <div v-if="defaultModelSaved" class="nest-mono text-success mt-2" style="font-size: 11px">
        {{ t('settings.defaultModel.saved') }}
      </div>
    </section>

    <section class="nest-section">
      <AppearancePanel />
    </section>

    <section class="nest-section">
      <BYOKPanel />
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
