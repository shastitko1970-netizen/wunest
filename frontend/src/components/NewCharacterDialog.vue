<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useCharactersStore } from '@/stores/characters'

const { t } = useI18n()

const props = defineProps<{
  modelValue: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'created', id: string): void
}>()

const store = useCharactersStore()

// Form state. Everything lives in reactive() so we can reset in one line.
const form = reactive({
  name: '',
  description: '',
  personality: '',
  scenario: '',
  first_mes: '',
  tags: '',
})

const busy = ref(false)
const error = ref<string | null>(null)

// Reset the form every time the dialog opens — fresh character each session.
watch(() => props.modelValue, (open) => {
  if (open) {
    form.name = ''
    form.description = ''
    form.personality = ''
    form.scenario = ''
    form.first_mes = ''
    form.tags = ''
    error.value = null
    busy.value = false
  }
})

function close() {
  emit('update:modelValue', false)
}

async function save() {
  const name = form.name.trim()
  if (!name) {
    error.value = t('library.create.nameRequired')
    return
  }

  busy.value = true
  error.value = null
  try {
    const tags = form.tags
      .split(',')
      .map(t2 => t2.trim())
      .filter(Boolean)

    const created = await store.create({
      name,
      data: {
        name,
        description: form.description.trim(),
        personality: form.personality.trim(),
        scenario: form.scenario.trim(),
        first_mes: form.first_mes.trim(),
        tags,
      },
      tags,
    })
    emit('created', created.id)
    close()
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <v-dialog
    :model-value="modelValue"
    max-width="640"
    scrollable
    @update:model-value="emit('update:modelValue', $event)"
  >
    <v-card class="nest-create">
      <v-card-title class="nest-create-title">
        <span>{{ t('library.create.title') }}</span>
        <v-btn icon="mdi-close" variant="text" size="small" @click="close" />
      </v-card-title>

      <v-card-text class="pb-0">
        <v-text-field
          v-model="form.name"
          :label="t('library.create.name')"
          :placeholder="t('library.create.namePlaceholder')"
          :error-messages="error && !form.name.trim() ? [error] : []"
          autofocus
          class="mb-2"
        />

        <v-textarea
          v-model="form.description"
          :label="t('library.create.description')"
          :placeholder="t('library.create.descriptionPlaceholder')"
          rows="3"
          auto-grow
          class="mb-2"
        />

        <v-textarea
          v-model="form.personality"
          :label="t('library.create.personality')"
          :placeholder="t('library.create.personalityPlaceholder')"
          rows="2"
          auto-grow
          class="mb-2"
        />

        <v-textarea
          v-model="form.scenario"
          :label="t('library.create.scenario')"
          :placeholder="t('library.create.scenarioPlaceholder')"
          rows="2"
          auto-grow
          class="mb-2"
        />

        <v-textarea
          v-model="form.first_mes"
          :label="t('library.create.firstMes')"
          :placeholder="t('library.create.firstMesPlaceholder')"
          rows="3"
          auto-grow
          class="mb-2"
        />

        <v-text-field
          v-model="form.tags"
          :label="t('library.create.tags')"
          :placeholder="t('library.create.tagsPlaceholder')"
          :hint="t('library.create.tagsHint')"
          persistent-hint
        />

        <v-alert
          v-if="error && form.name.trim()"
          type="error"
          variant="tonal"
          density="compact"
          class="mt-3"
        >
          {{ error }}
        </v-alert>
      </v-card-text>

      <v-card-actions class="px-6 pb-4 pt-4">
        <v-spacer />
        <v-btn variant="text" :disabled="busy" @click="close">
          {{ t('common.cancel') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="flat"
          :loading="busy"
          @click="save"
        >
          {{ t('library.create.createBtn') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<style lang="scss" scoped>
.nest-create {
  background: var(--nest-surface) !important;
  border: 1px solid var(--nest-border);
  border-radius: var(--nest-radius) !important;
}

.nest-create-title {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-family: var(--nest-font-display);
  font-size: 18px;
  padding: 20px 20px 8px;
}
</style>
