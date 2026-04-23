<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useWorldsStore } from '@/stores/worlds'
import type { World, WorldEntry } from '@/api/worlds'
import ImportLorebookDialog from '@/components/ImportLorebookDialog.vue'

const { t } = useI18n()
const store = useWorldsStore()
const { items, loading, error } = storeToRefs(store)

const selectedId = ref<string | null>(null)
const selected = ref<World | null>(null)
const importOpen = ref(false)
const confirmDeleteId = ref<string | null>(null)
const saving = ref(false)
const saveError = ref<string | null>(null)

// Draft state while editing — only committed to the server on Save.
const draftName = ref('')
const draftDesc = ref('')
const draftEntries = ref<WorldEntry[]>([])
const expandedEntry = ref<number | null>(null)

onMounted(() => store.fetchAll())

watch(items, (list) => {
  // Auto-select first book if none selected (but only once).
  if (!selectedId.value && list.length) {
    void openBook(list[0].id)
  }
})

async function openBook(id: string) {
  selectedId.value = id
  expandedEntry.value = null
  saveError.value = null
  try {
    const w = await store.loadFull(id)
    selected.value = w
    draftName.value = w.name
    draftDesc.value = w.description
    draftEntries.value = w.entries.map(e => ({ ...e, keys: [...(e.keys ?? [])], secondary_keys: [...(e.secondary_keys ?? [])] }))
  } catch (e) {
    saveError.value = (e as Error).message
  }
}

async function createNew() {
  saving.value = true
  saveError.value = null
  try {
    const w = await store.create(t('worlds.newName'), '', [])
    await openBook(w.id)
  } catch (e) {
    saveError.value = (e as Error).message
  } finally {
    saving.value = false
  }
}

async function saveDraft() {
  if (!selected.value) return
  saving.value = true
  saveError.value = null
  try {
    const w = await store.update(selected.value.id, {
      name: draftName.value.trim() || t('worlds.newName'),
      description: draftDesc.value,
      entries: draftEntries.value,
    })
    selected.value = w
  } catch (e) {
    saveError.value = (e as Error).message
  } finally {
    saving.value = false
  }
}

function addEntry() {
  draftEntries.value.push({
    keys: [],
    content: '',
    enabled: true,
    position: 'before_char',
    insertion_order: 100,
  })
  expandedEntry.value = draftEntries.value.length - 1
}

function removeEntry(i: number) {
  draftEntries.value.splice(i, 1)
  if (expandedEntry.value === i) expandedEntry.value = null
}

function toggleExpand(i: number) {
  expandedEntry.value = expandedEntry.value === i ? null : i
}

function keysToString(entry: WorldEntry): string {
  return entry.keys.join(', ')
}

function stringToKeys(s: string): string[] {
  return s.split(',').map(k => k.trim()).filter(Boolean)
}

async function doDelete() {
  if (!confirmDeleteId.value) return
  const id = confirmDeleteId.value
  confirmDeleteId.value = null
  try {
    await store.remove(id)
    if (selectedId.value === id) {
      selected.value = null
      selectedId.value = null
    }
  } catch (e) {
    saveError.value = (e as Error).message
  }
}

function entryLabel(e: WorldEntry, i: number): string {
  return e.name || e.comment || e.keys[0] || `${t('worlds.entry')} ${i + 1}`
}

// Quick summary of activation conditions for collapsed rows.
const entryConditionText = (e: WorldEntry) => {
  if (e.constant) return t('worlds.cond.constant')
  if (e.keys.length === 0) return t('worlds.cond.noKeys')
  return e.keys.slice(0, 3).join(' · ') + (e.keys.length > 3 ? '…' : '')
}

const hasDraftChanges = computed(() => {
  if (!selected.value) return false
  if (draftName.value !== selected.value.name) return true
  if (draftDesc.value !== selected.value.description) return true
  if (JSON.stringify(draftEntries.value) !== JSON.stringify(selected.value.entries)) return true
  return false
})
</script>

<template>
  <div class="nest-worlds-panel">
    <!-- Left column: list of books -->
    <aside class="nest-worlds-list">
      <div class="nest-worlds-list-head">
        <h3 class="nest-h3">{{ t('worlds.books') }}</h3>
        <div class="d-flex ga-1">
          <v-btn
            size="small"
            variant="outlined"
            prepend-icon="mdi-upload"
            @click="importOpen = true"
          >
            {{ t('worlds.actions.import') }}
          </v-btn>
          <v-btn
            size="small"
            variant="flat"
            color="primary"
            prepend-icon="mdi-plus"
            :loading="saving"
            @click="createNew"
          >
            {{ t('worlds.actions.new') }}
          </v-btn>
        </div>
      </div>

      <div v-if="loading && !items.length" class="nest-state">
        <v-progress-circular indeterminate color="primary" size="24" />
      </div>
      <v-alert v-else-if="error" type="error" variant="tonal" density="compact">
        {{ error }}
      </v-alert>
      <div v-else-if="items.length === 0" class="nest-worlds-empty">
        <div class="nest-h3 mt-2">{{ t('worlds.empty.title') }}</div>
        <p class="nest-subtitle mt-2">{{ t('worlds.empty.hint') }}</p>
      </div>
      <div v-else class="nest-worlds-list-items">
        <button
          v-for="b in items"
          :key="b.id"
          class="nest-world-card"
          :class="{ active: selectedId === b.id }"
          @click="openBook(b.id)"
        >
          <div class="nest-world-name">{{ b.name }}</div>
          <div class="nest-world-meta">
            <span class="nest-mono">{{ b.entry_count }}</span> {{ t('worlds.entriesShort') }}
          </div>
          <div v-if="b.description" class="nest-world-desc">{{ b.description }}</div>
        </button>
      </div>
    </aside>

    <!-- Right column: editor -->
    <section class="nest-worlds-editor">
      <div v-if="!selected" class="nest-state">
        <v-icon size="40" color="surface-variant">mdi-book-open-page-variant-outline</v-icon>
        <p class="nest-subtitle mt-3">{{ t('worlds.pickOrCreate') }}</p>
      </div>
      <template v-else>
        <div class="nest-editor-head">
          <div class="nest-editor-name">
            <v-text-field
              v-model="draftName"
              :label="t('worlds.nameLabel')"
              density="compact"
              hide-details
              variant="plain"
              class="nest-world-name-input"
            />
            <v-text-field
              v-model="draftDesc"
              :placeholder="t('worlds.descPlaceholder')"
              density="compact"
              hide-details
              variant="plain"
              class="nest-world-desc-input"
            />
          </div>
          <div class="nest-editor-actions">
            <v-btn
              size="small"
              :disabled="!hasDraftChanges"
              :loading="saving"
              color="primary"
              variant="flat"
              prepend-icon="mdi-content-save"
              @click="saveDraft"
            >
              {{ t('common.save') }}
            </v-btn>
            <v-btn
              size="small"
              variant="text"
              color="error"
              prepend-icon="mdi-delete-outline"
              @click="confirmDeleteId = selected.id"
            >
              {{ t('common.delete') }}
            </v-btn>
          </div>
        </div>

        <v-alert v-if="saveError" type="error" variant="tonal" density="compact" class="mb-3">
          {{ saveError }}
        </v-alert>

        <div class="nest-entries-head">
          <h4 class="nest-h4">{{ t('worlds.entries') }}</h4>
          <v-btn
            size="small"
            variant="outlined"
            prepend-icon="mdi-plus"
            @click="addEntry"
          >
            {{ t('worlds.actions.addEntry') }}
          </v-btn>
        </div>

        <div v-if="draftEntries.length === 0" class="nest-entries-empty">
          {{ t('worlds.noEntries') }}
        </div>

        <div class="nest-entries-list">
          <div
            v-for="(entry, i) in draftEntries"
            :key="i"
            class="nest-entry-row"
            :class="{
              expanded: expandedEntry === i,
              disabled: !entry.enabled,
            }"
          >
            <div class="nest-entry-head" @click="toggleExpand(i)">
              <div class="nest-entry-title">
                <v-icon size="16" :color="entry.enabled ? 'primary' : 'surface-variant'">
                  {{ entry.constant ? 'mdi-flash' : 'mdi-key-outline' }}
                </v-icon>
                <span class="nest-entry-label">{{ entryLabel(entry, i) }}</span>
                <span class="nest-entry-cond">{{ entryConditionText(entry) }}</span>
              </div>
              <div class="nest-entry-head-actions" @click.stop>
                <v-switch
                  v-model="entry.enabled"
                  hide-details
                  density="compact"
                  inset
                  color="primary"
                />
                <v-btn
                  size="x-small"
                  variant="text"
                  color="error"
                  icon="mdi-delete-outline"
                  :title="t('common.delete')"
                  @click="removeEntry(i)"
                />
              </div>
            </div>

            <div v-if="expandedEntry === i" class="nest-entry-body">
              <v-text-field
                v-model="entry.name"
                :label="t('worlds.entryNameLabel')"
                density="compact"
                hide-details
                class="mb-3"
              />

              <div class="d-flex flex-wrap ga-3 mb-3">
                <v-text-field
                  :model-value="keysToString(entry)"
                  :label="t('worlds.keysLabel')"
                  :placeholder="t('worlds.keysPlaceholder')"
                  density="compact"
                  hide-details
                  class="nest-entry-keys"
                  @update:model-value="v => (entry.keys = stringToKeys(v))"
                />
                <v-select
                  v-model="entry.position"
                  :items="[
                    { value: 'before_char', title: t('worlds.position.beforeChar') },
                    { value: 'after_char', title: t('worlds.position.afterChar') },
                    { value: 'at_depth', title: t('worlds.position.atDepth') },
                    { value: 'before_an', title: t('worlds.position.beforeAN') },
                    { value: 'after_an', title: t('worlds.position.afterAN') },
                  ]"
                  :label="t('worlds.positionLabel')"
                  density="compact"
                  hide-details
                  class="nest-entry-position"
                />
              </div>

              <v-textarea
                v-model="entry.content"
                :label="t('worlds.contentLabel')"
                :placeholder="t('worlds.contentPlaceholder')"
                rows="4"
                auto-grow
                density="compact"
                class="mb-3"
                hide-details
              />

              <!-- Row 1: activation basics (constant/selective, order, depth) -->
              <div class="nest-entry-flags">
                <v-checkbox
                  v-model="entry.constant"
                  :label="t('worlds.flag.constant')"
                  density="compact"
                  hide-details
                  color="primary"
                />
                <v-checkbox
                  v-model="entry.selective"
                  :label="t('worlds.flag.selective')"
                  density="compact"
                  hide-details
                  color="primary"
                  :disabled="(entry.secondary_keys?.length ?? 0) === 0"
                />
                <v-text-field
                  v-model.number="entry.insertion_order"
                  :label="t('worlds.orderLabel')"
                  type="number"
                  density="compact"
                  hide-details
                  class="nest-entry-num"
                />
                <v-text-field
                  v-model.number="entry.depth"
                  :label="t('worlds.depthLabel')"
                  type="number"
                  density="compact"
                  hide-details
                  class="nest-entry-num"
                />
              </div>

              <!-- Row 2: matching controls (whole words / case / probability) -->
              <div class="nest-entry-flags mt-2">
                <v-checkbox
                  v-model="entry.match_whole_words"
                  :label="t('worlds.flag.matchWholeWords')"
                  :title="t('worlds.flag.matchWholeWordsHint')"
                  density="compact"
                  hide-details
                  color="primary"
                />
                <v-checkbox
                  v-model="entry.case_sensitive"
                  :label="t('worlds.flag.caseSensitive')"
                  density="compact"
                  hide-details
                  color="primary"
                />
                <v-text-field
                  v-model.number="entry.probability"
                  :label="t('worlds.probabilityLabel')"
                  :hint="t('worlds.probabilityHint')"
                  type="number"
                  min="0"
                  max="100"
                  density="compact"
                  hide-details
                  class="nest-entry-num"
                />
              </div>

              <!-- Row 3: group + override -->
              <div class="d-flex flex-wrap ga-3 mt-3">
                <v-text-field
                  v-model="entry.group"
                  :label="t('worlds.groupLabel')"
                  :placeholder="t('worlds.groupPlaceholder')"
                  :hint="t('worlds.groupHint')"
                  density="compact"
                  hide-details="auto"
                  persistent-hint
                  class="nest-entry-group"
                />
                <v-checkbox
                  v-model="entry.group_override"
                  :label="t('worlds.flag.groupOverride')"
                  :disabled="!entry.group"
                  density="compact"
                  hide-details
                  color="primary"
                />
              </div>

              <!-- Row 4: recursion controls -->
              <div class="nest-entry-flags mt-2">
                <v-checkbox
                  v-model="entry.exclude_recursion"
                  :label="t('worlds.flag.excludeRecursion')"
                  :title="t('worlds.flag.excludeRecursionHint')"
                  density="compact"
                  hide-details
                  color="primary"
                />
                <v-checkbox
                  v-model="entry.prevent_recursion"
                  :label="t('worlds.flag.preventRecursion')"
                  :title="t('worlds.flag.preventRecursionHint')"
                  density="compact"
                  hide-details
                  color="primary"
                />
              </div>

              <!-- Row 5: at-depth role (only meaningful for at_depth position) -->
              <div v-if="entry.position === 'at_depth'" class="d-flex flex-wrap ga-3 mt-3">
                <v-select
                  v-model="entry.role"
                  :items="[
                    { value: 'system', title: t('chat.authorsNote.roleSystem') },
                    { value: 'user', title: t('chat.authorsNote.roleUser') },
                    { value: 'assistant', title: t('chat.authorsNote.roleAssistant') },
                  ]"
                  :label="t('worlds.roleLabel')"
                  density="compact"
                  hide-details
                  class="nest-entry-position"
                />
              </div>

              <!-- Row 6: stateful activation timers (stored, not yet enforced) -->
              <details class="nest-entry-advanced mt-3">
                <summary class="nest-entry-advanced-head">
                  {{ t('worlds.advanced') }}
                </summary>
                <div class="d-flex flex-wrap ga-3 mt-2">
                  <v-text-field
                    v-model.number="entry.sticky"
                    :label="t('worlds.stickyLabel')"
                    :hint="t('worlds.stickyHint')"
                    type="number"
                    min="0"
                    density="compact"
                    hide-details="auto"
                    persistent-hint
                    class="nest-entry-num"
                  />
                  <v-text-field
                    v-model.number="entry.cooldown"
                    :label="t('worlds.cooldownLabel')"
                    :hint="t('worlds.cooldownHint')"
                    type="number"
                    min="0"
                    density="compact"
                    hide-details="auto"
                    persistent-hint
                    class="nest-entry-num"
                  />
                  <v-text-field
                    v-model.number="entry.delay"
                    :label="t('worlds.delayLabel')"
                    :hint="t('worlds.delayHint')"
                    type="number"
                    min="0"
                    density="compact"
                    hide-details="auto"
                    persistent-hint
                    class="nest-entry-num"
                  />
                </div>
              </details>

              <v-text-field
                v-if="entry.selective || (entry.secondary_keys?.length ?? 0) > 0"
                :model-value="(entry.secondary_keys ?? []).join(', ')"
                :label="t('worlds.secondaryKeysLabel')"
                :placeholder="t('worlds.keysPlaceholder')"
                density="compact"
                hide-details
                class="mt-3"
                @update:model-value="v => (entry.secondary_keys = stringToKeys(v))"
              />
            </div>
          </div>
        </div>
      </template>
    </section>

    <!-- Import dialog -->
    <ImportLorebookDialog
      v-model="importOpen"
      @imported="id => openBook(id)"
    />

    <!-- Delete confirmation -->
    <v-dialog
      :model-value="confirmDeleteId !== null"
      max-width="400"
      @update:model-value="v => !v && (confirmDeleteId = null)"
    >
      <v-card class="nest-confirm">
        <v-card-title>{{ t('worlds.delete.title') }}</v-card-title>
        <v-card-text>{{ t('worlds.delete.body') }}</v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="confirmDeleteId = null">{{ t('common.cancel') }}</v-btn>
          <v-btn color="error" variant="flat" @click="doDelete">{{ t('common.delete') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<style lang="scss" scoped>
.nest-worlds-panel {
  display: grid;
  grid-template-columns: 280px 1fr;
  gap: 16px;
  min-height: 500px;
}

.nest-worlds-list {
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius);
  padding: 12px;
  background: var(--nest-surface);
  display: flex;
  flex-direction: column;
  gap: 10px;
  max-height: 80vh;
  overflow-y: auto;
}
.nest-worlds-list-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.nest-worlds-list-items {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.nest-world-card {
  text-align: left;
  background: var(--nest-bg);
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
  padding: 8px 10px;
  cursor: pointer;
  transition: border-color var(--nest-transition-fast), background var(--nest-transition-fast);

  &:hover { border-color: var(--nest-border); }
  &.active {
    border-color: var(--nest-accent);
    background: var(--nest-surface);
  }
}
.nest-world-name {
  font-size: 13px;
  font-weight: 500;
  color: var(--nest-text);
}
.nest-world-meta {
  font-size: 11px;
  color: var(--nest-text-muted);
  margin-top: 2px;
}
.nest-world-desc {
  font-size: 11.5px;
  color: var(--nest-text-secondary);
  margin-top: 4px;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.nest-worlds-empty {
  padding: 12px 4px;
  color: var(--nest-text-muted);
}

.nest-worlds-editor {
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius);
  padding: 16px 20px;
  background: var(--nest-surface);
  min-width: 0;
}

.nest-state {
  padding: 60px 24px;
  display: grid;
  place-items: center;
  color: var(--nest-text-muted);
  text-align: center;
}

.nest-editor-head {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--nest-border-subtle);
  margin-bottom: 16px;
  flex-wrap: wrap;
}
.nest-editor-name { flex: 1 1 auto; min-width: 200px; }
.nest-editor-actions { display: flex; gap: 6px; }

:deep(.nest-world-name-input input) {
  font-family: var(--nest-font-display);
  font-size: 18px;
  color: var(--nest-text);
  padding: 2px 0;
}
:deep(.nest-world-desc-input input) {
  font-size: 12.5px;
  color: var(--nest-text-secondary);
  padding: 2px 0;
}

.nest-entries-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
}

.nest-entries-empty {
  color: var(--nest-text-muted);
  font-size: 13px;
  padding: 16px 0;
}

.nest-entries-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.nest-entry-row {
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
  background: var(--nest-bg);
  transition: border-color var(--nest-transition-fast), opacity var(--nest-transition-fast);

  &:hover { border-color: var(--nest-border); }
  &.expanded { border-color: var(--nest-accent); }
  &.disabled { opacity: 0.6; }
}
.nest-entry-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  cursor: pointer;
  gap: 8px;
}
.nest-entry-title {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex: 1;
  overflow: hidden;
}
.nest-entry-label {
  font-size: 13.5px;
  color: var(--nest-text);
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.nest-entry-cond {
  font-family: var(--nest-font-mono);
  font-size: 11px;
  color: var(--nest-text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.nest-entry-head-actions {
  display: flex;
  align-items: center;
  gap: 4px;
}
.nest-entry-body {
  padding: 12px 14px 16px;
  border-top: 1px dashed var(--nest-border-subtle);
}

.nest-entry-flags {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
}

.nest-entry-advanced {
  border-top: 1px dashed var(--nest-border-subtle);
  padding-top: 10px;
  margin-top: 4px;
}
.nest-entry-advanced-head {
  font-family: var(--nest-font-mono);
  font-size: 10.5px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--nest-text-muted);
  cursor: pointer;
  list-style: none;
  padding: 2px 0;

  &::-webkit-details-marker { display: none; }
  &::before {
    content: '▸';
    display: inline-block;
    margin-right: 6px;
    transition: transform var(--nest-transition-fast);
  }
}
details[open] > .nest-entry-advanced-head::before {
  transform: rotate(90deg);
}

.nest-entry-group { flex: 1 1 220px; min-width: 0; }

.nest-confirm {
  background: var(--nest-surface) !important;
  border: 1px solid var(--nest-border);
}

// Entry editor field sizes — flex-basis instead of rigid widths so they
// wrap cleanly on narrow viewports without clipping number spinners.
.nest-entry-keys     { flex: 1 1 240px; min-width: 0; }
.nest-entry-position { flex: 0 1 180px; min-width: 140px; }
.nest-entry-num      { flex: 0 1 120px; min-width: 90px; }

@media (max-width: 860px) {
  .nest-worlds-panel {
    grid-template-columns: 1fr;
  }
  .nest-worlds-list { max-height: none; }
}

// iPhone SE territory: stack the keys row fully, let number inputs be
// full-width. Editor right column gets a single visible column.
@media (max-width: 480px) {
  .nest-worlds-editor { padding: 12px; }
  .nest-entry-keys, .nest-entry-position { flex: 1 1 100%; }
  .nest-entry-num { flex: 1 1 calc(50% - 6px); }
  .nest-editor-head { flex-direction: column; align-items: stretch; }
  .nest-editor-name { min-width: 0; }
}
</style>
