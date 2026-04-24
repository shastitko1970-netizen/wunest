<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useQuickRepliesStore } from '@/stores/quickReplies'
import type { QuickReply } from '@/api/quickReplies'

// Quick-reply manager. Renders as a Settings section — list of chips
// that can be edited inline + "Add new" row. No drag-sort in v1;
// users can edit Position manually if order matters.

const { t } = useI18n()
const store = useQuickRepliesStore()
const { items, loading } = storeToRefs(store)

onMounted(() => { if (!store.loaded) void store.fetchAll() })

const newLabel = ref('')
const newText = ref('')
const newSendNow = ref(false)
const editingId = ref<string | null>(null)
const editingLabel = ref('')
const editingText = ref('')
const editingSendNow = ref(false)

async function addNew() {
  const text = newText.value.trim()
  if (!text) return
  await store.create({
    label: newLabel.value.trim() || undefined,
    text,
    send_now: newSendNow.value,
  })
  newLabel.value = ''
  newText.value = ''
  newSendNow.value = false
}

function startEdit(qr: QuickReply) {
  editingId.value = qr.id
  editingLabel.value = qr.label
  editingText.value = qr.text
  editingSendNow.value = qr.send_now
}

async function saveEdit() {
  if (!editingId.value) return
  await store.update(editingId.value, {
    label: editingLabel.value.trim(),
    text: editingText.value.trim(),
    send_now: editingSendNow.value,
  })
  editingId.value = null
}

function cancelEdit() { editingId.value = null }

async function removeQR(qr: QuickReply) {
  if (!confirm(t('quickReplies.confirmDelete', { label: qr.label }))) return
  await store.remove(qr.id)
}
</script>

<template>
  <div class="nest-qr-panel">
    <div class="nest-eyebrow">{{ t('quickReplies.title') }}</div>
    <p class="nest-hint mb-3">{{ t('quickReplies.hint') }}</p>

    <div v-if="loading" class="text-center py-4">
      <v-progress-circular indeterminate size="24" />
    </div>

    <div v-else-if="items.length === 0" class="nest-state py-3">
      <v-icon size="32" color="medium-emphasis">mdi-lightning-bolt-outline</v-icon>
      <div class="text-body-2 mt-2">{{ t('quickReplies.empty') }}</div>
    </div>

    <div v-else class="nest-qr-list mb-3">
      <div v-for="qr in items" :key="qr.id" class="nest-qr-row">
        <template v-if="editingId === qr.id">
          <div class="nest-qr-edit">
            <v-text-field
              v-model="editingLabel"
              :label="t('quickReplies.labelLabel')"
              density="compact"
              variant="outlined"
              hide-details
            />
            <v-textarea
              v-model="editingText"
              :label="t('quickReplies.textLabel')"
              rows="2"
              auto-grow
              density="compact"
              variant="outlined"
              hide-details
            />
            <div class="d-flex align-center ga-2">
              <v-switch
                v-model="editingSendNow"
                :label="t('quickReplies.sendNow')"
                density="compact"
                hide-details
                color="primary"
              />
              <v-spacer />
              <v-btn variant="text" size="small" @click="cancelEdit">
                {{ t('common.cancel') }}
              </v-btn>
              <v-btn variant="flat" size="small" color="primary" @click="saveEdit">
                {{ t('common.save') }}
              </v-btn>
            </div>
          </div>
        </template>
        <template v-else>
          <div class="nest-qr-row-text">
            <div class="nest-qr-row-label">
              <v-icon v-if="qr.send_now" size="14" color="primary">mdi-send</v-icon>
              <strong>{{ qr.label }}</strong>
            </div>
            <div class="nest-qr-row-preview">{{ qr.text }}</div>
          </div>
          <div class="d-flex ga-1">
            <v-btn size="small" variant="text" icon="mdi-pencil-outline" @click="startEdit(qr)" />
            <v-btn size="small" variant="text" color="error" icon="mdi-delete-outline" @click="removeQR(qr)" />
          </div>
        </template>
      </div>
    </div>

    <div class="nest-qr-new">
      <div class="nest-eyebrow mb-2">{{ t('quickReplies.addTitle') }}</div>
      <v-text-field
        v-model="newLabel"
        :label="t('quickReplies.labelLabel')"
        :placeholder="t('quickReplies.labelPlaceholder')"
        density="compact"
        variant="outlined"
        hide-details
        class="mb-2"
      />
      <v-textarea
        v-model="newText"
        :label="t('quickReplies.textLabel')"
        :placeholder="t('quickReplies.textPlaceholder')"
        rows="2"
        auto-grow
        density="compact"
        variant="outlined"
        hide-details
        class="mb-2"
      />
      <div class="d-flex align-center ga-2">
        <v-switch
          v-model="newSendNow"
          :label="t('quickReplies.sendNow')"
          density="compact"
          hide-details
          color="primary"
        />
        <v-spacer />
        <v-btn
          variant="flat"
          color="primary"
          size="small"
          prepend-icon="mdi-plus"
          :disabled="!newText.trim()"
          @click="addNew"
        >
          {{ t('quickReplies.add') }}
        </v-btn>
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.nest-qr-panel { padding: 0; }
.nest-hint {
  font-size: 12.5px;
  color: var(--nest-text-muted);
  line-height: 1.45;
}
.nest-state {
  text-align: center;
  color: var(--nest-text-muted);
}
.nest-qr-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.nest-qr-row {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 10px 12px;
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius);
  background: var(--nest-bg-elevated);
}
.nest-qr-row-text { flex: 1; min-width: 0; }
.nest-qr-row-label {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 13.5px;
}
.nest-qr-row-preview {
  font-size: 11.5px;
  color: var(--nest-text-muted);
  margin-top: 2px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.nest-qr-edit {
  display: flex;
  flex-direction: column;
  gap: 6px;
  width: 100%;
}
.nest-qr-new {
  padding: 10px 12px;
  border: 1px dashed var(--nest-border-subtle);
  border-radius: var(--nest-radius);
}
</style>
