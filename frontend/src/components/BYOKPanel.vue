<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useDisplay } from 'vuetify'
import { byokApi, type BYOKKey } from '@/api/byok'

// BYOKPanel — manage "bring-your-own" provider API keys. Keys are
// encrypted at rest server-side using AES-GCM and never round-trip
// back to the client in plaintext. The list shows a masked preview
// (e.g. "sk-…6411"); to use a key, pin it to a chat via the chat-
// header picker.

const { t } = useI18n()
const { smAndDown } = useDisplay()

const items = ref<BYOKKey[]>([])
const providers = ref<string[]>([])
const loading = ref(false)
const error = ref<string | null>(null)

// Add form state
const formProvider = ref('openai')
const formLabel = ref('')
const formKey = ref('')
const saving = ref(false)
const addOpen = ref(false)
const confirmDeleteId = ref<string | null>(null)

onMounted(async () => {
  loading.value = true
  try {
    const [list, provs] = await Promise.all([
      byokApi.list(),
      byokApi.providers(),
    ])
    items.value = list.items
    providers.value = provs.items
    if (provs.items.length > 0 && !provs.items.includes(formProvider.value)) {
      formProvider.value = provs.items[0]
    }
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    loading.value = false
  }
})

function openAdd() {
  formProvider.value = providers.value[0] ?? 'openai'
  formLabel.value = ''
  formKey.value = ''
  error.value = null
  addOpen.value = true
}

async function save() {
  if (!formKey.value.trim()) {
    error.value = t('byok.errors.keyRequired')
    return
  }
  saving.value = true
  error.value = null
  try {
    const created = await byokApi.create({
      provider: formProvider.value,
      label: formLabel.value.trim() || undefined,
      key: formKey.value.trim(),
    })
    items.value = [created, ...items.value]
    // Wipe local plaintext buffer immediately after persist succeeds.
    formKey.value = ''
    formLabel.value = ''
    addOpen.value = false
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    saving.value = false
  }
}

async function doDelete() {
  if (!confirmDeleteId.value) return
  const id = confirmDeleteId.value
  confirmDeleteId.value = null
  try {
    await byokApi.delete(id)
    items.value = items.value.filter(k => k.id !== id)
  } catch (e) {
    error.value = (e as Error).message
  }
}

const groupedByProvider = computed<Record<string, BYOKKey[]>>(() => {
  const g: Record<string, BYOKKey[]> = {}
  for (const k of items.value) {
    if (!g[k.provider]) g[k.provider] = []
    g[k.provider].push(k)
  }
  return g
})

function providerLabel(p: string): string {
  // Keep it simple — no i18n for provider IDs, they're brand names.
  return p.charAt(0).toUpperCase() + p.slice(1)
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString()
}
</script>

<template>
  <section class="nest-byok-panel">
    <div class="nest-byok-head">
      <div>
        <div class="nest-eyebrow">{{ t('byok.eyebrow') }}</div>
        <h2 class="nest-h2 mt-1">{{ t('byok.title') }}</h2>
        <p class="nest-subtitle mt-1">{{ t('byok.tagline') }}</p>
      </div>
      <v-btn
        color="primary"
        variant="flat"
        prepend-icon="mdi-key-plus"
        @click="openAdd"
      >
        {{ t('byok.add') }}
      </v-btn>
    </div>

    <v-alert v-if="error" type="error" variant="tonal" density="compact" class="mt-3">
      {{ error }}
    </v-alert>

    <div v-if="loading" class="nest-state">
      <v-progress-circular indeterminate color="primary" size="24" />
    </div>
    <div v-else-if="items.length === 0" class="nest-byok-empty">
      {{ t('byok.empty') }}
    </div>
    <div v-else class="nest-byok-groups">
      <div v-for="(keys, provider) in groupedByProvider" :key="provider" class="nest-byok-group">
        <h3 class="nest-h4">{{ providerLabel(provider) }}</h3>
        <div class="nest-byok-list">
          <div v-for="k in keys" :key="k.id" class="nest-byok-row">
            <div class="nest-byok-meta">
              <div class="nest-byok-label">
                {{ k.label || t('byok.unnamed') }}
              </div>
              <div class="nest-byok-mask nest-mono">{{ k.masked }}</div>
            </div>
            <div class="nest-byok-actions">
              <span class="nest-mono nest-byok-date">{{ formatDate(k.created_at) }}</span>
              <v-btn
                size="x-small"
                variant="text"
                color="error"
                icon="mdi-delete-outline"
                :title="t('common.delete')"
                @click="confirmDeleteId = k.id"
              />
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Add dialog. Fullscreen on phones so the password-type key field
         has space for long API keys (Anthropic ones are ~100 chars). -->
    <v-dialog v-model="addOpen" :max-width="smAndDown ? undefined : 500" :fullscreen="smAndDown">
      <v-card class="nest-byok-dialog">
        <v-card-title class="nest-byok-dialog-title">
          <span>{{ t('byok.addDialog.title') }}</span>
          <v-btn icon="mdi-close" variant="text" size="small" @click="addOpen = false" />
        </v-card-title>
        <v-card-text>
          <p class="nest-subtitle mb-3">{{ t('byok.addDialog.safety') }}</p>
          <div class="nest-field">
            <label class="nest-field-label">{{ t('byok.form.provider') }}</label>
            <v-select
              v-model="formProvider"
              :items="providers.map(p => ({ value: p, title: providerLabel(p) }))"
              item-title="title"
              item-value="value"
              density="compact"
              hide-details
            />
          </div>
          <div class="nest-field mt-3">
            <label class="nest-field-label">
              {{ t('byok.form.label') }}
              <span class="nest-field-hint-inline">{{ t('byok.form.labelHint') }}</span>
            </label>
            <v-text-field
              v-model="formLabel"
              :placeholder="t('byok.form.labelPlaceholder')"
              density="compact"
              hide-details
            />
          </div>
          <div class="nest-field mt-3">
            <label class="nest-field-label">{{ t('byok.form.key') }}</label>
            <v-text-field
              v-model="formKey"
              type="password"
              autocomplete="off"
              :placeholder="t('byok.form.keyPlaceholder')"
              density="compact"
              hide-details
            />
          </div>
        </v-card-text>
        <v-card-actions class="px-6 pb-4">
          <v-spacer />
          <v-btn variant="text" :disabled="saving" @click="addOpen = false">
            {{ t('common.cancel') }}
          </v-btn>
          <v-btn
            color="primary"
            variant="flat"
            :loading="saving"
            :disabled="!formKey.trim()"
            @click="save"
          >
            {{ t('byok.form.save') }}
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Delete confirmation -->
    <v-dialog
      :model-value="confirmDeleteId !== null"
      max-width="400"
      @update:model-value="v => !v && (confirmDeleteId = null)"
    >
      <v-card class="nest-confirm">
        <v-card-title>{{ t('byok.delete.title') }}</v-card-title>
        <v-card-text>{{ t('byok.delete.body') }}</v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="confirmDeleteId = null">{{ t('common.cancel') }}</v-btn>
          <v-btn color="error" variant="flat" @click="doDelete">{{ t('common.delete') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </section>
</template>

<style lang="scss" scoped>
.nest-byok-panel {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.nest-byok-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  flex-wrap: wrap;
}

.nest-byok-empty {
  color: var(--nest-text-muted);
  font-size: 13px;
  padding: 12px 0;
}

.nest-state {
  padding: 40px;
  display: grid;
  place-items: center;
}

.nest-byok-groups {
  display: flex;
  flex-direction: column;
  gap: 18px;
}
.nest-byok-group h3 {
  margin-bottom: 6px;
}

.nest-byok-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.nest-byok-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-sm);
  background: var(--nest-surface);
}

.nest-byok-meta { min-width: 0; }
.nest-byok-label {
  font-size: 13.5px;
  color: var(--nest-text);
  font-weight: 500;
}
.nest-byok-mask {
  font-size: 12px;
  color: var(--nest-text-muted);
  margin-top: 2px;
}

.nest-byok-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.nest-byok-date {
  font-size: 11px;
  color: var(--nest-text-muted);
}

.nest-byok-dialog {
  background: var(--nest-surface) !important;
  border: 1px solid var(--nest-border);
}
.nest-byok-dialog-title {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px 20px 8px;
}

.nest-field { display: flex; flex-direction: column; gap: 6px; }
.nest-field-label {
  display: flex;
  justify-content: space-between;
  font-family: var(--nest-font-mono);
  font-size: 10.5px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--nest-text-muted);
}
.nest-field-hint-inline {
  text-transform: none;
  letter-spacing: 0;
  font-size: 10px;
  color: var(--nest-text-muted);
  opacity: 0.8;
}

.nest-confirm {
  background: var(--nest-surface) !important;
  border: 1px solid var(--nest-border);
}

// Phones. Key rows collapse: the date becomes a line under the masked
// preview instead of taking a column, keeping the delete button anchored
// right without squeezing the meta. Add button in the head wraps below
// the title rather than sitting beside it on a 375px screen.
@media (max-width: 520px) {
  .nest-byok-head {
    gap: 10px;
    .v-btn { width: 100%; }
  }
  .nest-byok-row {
    gap: 6px;
    padding: 10px;
    align-items: flex-start;
  }
  .nest-byok-actions {
    flex-direction: column;
    align-items: flex-end;
    gap: 4px;
  }
  .nest-byok-date {
    font-size: 10px;
    white-space: nowrap;
  }
  .nest-byok-dialog-title {
    padding: 14px 14px 6px;
    font-size: 16px;
  }
}
</style>
