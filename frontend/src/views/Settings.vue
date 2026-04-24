<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import AppearancePanel from '@/components/AppearancePanel.vue'
import BYOKPanel from '@/components/BYOKPanel.vue'
import { useModelsStore } from '@/stores/models'
import { usePreferencesStore } from '@/stores/preferences'
import { apiFetch } from '@/api/client'

const { t, locale, availableLocales } = useI18n()
const models = useModelsStore()
const prefs = usePreferencesStore()
const { disableStreaming } = storeToRefs(prefs)
const { items: modelOptions, loading: modelsLoading } = storeToRefs(models)

// Theme mode toggle жил здесь до этой итерации — M42.X добавил его как
// replacement для удалённой radio-group'ы. Тестер попросил убрать и
// его: «быстрая кнопка светла/тёмная» дублировала 5-preset grid из
// AppearancePanel ниже. Оставляем единый источник theme UX'а — picker
// в Appearance. Pair-logic в theme store живёт дальше, используется в
// /themes галерее как "pair for flip" мета.

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
  // Settings page isn't chat-scoped, so just show the WuApi pool here.
  // If the user wants to default to a BYOK-only model, they'd pin it per-chat.
  await models.setActiveSource('wuapi')
})

// Options for the select: "— server default —" (empty string = clear the
// preference) plus every model from the catalogue. We don't try to merge
// the stored preference in if the catalogue doesn't have it; a stale id
// keeps working at generation time — the server just passes it through.
const defaultModelOptions = computed(() => [
  { id: '', title: t('settings.defaultModel.serverFallback') },
  ...modelOptions.value.map((m: { id: string }) => ({ id: m.id, title: m.id })),
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
  <v-container class="nest-settings nest-admin">
    <div class="nest-eyebrow">{{ t('nav.settings') }}</div>
    <h1 class="nest-h1 mt-1">{{ t('settings.title') }}</h1>

    <!-- Theme picker лежит в AppearancePanel ниже — единый источник
         theme UX'а. Быстрый dark/light toggle был удалён как
         дублирующий, 5-preset grid делает ту же работу одним кликом. -->

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
        class="nest-settings-select"
        @update:model-value="saveDefaultModel"
      />
      <div v-if="defaultModelSaved" class="nest-mono text-success mt-2 nest-hint--xs">
        {{ t('settings.defaultModel.saved') }}
      </div>
    </section>

    <section class="nest-section">
      <h2 class="nest-h2">{{ t('settings.streaming.title') }}</h2>
      <p class="nest-subtitle mb-3">{{ t('settings.streaming.tagline') }}</p>
      <v-switch
        v-model="disableStreaming"
        :label="t('settings.streaming.disableLabel')"
        color="primary"
        inset
        hide-details
        density="compact"
      />
      <p class="nest-hint mt-2">
        {{ t('settings.streaming.disableHint') }}
      </p>
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

// Default-model select gets a fixed cap so it doesn't sprawl across
// the 720px column — was inline style="max-width: 360px".
.nest-settings-select {
  max-width: 360px;
}
</style>
