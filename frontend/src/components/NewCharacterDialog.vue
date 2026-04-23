<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useDisplay } from 'vuetify'
import { useCharactersStore } from '@/stores/characters'
import type { Character } from '@/api/characters'

// Character create / edit dialog. Same form serves both — when `character`
// is provided on open, we hydrate from it and PATCH on save; otherwise we
// start blank and POST a new row.
//
// Visual treatment: a compact multi-section form, grouped so the user
// fills Identity → Profile → Scene top-down. On mobile we switch to a
// full-width bottom sheet so the soft keyboard has room and fields
// don't float in a narrow centred card. Desktop keeps the centred
// dialog but wider (720px) so the three multi-line blocks stack cleanly.
const { t } = useI18n()
const { smAndDown } = useDisplay()

const props = defineProps<{
  modelValue: boolean
  /** When present, dialog opens in EDIT mode — form hydrates from this
   *  character and save calls store.update() instead of store.create(). */
  character?: Character | null
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'created', id: string): void
  (e: 'saved', id: string): void
}>()

const store = useCharactersStore()

const isEdit = computed(() => !!props.character)

// Everything in one reactive bag → single-line reset.
const form = reactive({
  name: '',
  avatar_url: '',
  tags: '',
  description: '',
  personality: '',
  scenario: '',
  first_mes: '',
})

const busy = ref(false)
const error = ref<string | null>(null)
const focusNameEl = ref<HTMLElement | null>(null)

watch(() => [props.modelValue, props.character] as const, ([open, ch]) => {
  if (!open) return
  if (ch) {
    // Edit mode — hydrate from the character.
    form.name = ch.name
    form.avatar_url = ch.avatar_url ?? ''
    form.tags = (ch.tags ?? []).join(', ')
    form.description = ch.data?.description ?? ''
    form.personality = ch.data?.personality ?? ''
    form.scenario = ch.data?.scenario ?? ''
    form.first_mes = ch.data?.first_mes ?? ''
  } else {
    form.name = ''
    form.avatar_url = ''
    form.tags = ''
    form.description = ''
    form.personality = ''
    form.scenario = ''
    form.first_mes = ''
  }
  error.value = null
  busy.value = false
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

    if (props.character) {
      // EDIT — PATCH the existing row, preserve data fields the form
      // doesn't surface (alternate_greetings, creator_notes, character_book,
      // extensions, etc.) by spreading the original data blob first.
      const updated = await store.update(props.character.id, {
        name,
        avatar_url: form.avatar_url.trim() || undefined,
        data: {
          ...props.character.data,
          name,
          description: form.description.trim(),
          personality: form.personality.trim(),
          scenario: form.scenario.trim(),
          first_mes: form.first_mes.trim(),
          tags,
        },
        tags,
      })
      emit('saved', updated.id)
    } else {
      // CREATE — fresh row.
      const created = await store.create({
        name,
        avatar_url: form.avatar_url.trim() || undefined,
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
      emit('saved', created.id)
    }
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
    :max-width="smAndDown ? undefined : 720"
    :fullscreen="smAndDown"
    scrollable
    @update:model-value="emit('update:modelValue', $event)"
  >
    <v-card class="nest-create">
      <v-card-title class="nest-create-title">
        <div>
          <div class="nest-eyebrow">{{ t('library.title') }}</div>
          <span class="nest-h3 mt-1">
            {{ isEdit ? t('library.edit.title') : t('library.create.title') }}
          </span>
        </div>
        <v-btn icon="mdi-close" variant="text" size="small" @click="close" />
      </v-card-title>

      <v-card-text class="nest-create-body">
        <!-- ─── Identity section ─── -->
        <section class="nest-create-section">
          <div class="nest-create-section-head">{{ t('library.create.section.identity') }}</div>
          <div class="nest-create-grid">
            <v-text-field
              ref="focusNameEl"
              v-model="form.name"
              :label="t('library.create.name')"
              :placeholder="t('library.create.namePlaceholder')"
              :error-messages="error && !form.name.trim() ? [error] : []"
              density="compact"
              variant="outlined"
              hide-details="auto"
              autofocus
              class="nest-create-field-wide"
            />
            <v-text-field
              v-model="form.tags"
              :label="t('library.create.tags')"
              :placeholder="t('library.create.tagsPlaceholder')"
              :hint="t('library.create.tagsHint')"
              density="compact"
              variant="outlined"
              hide-details="auto"
              persistent-hint
            />
            <v-text-field
              v-model="form.avatar_url"
              :label="t('library.create.avatarUrl')"
              :placeholder="t('library.create.avatarPlaceholder')"
              :hint="t('library.create.avatarHint')"
              density="compact"
              variant="outlined"
              hide-details="auto"
              persistent-hint
              class="nest-create-field-wide"
            />
          </div>
        </section>

        <!-- ─── Profile section ─── -->
        <section class="nest-create-section">
          <div class="nest-create-section-head">{{ t('library.create.section.profile') }}</div>
          <v-textarea
            v-model="form.description"
            :label="t('library.create.description')"
            :placeholder="t('library.create.descriptionPlaceholder')"
            rows="3"
            auto-grow
            density="compact"
            variant="outlined"
            hide-details="auto"
            class="mb-3"
          />
          <v-textarea
            v-model="form.personality"
            :label="t('library.create.personality')"
            :placeholder="t('library.create.personalityPlaceholder')"
            rows="2"
            auto-grow
            density="compact"
            variant="outlined"
            hide-details="auto"
          />
        </section>

        <!-- ─── Scene section ─── -->
        <section class="nest-create-section">
          <div class="nest-create-section-head">{{ t('library.create.section.scene') }}</div>
          <v-textarea
            v-model="form.scenario"
            :label="t('library.create.scenario')"
            :placeholder="t('library.create.scenarioPlaceholder')"
            rows="2"
            auto-grow
            density="compact"
            variant="outlined"
            hide-details="auto"
            class="mb-3"
          />
          <v-textarea
            v-model="form.first_mes"
            :label="t('library.create.firstMes')"
            :placeholder="t('library.create.firstMesPlaceholder')"
            rows="3"
            auto-grow
            density="compact"
            variant="outlined"
            hide-details="auto"
          />
        </section>

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

      <v-card-actions class="nest-create-actions">
        <v-spacer />
        <v-btn variant="text" :disabled="busy" @click="close">
          {{ t('common.cancel') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="flat"
          :loading="busy"
          :disabled="!form.name.trim()"
          @click="save"
        >
          {{ isEdit ? t('common.save') : t('library.create.createBtn') }}
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

  // On fullscreen (mobile) the border/radius just eat space; disable them.
  :global(.v-overlay--active) &.v-card--variant-elevated {
    border-radius: 0 !important;
  }
}

.nest-create-title {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  padding: 20px 20px 12px;
  border-bottom: 1px solid var(--nest-border-subtle);
}

.nest-create-body {
  padding: 16px 20px 8px;
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.nest-create-section {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.nest-create-section-head {
  font-family: var(--nest-font-mono);
  font-size: 10.5px;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--nest-text-muted);
  padding-bottom: 4px;
  border-bottom: 1px dashed var(--nest-border-subtle);
}

.nest-create-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 10px 12px;
}

.nest-create-field-wide {
  grid-column: 1 / -1;
}

.nest-create-actions {
  padding: 12px 20px 16px;
  border-top: 1px solid var(--nest-border-subtle);
}

@media (max-width: 600px) {
  .nest-create-grid {
    grid-template-columns: 1fr;
  }
  .nest-create-title { padding: 16px 16px 10px; }
  .nest-create-body  { padding: 14px 16px 6px; }
  .nest-create-actions { padding: 10px 16px 14px; }
}
</style>
