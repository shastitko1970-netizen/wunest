<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type {
  OpenAIBundleData,
  PromptBlock,
  PromptOrderEntry,
} from '@/api/presets'

/**
 * PromptManagerPanel — the Prompt Manager tab inside a sampler/openai
 * preset editor. Mirrors SillyTavern's prompt-manager UI: every row is
 * a named prompt block that can be enabled / disabled / reordered /
 * edited inline. Markers (chatHistory, charDescription, …) are shown
 * but don't expose a content field — they're positional only.
 *
 * v-model carries the full bundle; we mutate prompts[] and prompt_order[]
 * directly on the model to keep the parent form's Save path simple.
 */

const { t } = useI18n()

const props = defineProps<{
  modelValue: OpenAIBundleData
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: OpenAIBundleData): void
}>()

// Wildcard group identifier used by ST for "applies to any character".
const WILDCARD_CHARACTER_ID = 100001

// ST reserved marker identifiers. Rows with these identifiers are rendered
// read-only (no content editor) because content is resolved server-side
// from the chat context.
const MARKER_IDENTIFIERS = new Set([
  'chatHistory',
  'worldInfoBefore',
  'worldInfoAfter',
  'dialogueExamples',
  'charDescription',
  'charPersonality',
  'scenario',
  'personaDescription',
  'jailbreak',
  'nsfw',
  'enhanceDefinitions',
])

// Shallow-proxy helper: returns the current wildcard PromptOrderGroup's
// order[] array. If the preset is missing one, we create it on demand so
// the UI can render a row list even for hand-made / empty bundles.
const orderList = computed<PromptOrderEntry[]>(() => {
  const groups = props.modelValue.prompt_order ?? []
  const wildcard = groups.find(g => g.character_id === WILDCARD_CHARACTER_ID)
  return wildcard?.order ?? (groups[0]?.order ?? [])
})

// prompt.identifier → PromptBlock for O(1) lookup.
const promptsByID = computed<Record<string, PromptBlock>>(() => {
  const map: Record<string, PromptBlock> = {}
  for (const p of props.modelValue.prompts ?? []) {
    map[p.identifier] = p
  }
  return map
})

// ─── Edit state ────────────────────────────────────────────────────
const expandedIdx = ref<number | null>(null)   // which order[] index is inline-editing
const addDialogOpen = ref(false)
const newPrompt = ref<{
  name: string
  role: string
  content: string
  position: number  // 0 = system, 1 = relative chat
  depth: number
}>({ name: '', role: 'system', content: '', position: 0, depth: 4 })

function toggleExpand(idx: number) {
  expandedIdx.value = expandedIdx.value === idx ? null : idx
}

function isMarker(id: string): boolean { return MARKER_IDENTIFIERS.has(id) }

function roleColor(role?: string): string {
  switch (role) {
    case 'system':    return 'primary'
    case 'assistant': return 'success'
    case 'user':      return 'warning'
    default:          return ''
  }
}

// Compact preview for each row (first ~80 chars of content). Empty for
// markers that don't carry content.
function previewContent(id: string): string {
  if (isMarker(id)) return t(`presets.prompt.markerHint.${id}`) || t('presets.prompt.markerGeneric')
  const p = promptsByID.value[id]
  if (!p?.content) return ''
  const trimmed = p.content.trim().replace(/\s+/g, ' ')
  return trimmed.length > 80 ? trimmed.slice(0, 77) + '…' : trimmed
}

function displayName(id: string): string {
  const p = promptsByID.value[id]
  if (p?.name) return p.name
  // Marker fallback — use the identifier itself.
  return id
}

// ─── Mutations ─────────────────────────────────────────────────────

function setBundle(next: OpenAIBundleData) {
  emit('update:modelValue', next)
}

function updateOrder(nextOrder: PromptOrderEntry[]) {
  const groups = [...(props.modelValue.prompt_order ?? [])]
  let wildcardIdx = groups.findIndex(g => g.character_id === WILDCARD_CHARACTER_ID)
  if (wildcardIdx < 0 && groups.length > 0) {
    // Reuse first group if no wildcard exists yet.
    wildcardIdx = 0
  }
  if (wildcardIdx < 0) {
    groups.push({ character_id: WILDCARD_CHARACTER_ID, order: nextOrder })
  } else {
    groups[wildcardIdx] = { ...groups[wildcardIdx], order: nextOrder }
  }
  setBundle({ ...props.modelValue, prompt_order: groups })
}

function updatePrompts(nextPrompts: PromptBlock[]) {
  setBundle({ ...props.modelValue, prompts: nextPrompts })
}

function toggleEnabled(idx: number) {
  const next = [...orderList.value]
  next[idx] = { ...next[idx], enabled: !next[idx].enabled }
  updateOrder(next)
}

function moveUp(idx: number) {
  if (idx <= 0) return
  const next = [...orderList.value]
  ;[next[idx - 1], next[idx]] = [next[idx], next[idx - 1]]
  updateOrder(next)
}
function moveDown(idx: number) {
  if (idx >= orderList.value.length - 1) return
  const next = [...orderList.value]
  ;[next[idx], next[idx + 1]] = [next[idx + 1], next[idx]]
  updateOrder(next)
}

function editPromptField<K extends keyof PromptBlock>(
  identifier: string,
  key: K,
  value: PromptBlock[K],
) {
  const next = (props.modelValue.prompts ?? []).map(p =>
    p.identifier === identifier ? { ...p, [key]: value } : p,
  )
  updatePrompts(next)
}

function removeOrderEntry(idx: number) {
  const id = orderList.value[idx]?.identifier
  if (!id) return
  // Remove from order.
  const nextOrder = orderList.value.filter((_, i) => i !== idx)
  updateOrder(nextOrder)
  // Optionally remove from prompts[] too — but ST keeps orphan prompts
  // around so we match that behavior. The user can still re-add via the
  // "Add from prompts[]" flow (if we build one). For now just drop from
  // order; prompt data is preserved.
}

function addNewPrompt() {
  const id = cryptoRandomID()
  const name = newPrompt.value.name.trim() || 'Untitled prompt'
  const block: PromptBlock = {
    identifier: id,
    name,
    role: newPrompt.value.role,
    content: newPrompt.value.content,
    injection_position: newPrompt.value.position,
    injection_depth: newPrompt.value.depth,
    injection_order: 100,
  }
  const nextPrompts = [...(props.modelValue.prompts ?? []), block]
  const nextOrder = [...orderList.value, { identifier: id, enabled: true }]
  setBundle({
    ...props.modelValue,
    prompts: nextPrompts,
    prompt_order: updateOrderOnBundle(props.modelValue, nextOrder).prompt_order,
  })
  addDialogOpen.value = false
  newPrompt.value = { name: '', role: 'system', content: '', position: 0, depth: 4 }
}

// Side-effect-free: returns a new bundle with prompt_order replaced.
function updateOrderOnBundle(b: OpenAIBundleData, nextOrder: PromptOrderEntry[]): OpenAIBundleData {
  const groups = [...(b.prompt_order ?? [])]
  let idx = groups.findIndex(g => g.character_id === WILDCARD_CHARACTER_ID)
  if (idx < 0 && groups.length > 0) idx = 0
  if (idx < 0) {
    groups.push({ character_id: WILDCARD_CHARACTER_ID, order: nextOrder })
  } else {
    groups[idx] = { ...groups[idx], order: nextOrder }
  }
  return { ...b, prompt_order: groups }
}

// Tiny UUID-ish id for a new custom prompt. Not cryptographically unique,
// but stable per-session is good enough since the server treats it as a
// string identifier and collisions within one preset are what matter.
function cryptoRandomID(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return crypto.randomUUID()
  }
  return 'p-' + Math.random().toString(36).slice(2, 12)
}

// ─── Stats for header ──────────────────────────────────────────────
const enabledCount = computed(() => orderList.value.filter(e => e.enabled).length)
const totalCount   = computed(() => orderList.value.length)
</script>

<template>
  <div class="nest-prompt-manager">
    <!-- Header row — stats + add button. -->
    <div class="nest-pm-head">
      <div class="nest-pm-stats nest-mono">
        {{ t('presets.prompt.stats', { enabled: enabledCount, total: totalCount }) }}
      </div>
      <v-btn
        size="small"
        variant="outlined"
        prepend-icon="mdi-plus"
        @click="addDialogOpen = true"
      >
        {{ t('presets.prompt.addBtn') }}
      </v-btn>
    </div>

    <!-- Empty state (unusual — a valid bundle has at least markers). -->
    <div v-if="totalCount === 0" class="nest-pm-empty">
      {{ t('presets.prompt.empty') }}
    </div>

    <!-- List of prompts in prompt_order. Each row: enable toggle + name
         chip + role chip + content preview, expands to full editor. -->
    <div v-else class="nest-pm-list">
      <div
        v-for="(entry, idx) in orderList"
        :key="entry.identifier + idx"
        class="nest-pm-row"
        :class="{ disabled: !entry.enabled, expanded: expandedIdx === idx }"
      >
        <div class="nest-pm-row-main" @click="toggleExpand(idx)">
          <!-- Enable toggle. Stop propagation so clicking the checkbox
               doesn't also expand the row. -->
          <v-checkbox-btn
            :model-value="entry.enabled"
            color="success"
            density="compact"
            hide-details
            @click.stop
            @update:model-value="toggleEnabled(idx)"
          />

          <!-- Name / identifier -->
          <div class="nest-pm-name">
            {{ displayName(entry.identifier) }}
            <v-chip
              v-if="isMarker(entry.identifier)"
              size="x-small"
              variant="tonal"
              class="nest-mono ml-1"
            >
              {{ t('presets.prompt.markerBadge') }}
            </v-chip>
          </div>

          <!-- Role chip for non-marker prompts. -->
          <v-chip
            v-if="!isMarker(entry.identifier) && promptsByID[entry.identifier]?.role"
            size="x-small"
            :color="roleColor(promptsByID[entry.identifier]?.role)"
            variant="tonal"
            class="nest-mono"
          >
            {{ promptsByID[entry.identifier]?.role }}
          </v-chip>

          <!-- Short content preview. -->
          <div class="nest-pm-preview">{{ previewContent(entry.identifier) }}</div>

          <!-- Reorder / delete icons. -->
          <div class="nest-pm-row-actions" @click.stop>
            <v-btn
              size="x-small"
              variant="text"
              icon="mdi-chevron-up"
              :disabled="idx === 0"
              :title="t('presets.prompt.moveUp')"
              @click="moveUp(idx)"
            />
            <v-btn
              size="x-small"
              variant="text"
              icon="mdi-chevron-down"
              :disabled="idx === totalCount - 1"
              :title="t('presets.prompt.moveDown')"
              @click="moveDown(idx)"
            />
            <v-btn
              size="x-small"
              variant="text"
              color="error"
              icon="mdi-delete-outline"
              :title="t('presets.prompt.removeFromOrder')"
              @click="removeOrderEntry(idx)"
            />
          </div>
        </div>

        <!-- Inline editor for non-marker prompts. -->
        <div
          v-if="expandedIdx === idx && !isMarker(entry.identifier) && promptsByID[entry.identifier]"
          class="nest-pm-row-edit"
        >
          <div class="nest-field-row">
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">{{ t('presets.prompt.nameLabel') }}</label>
              <v-text-field
                :model-value="promptsByID[entry.identifier]?.name"
                density="compact" hide-details
                @update:model-value="v => editPromptField(entry.identifier, 'name', v)"
              />
            </div>
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">{{ t('presets.prompt.roleLabel') }}</label>
              <v-select
                :model-value="promptsByID[entry.identifier]?.role"
                :items="[
                  { value: 'system', title: 'system' },
                  { value: 'user', title: 'user' },
                  { value: 'assistant', title: 'assistant' },
                ]"
                density="compact" hide-details
                @update:model-value="v => editPromptField(entry.identifier, 'role', v)"
              />
            </div>
          </div>

          <div class="nest-field-row">
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">
                {{ t('presets.prompt.positionLabel') }}
                <v-tooltip location="top" :text="t('presets.prompt.positionHint')">
                  <template #activator="{ props: p }">
                    <v-icon v-bind="p" size="12" class="nest-hint-icon">mdi-information-outline</v-icon>
                  </template>
                </v-tooltip>
              </label>
              <v-select
                :model-value="promptsByID[entry.identifier]?.injection_position ?? 0"
                :items="[
                  { value: 0, title: t('presets.prompt.posSystem') },
                  { value: 1, title: t('presets.prompt.posRelative') },
                ]"
                density="compact" hide-details
                @update:model-value="v => editPromptField(entry.identifier, 'injection_position', v)"
              />
            </div>
            <div
              v-if="(promptsByID[entry.identifier]?.injection_position ?? 0) === 1"
              class="nest-field nest-field-half"
            >
              <label class="nest-field-label">{{ t('presets.prompt.depthLabel') }}</label>
              <v-text-field
                :model-value="promptsByID[entry.identifier]?.injection_depth ?? 0"
                type="number" :min="0" :max="50"
                density="compact" hide-details
                @update:model-value="v => editPromptField(entry.identifier, 'injection_depth', Number(v))"
              />
            </div>
          </div>

          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.prompt.contentLabel') }}</label>
            <v-textarea
              :model-value="promptsByID[entry.identifier]?.content"
              rows="6" auto-grow density="compact" hide-details
              class="nest-pm-content-textarea"
              @update:model-value="v => editPromptField(entry.identifier, 'content', v)"
            />
          </div>
        </div>
      </div>
    </div>

    <!-- Add-new-prompt dialog. -->
    <v-dialog v-model="addDialogOpen" max-width="520">
      <v-card class="nest-pm-addcard">
        <v-card-title>{{ t('presets.prompt.addDialogTitle') }}</v-card-title>
        <v-card-text>
          <div class="nest-field-row">
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">{{ t('presets.prompt.nameLabel') }}</label>
              <v-text-field v-model="newPrompt.name" density="compact" hide-details autofocus />
            </div>
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">{{ t('presets.prompt.roleLabel') }}</label>
              <v-select
                v-model="newPrompt.role"
                :items="[
                  { value: 'system', title: 'system' },
                  { value: 'user', title: 'user' },
                  { value: 'assistant', title: 'assistant' },
                ]"
                density="compact" hide-details
              />
            </div>
          </div>
          <div class="nest-field-row">
            <div class="nest-field nest-field-half">
              <label class="nest-field-label">{{ t('presets.prompt.positionLabel') }}</label>
              <v-select
                v-model="newPrompt.position"
                :items="[
                  { value: 0, title: t('presets.prompt.posSystem') },
                  { value: 1, title: t('presets.prompt.posRelative') },
                ]"
                density="compact" hide-details
              />
            </div>
            <div v-if="newPrompt.position === 1" class="nest-field nest-field-half">
              <label class="nest-field-label">{{ t('presets.prompt.depthLabel') }}</label>
              <v-text-field
                v-model.number="newPrompt.depth"
                type="number" :min="0" :max="50"
                density="compact" hide-details
              />
            </div>
          </div>
          <div class="nest-field">
            <label class="nest-field-label">{{ t('presets.prompt.contentLabel') }}</label>
            <v-textarea v-model="newPrompt.content" rows="4" auto-grow density="compact" hide-details />
          </div>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="addDialogOpen = false">{{ t('common.cancel') }}</v-btn>
          <v-btn color="primary" variant="flat" @click="addNewPrompt">
            {{ t('presets.prompt.addBtn') }}
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<style lang="scss" scoped>
.nest-prompt-manager {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.nest-pm-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 10px;
  padding: 2px 4px;
}
.nest-pm-stats {
  font-size: 11.5px;
  color: var(--nest-text-muted);
}

.nest-pm-empty {
  padding: 24px;
  text-align: center;
  color: var(--nest-text-muted);
  font-size: 13px;
  border: 1px dashed var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
}

.nest-pm-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.nest-pm-row {
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
  background: var(--nest-surface);
  transition: border-color var(--nest-transition-fast);

  &:hover:not(.expanded) {
    border-color: var(--nest-border);
  }
  &.disabled {
    opacity: 0.55;
  }
  &.expanded {
    border-color: var(--nest-accent);
  }
}

.nest-pm-row-main {
  display: grid;
  grid-template-columns: auto auto auto 1fr auto;
  gap: 8px;
  align-items: center;
  padding: 6px 10px;
  cursor: pointer;
  min-height: 36px;
}

.nest-pm-name {
  font-size: 13px;
  color: var(--nest-text);
  font-weight: 500;
  display: flex;
  align-items: center;
  white-space: nowrap;
}

.nest-pm-preview {
  font-size: 11.5px;
  color: var(--nest-text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}

.nest-pm-row-actions {
  display: flex;
  gap: 0;
}

.nest-pm-row-edit {
  padding: 10px 14px 14px;
  border-top: 1px solid var(--nest-border-subtle);
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.nest-pm-content-textarea {
  :deep(textarea) {
    font-family: var(--nest-font-mono) !important;
    font-size: 12px !important;
    line-height: 1.5 !important;
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

.nest-pm-addcard {
  background: var(--nest-surface) !important;
  border: 1px solid var(--nest-border);
}

// Mobile — let the preview drop below name+chips so the row is still
// legible at 375px.
@media (max-width: 560px) {
  .nest-pm-row-main {
    grid-template-columns: auto 1fr auto;
    grid-template-areas:
      "check name actions"
      "check preview actions";
    row-gap: 2px;
  }
  .nest-pm-preview { grid-area: preview; white-space: normal; }
}
</style>
