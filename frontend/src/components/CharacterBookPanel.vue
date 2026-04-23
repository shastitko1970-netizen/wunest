<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { CharacterBook, CharacterBookEntry } from '@/api/characters'

/**
 * CharacterBookPanel — inline editor for the character's embedded
 * lorebook (SillyTavern's `character_book` field on V2/V3 cards).
 *
 * Mirrors ST's per-character lorebook UI: book-level settings at the
 * top (name, description, scan_depth, token_budget, recursive_scanning),
 * then a list of entries below. Each entry row shows the enable
 * toggle, keys chip, content preview, position chip, edit/delete, and
 * expands inline to a full editor covering every V3 spec field
 * (primary + secondary keys, content, insertion_order, priority,
 * position, selective/constant flags, case_sensitive, comment, name).
 *
 * v-model is the book itself (CharacterBook | null). null when the
 * character has no embedded book; the "Create book" button lazily
 * instantiates it.
 */

const { t } = useI18n()

const props = defineProps<{
  modelValue: CharacterBook | null
}>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: CharacterBook | null): void
}>()

const expandedIdx = ref<number | null>(null)
const confirmDeleteIdx = ref<number | null>(null)

// Stable "has book" computed — true when modelValue is a non-null object.
// Users can remove every entry but keep the book shell (preserves name /
// description / settings), so we distinguish "no book" from "empty book".
const hasBook = computed(() => props.modelValue !== null)

const entries = computed<CharacterBookEntry[]>(() =>
  props.modelValue?.entries ?? [],
)

const enabledCount = computed(() => entries.value.filter(e => e.enabled).length)

// ── Mutations ──────────────────────────────────────────────────────

function setBook(next: CharacterBook | null) {
  emit('update:modelValue', next)
}

function updateField<K extends keyof CharacterBook>(key: K, value: CharacterBook[K]) {
  if (!props.modelValue) return
  setBook({ ...props.modelValue, [key]: value })
}

function updateEntries(next: CharacterBookEntry[]) {
  if (!props.modelValue) return
  setBook({ ...props.modelValue, entries: next })
}

function updateEntry(idx: number, patch: Partial<CharacterBookEntry>) {
  const next = [...entries.value]
  next[idx] = { ...next[idx], ...patch }
  updateEntries(next)
}

function createBook() {
  setBook({
    name: '',
    description: '',
    entries: [],
  })
}

function removeBook() {
  setBook(null)
  expandedIdx.value = null
  confirmDeleteIdx.value = null
}

function addEntry() {
  if (!props.modelValue) createBook()
  const nextID = Math.max(0, ...entries.value.map(e => e.id ?? 0)) + 1
  const newEntry: CharacterBookEntry = {
    id: nextID,
    keys: [],
    content: '',
    enabled: true,
    insertion_order: entries.value.length * 100,
    position: 'before_char',
  }
  const next = [...entries.value, newEntry]
  // setBook with next including the new entries
  setBook({ ...(props.modelValue ?? { entries: [] }), entries: next })
  expandedIdx.value = next.length - 1
}

function removeEntry(idx: number) {
  const next = entries.value.filter((_, i) => i !== idx)
  updateEntries(next)
  if (expandedIdx.value === idx) expandedIdx.value = null
  confirmDeleteIdx.value = null
}

function toggleEntry(idx: number) {
  updateEntry(idx, { enabled: !entries.value[idx].enabled })
}

function moveUp(idx: number) {
  if (idx <= 0) return
  const next = [...entries.value]
  ;[next[idx - 1], next[idx]] = [next[idx], next[idx - 1]]
  updateEntries(next)
}
function moveDown(idx: number) {
  if (idx >= entries.value.length - 1) return
  const next = [...entries.value]
  ;[next[idx], next[idx + 1]] = [next[idx + 1], next[idx]]
  updateEntries(next)
}

// Keys and secondary-keys are stored as string[] on the spec, but the
// v-combobox used below delivers strings with trailing whitespace when
// the user presses Enter. Normalize on write.
function setKeys(idx: number, value: unknown) {
  const arr = Array.isArray(value)
    ? value.map(v => String(v).trim()).filter(Boolean)
    : []
  updateEntry(idx, { keys: arr })
}
function setSecondaryKeys(idx: number, value: unknown) {
  const arr = Array.isArray(value)
    ? value.map(v => String(v).trim()).filter(Boolean)
    : []
  updateEntry(idx, { secondary_keys: arr })
}

// Content preview for the row — first ~80 chars, single-line, ellipsis.
function contentPreview(entry: CharacterBookEntry): string {
  const content = (entry.content ?? '').trim().replace(/\s+/g, ' ')
  if (!content) return t('characterBook.entry.noContent')
  return content.length > 80 ? content.slice(0, 77) + '…' : content
}

function positionChip(entry: CharacterBookEntry): string {
  switch (entry.position) {
    case 'before_char': return t('characterBook.entry.posBefore')
    case 'after_char':  return t('characterBook.entry.posAfter')
    default:            return entry.position ?? 'before_char'
  }
}
</script>

<template>
  <div class="nest-charbook">
    <!-- No-book state: user can opt in to creating the embedded book. -->
    <div v-if="!hasBook" class="nest-charbook-empty">
      <v-icon size="36" color="surface-variant">mdi-book-outline</v-icon>
      <div class="nest-charbook-empty-title">{{ t('characterBook.emptyTitle') }}</div>
      <p class="nest-charbook-empty-body">{{ t('characterBook.emptyBody') }}</p>
      <v-btn
        color="primary"
        variant="flat"
        size="small"
        prepend-icon="mdi-plus"
        @click="createBook"
      >
        {{ t('characterBook.createBtn') }}
      </v-btn>
    </div>

    <!-- Book present — surface settings + entries list. -->
    <template v-else>
      <!-- Book-level settings. -->
      <div class="nest-charbook-header">
        <div class="nest-field-row">
          <div class="nest-field nest-field-half">
            <label class="nest-field-label">{{ t('characterBook.bookName') }}</label>
            <v-text-field
              :model-value="modelValue?.name"
              :placeholder="t('characterBook.bookNamePlaceholder')"
              density="compact" hide-details
              @update:model-value="v => updateField('name', v)"
            />
          </div>
          <div class="nest-field nest-field-half">
            <label class="nest-field-label">
              {{ t('characterBook.scanDepth') }}
              <v-tooltip location="top" :text="t('characterBook.scanDepthHint')">
                <template #activator="{ props: p }">
                  <v-icon v-bind="p" size="12" class="nest-hint-icon">mdi-information-outline</v-icon>
                </template>
              </v-tooltip>
            </label>
            <v-text-field
              :model-value="modelValue?.scan_depth ?? null"
              type="number" :min="1" :max="100"
              :placeholder="t('characterBook.scanDepthPlaceholder')"
              density="compact" hide-details clearable
              @update:model-value="v => updateField('scan_depth', v === '' ? null : Number(v))"
            />
          </div>
        </div>
        <div class="nest-field-row">
          <div class="nest-field nest-field-half">
            <label class="nest-field-label">
              {{ t('characterBook.tokenBudget') }}
              <v-tooltip location="top" :text="t('characterBook.tokenBudgetHint')">
                <template #activator="{ props: p }">
                  <v-icon v-bind="p" size="12" class="nest-hint-icon">mdi-information-outline</v-icon>
                </template>
              </v-tooltip>
            </label>
            <v-text-field
              :model-value="modelValue?.token_budget ?? null"
              type="number" :min="0"
              :placeholder="t('characterBook.tokenBudgetPlaceholder')"
              density="compact" hide-details clearable
              @update:model-value="v => updateField('token_budget', v === '' ? null : Number(v))"
            />
          </div>
          <div class="nest-field nest-field-half">
            <v-switch
              :model-value="modelValue?.recursive_scanning ?? false"
              color="primary"
              density="compact"
              hide-details
              :label="t('characterBook.recursive')"
              @update:model-value="v => updateField('recursive_scanning', v)"
            />
          </div>
        </div>
        <div class="nest-field">
          <label class="nest-field-label">{{ t('characterBook.bookDesc') }}</label>
          <v-textarea
            :model-value="modelValue?.description"
            :placeholder="t('characterBook.bookDescPlaceholder')"
            rows="2" auto-grow density="compact" hide-details
            @update:model-value="v => updateField('description', v)"
          />
        </div>

        <!-- Entries stats + actions -->
        <div class="nest-charbook-entries-head">
          <div class="nest-mono nest-charbook-stats">
            {{ t('characterBook.stats', { enabled: enabledCount, total: entries.length }) }}
          </div>
          <v-btn
            size="small"
            variant="outlined"
            prepend-icon="mdi-plus"
            @click="addEntry"
          >
            {{ t('characterBook.addEntry') }}
          </v-btn>
          <v-btn
            size="small"
            variant="text"
            color="error"
            prepend-icon="mdi-delete-outline"
            :title="t('characterBook.removeBookTitle')"
            @click="removeBook"
          >
            {{ t('characterBook.removeBook') }}
          </v-btn>
        </div>
      </div>

      <!-- Entries list. -->
      <div v-if="entries.length === 0" class="nest-charbook-entries-empty">
        {{ t('characterBook.entriesEmpty') }}
      </div>
      <div v-else class="nest-charbook-entries">
        <div
          v-for="(entry, idx) in entries"
          :key="(entry.id ?? -1) + '_' + idx"
          class="nest-charbook-entry"
          :class="{ disabled: !entry.enabled, expanded: expandedIdx === idx }"
        >
          <!-- Entry row-main — clickable anywhere except action cluster. -->
          <div
            class="nest-charbook-entry-main"
            @click="expandedIdx = expandedIdx === idx ? null : idx"
          >
            <v-checkbox-btn
              :model-value="entry.enabled"
              color="success"
              density="compact"
              hide-details
              @click.stop
              @update:model-value="toggleEntry(idx)"
            />
            <!-- Entry name OR first key OR "(no keys)" -->
            <span class="nest-charbook-entry-name">
              {{ entry.name || entry.keys[0] || t('characterBook.entry.noKeys') }}
            </span>
            <v-chip
              v-if="entry.keys.length > 1"
              size="x-small"
              variant="tonal"
              class="nest-mono"
            >
              +{{ entry.keys.length - 1 }}
            </v-chip>
            <v-chip
              size="x-small"
              variant="tonal"
              :color="entry.position === 'after_char' ? 'warning' : 'primary'"
              class="nest-mono"
            >
              {{ positionChip(entry) }}
            </v-chip>
            <span class="nest-charbook-preview">{{ contentPreview(entry) }}</span>
            <!-- Action cluster — @click.stop prevents row expansion. -->
            <div class="nest-charbook-entry-actions" @click.stop>
              <v-btn
                size="x-small" variant="text" icon="mdi-chevron-up"
                :disabled="idx === 0" :title="t('common.moveUp')"
                @click="moveUp(idx)"
              />
              <v-btn
                size="x-small" variant="text" icon="mdi-chevron-down"
                :disabled="idx === entries.length - 1" :title="t('common.moveDown')"
                @click="moveDown(idx)"
              />
              <v-btn
                size="x-small" variant="text" color="error"
                icon="mdi-delete-outline"
                :title="t('common.delete')"
                @click="confirmDeleteIdx = idx"
              />
            </div>
          </div>

          <!-- Inline editor for expanded entry — all V3 fields. -->
          <div v-if="expandedIdx === idx" class="nest-charbook-entry-edit">
            <div class="nest-field-row">
              <div class="nest-field nest-field-half">
                <label class="nest-field-label">{{ t('characterBook.entry.name') }}</label>
                <v-text-field
                  :model-value="entry.name"
                  density="compact" hide-details
                  @update:model-value="v => updateEntry(idx, { name: v })"
                />
              </div>
              <div class="nest-field nest-field-half">
                <label class="nest-field-label">{{ t('characterBook.entry.comment') }}</label>
                <v-text-field
                  :model-value="entry.comment"
                  density="compact" hide-details
                  @update:model-value="v => updateEntry(idx, { comment: v })"
                />
              </div>
            </div>

            <div class="nest-field">
              <label class="nest-field-label">
                {{ t('characterBook.entry.keys') }}
                <v-tooltip location="top" :text="t('characterBook.entry.keysHint')">
                  <template #activator="{ props: p }">
                    <v-icon v-bind="p" size="12" class="nest-hint-icon">mdi-information-outline</v-icon>
                  </template>
                </v-tooltip>
              </label>
              <v-combobox
                :model-value="entry.keys"
                :items="[]"
                multiple
                chips
                closable-chips
                density="compact"
                variant="outlined"
                hide-details
                :placeholder="t('characterBook.entry.keysPlaceholder')"
                @update:model-value="v => setKeys(idx, v)"
              />
            </div>

            <div v-if="entry.selective || (entry.secondary_keys?.length ?? 0) > 0" class="nest-field">
              <label class="nest-field-label">
                {{ t('characterBook.entry.secondaryKeys') }}
                <v-tooltip location="top" :text="t('characterBook.entry.secondaryKeysHint')">
                  <template #activator="{ props: p }">
                    <v-icon v-bind="p" size="12" class="nest-hint-icon">mdi-information-outline</v-icon>
                  </template>
                </v-tooltip>
              </label>
              <v-combobox
                :model-value="entry.secondary_keys ?? []"
                :items="[]"
                multiple
                chips
                closable-chips
                density="compact"
                variant="outlined"
                hide-details
                :placeholder="t('characterBook.entry.keysPlaceholder')"
                @update:model-value="v => setSecondaryKeys(idx, v)"
              />
            </div>

            <div class="nest-field">
              <label class="nest-field-label">{{ t('characterBook.entry.content') }}</label>
              <v-textarea
                :model-value="entry.content"
                :placeholder="t('characterBook.entry.contentPlaceholder')"
                rows="5" auto-grow density="compact" hide-details
                class="nest-charbook-content"
                @update:model-value="v => updateEntry(idx, { content: v })"
              />
            </div>

            <div class="nest-field-row">
              <div class="nest-field nest-field-half">
                <label class="nest-field-label">
                  {{ t('characterBook.entry.position') }}
                  <v-tooltip location="top" :text="t('characterBook.entry.positionHint')">
                    <template #activator="{ props: p }">
                      <v-icon v-bind="p" size="12" class="nest-hint-icon">mdi-information-outline</v-icon>
                    </template>
                  </v-tooltip>
                </label>
                <v-select
                  :model-value="entry.position || 'before_char'"
                  :items="[
                    { value: 'before_char', title: t('characterBook.entry.posBefore') },
                    { value: 'after_char',  title: t('characterBook.entry.posAfter') },
                  ]"
                  density="compact" hide-details
                  @update:model-value="v => updateEntry(idx, { position: v })"
                />
              </div>
              <div class="nest-field nest-field-half">
                <label class="nest-field-label">
                  {{ t('characterBook.entry.insertionOrder') }}
                  <v-tooltip location="top" :text="t('characterBook.entry.insertionOrderHint')">
                    <template #activator="{ props: p }">
                      <v-icon v-bind="p" size="12" class="nest-hint-icon">mdi-information-outline</v-icon>
                    </template>
                  </v-tooltip>
                </label>
                <v-text-field
                  :model-value="entry.insertion_order"
                  type="number"
                  density="compact" hide-details
                  @update:model-value="v => updateEntry(idx, { insertion_order: Number(v) || 0 })"
                />
              </div>
            </div>

            <div class="nest-field-row">
              <div class="nest-field nest-field-half">
                <label class="nest-field-label">
                  {{ t('characterBook.entry.priority') }}
                  <v-tooltip location="top" :text="t('characterBook.entry.priorityHint')">
                    <template #activator="{ props: p }">
                      <v-icon v-bind="p" size="12" class="nest-hint-icon">mdi-information-outline</v-icon>
                    </template>
                  </v-tooltip>
                </label>
                <v-text-field
                  :model-value="entry.priority ?? 0"
                  type="number"
                  density="compact" hide-details
                  @update:model-value="v => updateEntry(idx, { priority: Number(v) || 0 })"
                />
              </div>
              <div class="nest-field nest-field-half">
                <label class="nest-field-label">
                  {{ t('characterBook.entry.caseSensitive') }}
                </label>
                <v-select
                  :model-value="entry.case_sensitive ?? null"
                  :items="[
                    { value: null,  title: t('characterBook.entry.caseDefault') },
                    { value: true,  title: t('characterBook.entry.caseOn') },
                    { value: false, title: t('characterBook.entry.caseOff') },
                  ]"
                  density="compact" hide-details
                  @update:model-value="v => updateEntry(idx, { case_sensitive: v })"
                />
              </div>
            </div>

            <div class="nest-field-row nest-charbook-flags">
              <v-switch
                :model-value="entry.constant ?? false"
                color="primary" hide-details density="compact"
                :label="t('characterBook.entry.constant')"
                :hint="t('characterBook.entry.constantHint')"
                persistent-hint
                @update:model-value="v => updateEntry(idx, { constant: !!v })"
              />
              <v-switch
                :model-value="entry.selective ?? false"
                color="primary" hide-details density="compact"
                :label="t('characterBook.entry.selective')"
                :hint="t('characterBook.entry.selectiveHint')"
                persistent-hint
                @update:model-value="v => updateEntry(idx, { selective: !!v })"
              />
            </div>
          </div>
        </div>
      </div>
    </template>

    <!-- Confirm-delete dialog for entries. -->
    <v-dialog
      :model-value="confirmDeleteIdx !== null"
      max-width="360"
      @update:model-value="v => !v && (confirmDeleteIdx = null)"
    >
      <v-card class="nest-confirm">
        <v-card-title>{{ t('characterBook.entry.deleteTitle') }}</v-card-title>
        <v-card-text>{{ t('characterBook.entry.deleteBody') }}</v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="confirmDeleteIdx = null">{{ t('common.cancel') }}</v-btn>
          <v-btn
            color="error"
            variant="flat"
            @click="confirmDeleteIdx !== null && removeEntry(confirmDeleteIdx)"
          >
            {{ t('common.delete') }}
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<style lang="scss" scoped>
.nest-charbook {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.nest-charbook-empty {
  text-align: center;
  padding: 32px 20px;
  border: 1px dashed var(--nest-border-subtle);
  border-radius: var(--nest-radius);
  background: var(--nest-surface);

  .nest-charbook-empty-title {
    font-size: 14px;
    color: var(--nest-text);
    margin-top: 8px;
    font-weight: 500;
  }
  .nest-charbook-empty-body {
    font-size: 12.5px;
    color: var(--nest-text-muted);
    margin: 6px auto 14px;
    max-width: 360px;
  }
}

.nest-charbook-header {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px 14px;
  background: var(--nest-surface);
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
}

.nest-charbook-entries-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-top: 8px;
  margin-top: 4px;
  border-top: 1px solid var(--nest-border-subtle);
  flex-wrap: wrap;
}
.nest-charbook-stats {
  font-size: 11.5px;
  color: var(--nest-text-muted);
  flex: 1 1 auto;
}

.nest-charbook-entries {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.nest-charbook-entries-empty {
  padding: 18px;
  text-align: center;
  color: var(--nest-text-muted);
  font-size: 12.5px;
  border: 1px dashed var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
}

.nest-charbook-entry {
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
  background: var(--nest-surface);
  transition: border-color var(--nest-transition-fast);

  &:hover:not(.expanded) { border-color: var(--nest-border); }
  &.disabled { opacity: 0.55; }
  &.expanded { border-color: var(--nest-accent); }
}
.nest-charbook-entry-main {
  display: grid;
  grid-template-columns: auto auto auto auto 1fr auto;
  gap: 8px;
  align-items: center;
  padding: 6px 10px;
  cursor: pointer;
  min-height: 36px;
}
.nest-charbook-entry-name {
  font-size: 13px;
  color: var(--nest-text);
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 240px;
}
.nest-charbook-preview {
  font-size: 11.5px;
  color: var(--nest-text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
}
.nest-charbook-entry-actions {
  display: flex;
  gap: 0;
}

.nest-charbook-entry-edit {
  padding: 10px 14px 14px;
  border-top: 1px solid var(--nest-border-subtle);
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.nest-charbook-flags {
  :deep(.v-switch) {
    flex: 1 1 220px;
    min-width: 0;
  }
}

.nest-charbook-content {
  :deep(textarea) {
    font-family: var(--nest-font-mono) !important;
    font-size: 12.5px !important;
    line-height: 1.55 !important;
  }
}

.nest-field-row {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}
.nest-field { min-width: 0; }
.nest-field-half { flex: 1 1 220px; }
.nest-field-label {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--nest-text-secondary);
  margin-bottom: 4px;
}
.nest-hint-icon {
  color: var(--nest-text-muted);
  cursor: help;
  opacity: 0.7;
  &:hover { opacity: 1; color: var(--nest-accent); }
}

.nest-confirm {
  background: var(--nest-surface) !important;
  border: 1px solid var(--nest-border);
}

@media (max-width: 560px) {
  .nest-charbook-entry-main {
    grid-template-columns: auto 1fr auto;
    grid-template-areas:
      "check name actions"
      "check keys actions"
      "check preview actions";
    row-gap: 2px;
  }
  .nest-charbook-preview { grid-area: preview; white-space: normal; }
}
</style>
