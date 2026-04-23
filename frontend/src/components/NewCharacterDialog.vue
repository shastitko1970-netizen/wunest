<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useDisplay } from 'vuetify'
import { useCharactersStore } from '@/stores/characters'
import type { Character, CharacterBook } from '@/api/characters'
import CharacterBookPanel from '@/components/CharacterBookPanel.vue'

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

// Everything in one reactive bag → single-line reset. Fields mirror the
// SillyTavern V2/V3 character-card spec so imports round-trip through the
// form unchanged. Less-common fields (mes_example, system_prompt,
// creator meta) live behind expansion panels so the first-time flow
// stays short; power users expand them as needed.
const form = reactive({
  // Identity
  name: '',
  avatar_url: '',
  tags: '',
  nickname: '',
  // Profile
  description: '',
  personality: '',
  scenario: '',
  // Greetings
  first_mes: '',
  alternate_greetings: [] as string[],
  mes_example: '',
  // Prompt injection (rarely edited but ST-critical)
  system_prompt: '',
  post_history_instructions: '',
  // Meta
  creator: '',
  character_version: '',
  creator_notes: '',
  // Embedded lorebook (V2/V3 character_book). null = no book; non-null =
  // book exists. Full V3 spec — CharacterBookPanel renders + mutates.
  character_book: null as CharacterBook | null,
})

const busy = ref(false)
const error = ref<string | null>(null)
const focusNameEl = ref<HTMLElement | null>(null)

function resetForm() {
  form.name = ''
  form.avatar_url = ''
  form.tags = ''
  form.nickname = ''
  form.description = ''
  form.personality = ''
  form.scenario = ''
  form.first_mes = ''
  form.alternate_greetings = []
  form.mes_example = ''
  form.system_prompt = ''
  form.post_history_instructions = ''
  form.creator = ''
  form.character_version = ''
  form.creator_notes = ''
  form.character_book = null
}

watch(() => [props.modelValue, props.character] as const, ([open, ch]) => {
  if (!open) return
  if (ch) {
    // Edit mode — hydrate from the character. Array / optional-string
    // fields default to empty so the form UI never sees undefined.
    form.name = ch.name
    form.avatar_url = ch.avatar_url ?? ''
    form.tags = (ch.tags ?? []).join(', ')
    form.nickname = (ch.data as any)?.nickname ?? ''
    form.description = ch.data?.description ?? ''
    form.personality = ch.data?.personality ?? ''
    form.scenario = ch.data?.scenario ?? ''
    form.first_mes = ch.data?.first_mes ?? ''
    form.alternate_greetings = Array.isArray((ch.data as any)?.alternate_greetings)
      ? [...(ch.data as any).alternate_greetings]
      : []
    form.mes_example = (ch.data as any)?.mes_example ?? ''
    form.system_prompt = (ch.data as any)?.system_prompt ?? ''
    form.post_history_instructions = (ch.data as any)?.post_history_instructions ?? ''
    form.creator = (ch.data as any)?.creator ?? ''
    form.character_version = (ch.data as any)?.character_version ?? ''
    form.creator_notes = (ch.data as any)?.creator_notes ?? ''
    // Deep-clone the embedded book so in-dialog edits don't mutate the
    // store's cached Character until the user saves.
    const book = (ch.data as any)?.character_book
    form.character_book = book
      ? (JSON.parse(JSON.stringify(book)) as CharacterBook)
      : null
  } else {
    resetForm()
  }
  error.value = null
  busy.value = false
})

/**
 * Build the typed `data` payload from the form state. Preserves any
 * fields we don't surface (character_book, assets, extensions, etc.)
 * via the `...base` spread so import → save doesn't lose data.
 *
 * Empty strings / empty arrays become null/unset so the DB blob stays
 * clean and re-exports don't carry stale blank fields.
 */
function buildData(base: Record<string, any> = {}): Record<string, any> {
  const tags = form.tags.split(',').map(t2 => t2.trim()).filter(Boolean)
  const alternateGreetings = form.alternate_greetings
    .map(g => g.trim())
    .filter(Boolean)
  const out: Record<string, any> = { ...base }
  out.name = form.name.trim()
  out.description = form.description.trim()
  out.personality = form.personality.trim()
  out.scenario = form.scenario.trim()
  out.first_mes = form.first_mes.trim()
  out.tags = tags

  // Optional fields — write only if non-empty so the JSONB stays
  // trimmed when the user hasn't touched them.
  const setOrDel = (key: string, value: string) => {
    const trimmed = value.trim()
    if (trimmed) out[key] = trimmed
    else delete out[key]
  }
  setOrDel('nickname', form.nickname)
  setOrDel('mes_example', form.mes_example)
  setOrDel('system_prompt', form.system_prompt)
  setOrDel('post_history_instructions', form.post_history_instructions)
  setOrDel('creator', form.creator)
  setOrDel('character_version', form.character_version)
  setOrDel('creator_notes', form.creator_notes)

  if (alternateGreetings.length > 0) out.alternate_greetings = alternateGreetings
  else delete out.alternate_greetings

  // character_book — write when present (even empty), delete when null so
  // cards without a book stay clean. Users who opt in to the book by
  // clicking "Create book" get an empty shell they can add entries to.
  if (form.character_book) {
    out.character_book = form.character_book
  } else {
    delete out.character_book
  }

  return out
}

function addGreeting() {
  form.alternate_greetings.push('')
}
function removeGreeting(idx: number) {
  form.alternate_greetings.splice(idx, 1)
}

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
    const tags = form.tags.split(',').map(t2 => t2.trim()).filter(Boolean)

    if (props.character) {
      // EDIT — preserve character_book / assets / extensions / anything
      // else we don't surface by spreading ch.data into the base. buildData
      // overwrites surfaced fields + deletes cleared optional ones.
      const data = buildData((props.character.data ?? {}) as Record<string, any>) as any
      const updated = await store.update(props.character.id, {
        name,
        avatar_url: form.avatar_url.trim() || undefined,
        data,
        tags,
      })
      emit('saved', updated.id)
    } else {
      // CREATE — start with an empty base. No preservation needed since
      // there's no prior row.
      const data = buildData({}) as any
      const created = await store.create({
        name,
        avatar_url: form.avatar_url.trim() || undefined,
        data,
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
            <v-text-field
              v-model="form.nickname"
              :label="t('library.create.nickname')"
              :placeholder="t('library.create.nicknamePlaceholder')"
              density="compact"
              variant="outlined"
              hide-details="auto"
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
            class="mb-3"
          />

          <!-- Alternate greetings — ST V2/V3 field. Each entry = a
               different opening message the user can pick at chat
               start. Presented as a dynamic array of textareas with
               add/remove buttons. -->
          <div class="nest-alt-greetings">
            <div class="nest-alt-greetings-head">
              <label class="nest-alt-greetings-label">
                {{ t('library.create.alternateGreetings') }}
              </label>
              <v-btn
                size="x-small"
                variant="text"
                prepend-icon="mdi-plus"
                @click="addGreeting"
              >
                {{ t('library.create.addGreeting') }}
              </v-btn>
            </div>
            <div
              v-for="(_, idx) in form.alternate_greetings"
              :key="idx"
              class="nest-alt-greeting-row"
            >
              <v-textarea
                v-model="form.alternate_greetings[idx]"
                :placeholder="t('library.create.alternateGreetingPlaceholder', { n: idx + 2 })"
                rows="2"
                auto-grow
                density="compact"
                variant="outlined"
                hide-details="auto"
                class="flex-grow-1"
              />
              <v-btn
                size="x-small"
                variant="text"
                color="error"
                icon="mdi-delete-outline"
                :title="t('common.delete')"
                @click="removeGreeting(idx)"
              />
            </div>
            <div
              v-if="form.alternate_greetings.length === 0"
              class="nest-alt-greetings-empty"
            >
              {{ t('library.create.alternateGreetingsEmpty') }}
            </div>
          </div>
        </section>

        <!-- ─── Advanced — mes_example + system_prompt + post_history ─── -->
        <v-expansion-panels variant="accordion" class="nest-create-adv">
          <v-expansion-panel>
            <v-expansion-panel-title>
              <v-icon size="16" class="mr-2">mdi-format-quote-close</v-icon>
              {{ t('library.create.section.advanced') }}
            </v-expansion-panel-title>
            <v-expansion-panel-text>
              <v-textarea
                v-model="form.mes_example"
                :label="t('library.create.mesExample')"
                :placeholder="t('library.create.mesExamplePlaceholder')"
                :hint="t('library.create.mesExampleHint')"
                rows="5"
                auto-grow
                density="compact"
                variant="outlined"
                hide-details="auto"
                persistent-hint
                class="mb-3 nest-create-mono"
              />
              <v-textarea
                v-model="form.system_prompt"
                :label="t('library.create.systemPrompt')"
                :placeholder="t('library.create.systemPromptPlaceholder')"
                :hint="t('library.create.systemPromptHint')"
                rows="3"
                auto-grow
                density="compact"
                variant="outlined"
                hide-details="auto"
                persistent-hint
                class="mb-3"
              />
              <v-textarea
                v-model="form.post_history_instructions"
                :label="t('library.create.postHistory')"
                :placeholder="t('library.create.postHistoryPlaceholder')"
                :hint="t('library.create.postHistoryHint')"
                rows="3"
                auto-grow
                density="compact"
                variant="outlined"
                hide-details="auto"
                persistent-hint
              />
            </v-expansion-panel-text>
          </v-expansion-panel>

          <!-- ─── Character Book — embedded lorebook (ST V2/V3) ─── -->
          <v-expansion-panel>
            <v-expansion-panel-title>
              <v-icon size="16" class="mr-2">mdi-book-outline</v-icon>
              {{ t('library.create.section.book') }}
              <v-chip
                v-if="form.character_book && (form.character_book.entries?.length ?? 0) > 0"
                size="x-small"
                variant="tonal"
                color="primary"
                class="nest-mono ml-2"
              >
                {{ form.character_book.entries?.length ?? 0 }}
              </v-chip>
            </v-expansion-panel-title>
            <v-expansion-panel-text>
              <CharacterBookPanel v-model="form.character_book" />
            </v-expansion-panel-text>
          </v-expansion-panel>

          <!-- ─── Meta — creator / version / notes ─── -->
          <v-expansion-panel>
            <v-expansion-panel-title>
              <v-icon size="16" class="mr-2">mdi-information-outline</v-icon>
              {{ t('library.create.section.meta') }}
            </v-expansion-panel-title>
            <v-expansion-panel-text>
              <div class="nest-create-grid">
                <v-text-field
                  v-model="form.creator"
                  :label="t('library.create.creator')"
                  :placeholder="t('library.create.creatorPlaceholder')"
                  density="compact"
                  variant="outlined"
                  hide-details="auto"
                />
                <v-text-field
                  v-model="form.character_version"
                  :label="t('library.create.characterVersion')"
                  :placeholder="t('library.create.characterVersionPlaceholder')"
                  density="compact"
                  variant="outlined"
                  hide-details="auto"
                />
              </div>
              <v-textarea
                v-model="form.creator_notes"
                :label="t('library.create.creatorNotes')"
                :placeholder="t('library.create.creatorNotesPlaceholder')"
                rows="3"
                auto-grow
                density="compact"
                variant="outlined"
                hide-details="auto"
                class="mt-3"
              />
            </v-expansion-panel-text>
          </v-expansion-panel>
        </v-expansion-panels>

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

// Alternate greetings dynamic list.
.nest-alt-greetings {
  margin-top: 4px;
}
.nest-alt-greetings-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 6px;
}
.nest-alt-greetings-label {
  font-size: 12px;
  color: var(--nest-text-secondary);
  text-transform: none;
}
.nest-alt-greeting-row {
  display: flex;
  gap: 6px;
  align-items: flex-start;
  margin-bottom: 8px;
}
.nest-alt-greetings-empty {
  font-size: 12px;
  color: var(--nest-text-muted);
  font-style: italic;
  padding: 4px 2px;
}

// Collapsible advanced/meta sections — less visual weight than the
// always-visible Profile/Scene sections above, so they read as optional.
.nest-create-adv {
  margin-top: 12px;

  :deep(.v-expansion-panel-title) {
    min-height: 44px !important;
    padding: 10px 14px !important;
    font-size: 13px;
  }
  :deep(.v-expansion-panel-text__wrapper) {
    padding: 12px 14px 16px !important;
  }
  :deep(.v-expansion-panel) {
    background: var(--nest-surface) !important;
    border: 1px solid var(--nest-border-subtle) !important;
    border-radius: var(--nest-radius-sm) !important;
    margin-top: 6px;
  }
}

// mes_example is dialogue-shaped — monospace makes it readable.
.nest-create-mono {
  :deep(textarea) {
    font-family: var(--nest-font-mono) !important;
    font-size: 12.5px !important;
    line-height: 1.55 !important;
  }
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
