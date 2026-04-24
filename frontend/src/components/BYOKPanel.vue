<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useDisplay } from 'vuetify'
import { byokApi, type BYOKKey, type BYOKProviderInfo, type BYOKTestResult } from '@/api/byok'

// BYOKPanel — manage "bring-your-own" provider API keys. Keys are
// encrypted at rest server-side using AES-GCM and never round-trip
// back to the client in plaintext. The list shows a masked preview
// (e.g. "sk-…6411"); to use a key, pin it to a chat via the chat-
// header picker.
//
// Each key also carries the `base_url` it routes to, so when the chat
// stream dispatches to this key it goes DIRECTLY to the provider
// (openai.com, openrouter, etc.) rather than through WuApi. That's what
// makes a raw `sk-proj-...` OpenAI key actually work.

const { t } = useI18n()
const { smAndDown } = useDisplay()

const items = ref<BYOKKey[]>([])
const providers = ref<BYOKProviderInfo[]>([])
const loading = ref(false)
const error = ref<string | null>(null)

// Add form state
const formProvider = ref('openai')
const formLabel = ref('')
const formKey = ref('')
const formBaseURL = ref('')
const saving = ref(false)
const addOpen = ref(false)
const confirmDeleteId = ref<string | null>(null)

const currentProviderInfo = computed(() =>
  providers.value.find(p => p.id === formProvider.value) ?? null,
)

// Whenever the user picks a new provider, reset the base URL to that
// provider's canonical default. User can still override afterwards.
watch(formProvider, (p) => {
  const info = providers.value.find(x => x.id === p)
  formBaseURL.value = info?.default_url ?? ''
})

onMounted(async () => {
  loading.value = true
  try {
    const [list, provs] = await Promise.all([
      byokApi.list(),
      byokApi.providers(),
    ])
    items.value = list.items
    providers.value = provs.items
    if (provs.items.length > 0 && !provs.items.some(p => p.id === formProvider.value)) {
      formProvider.value = provs.items[0].id
    }
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    loading.value = false
  }
})

function openAdd() {
  const first = providers.value[0]
  formProvider.value = first?.id ?? 'openai'
  formLabel.value = ''
  formKey.value = ''
  formBaseURL.value = first?.default_url ?? ''
  error.value = null
  addOpen.value = true
}

async function save() {
  if (!formKey.value.trim()) {
    error.value = t('byok.errors.keyRequired')
    return
  }
  if (formProvider.value === 'custom' && !formBaseURL.value.trim()) {
    error.value = t('byok.errors.urlRequired')
    return
  }
  saving.value = true
  error.value = null
  try {
    const created = await byokApi.create({
      provider: formProvider.value,
      label: formLabel.value.trim() || undefined,
      key: formKey.value.trim(),
      base_url: formBaseURL.value.trim() || undefined,
    })
    items.value = [created, ...items.value]
    // Wipe local plaintext buffer immediately after persist succeeds.
    formKey.value = ''
    formLabel.value = ''
    formBaseURL.value = ''
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
    delete testResults.value[id]
  } catch (e) {
    error.value = (e as Error).message
  }
}

// Per-key Test state. Map id → { loading, result } so each row has its own
// spinner and result card without clobbering siblings when the user tests
// several keys in a row.
const testResults = ref<Record<string, { loading: boolean; result: BYOKTestResult | null }>>({})

async function testKey(id: string) {
  testResults.value = {
    ...testResults.value,
    [id]: { loading: true, result: null },
  }
  try {
    const r = await byokApi.test(id)
    testResults.value[id] = { loading: false, result: r }
  } catch (e) {
    // Network failure, auth failure, etc — surface raw message so user
    // can tell "bad cookie" from "bad provider key".
    testResults.value[id] = {
      loading: false,
      result: { ok: false, error: (e as Error).message },
    }
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
          <div v-for="k in keys" :key="k.id" class="nest-byok-row-wrap">
            <div class="nest-byok-row">
              <div class="nest-byok-meta">
                <div class="nest-byok-label">
                  {{ k.label || t('byok.unnamed') }}
                </div>
                <div class="nest-byok-mask nest-mono">{{ k.masked }}</div>
                <div v-if="k.base_url" class="nest-byok-url nest-mono" :title="k.base_url">
                  {{ k.base_url.replace(/^https?:\/\//, '') }}
                </div>
              </div>
              <div class="nest-byok-actions">
                <v-btn
                  size="x-small"
                  variant="tonal"
                  :prepend-icon="testResults[k.id]?.result?.ok === true
                    ? 'mdi-check-circle-outline'
                    : testResults[k.id]?.result?.ok === false
                      ? 'mdi-alert-circle-outline'
                      : 'mdi-connection'"
                  :loading="testResults[k.id]?.loading"
                  :color="testResults[k.id]?.result?.ok === true
                    ? 'success'
                    : testResults[k.id]?.result?.ok === false
                      ? 'error'
                      : undefined"
                  @click="testKey(k.id)"
                >
                  {{ t('byok.test.button') }}
                </v-btn>
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
            <!-- Test result card — green when ok, red on error. Shows the
                 first 3 model ids so the user sees proof that /models
                 actually returned real data from their provider. -->
            <div
              v-if="testResults[k.id]?.result"
              class="nest-byok-test-result"
              :class="{ ok: testResults[k.id]!.result!.ok, fail: !testResults[k.id]!.result!.ok }"
            >
              <div v-if="testResults[k.id]!.result!.ok" class="nest-byok-test-success">
                <v-icon size="14" color="success">mdi-check-circle</v-icon>
                <span>
                  {{ t('byok.test.success', { n: testResults[k.id]!.result!.model_count ?? 0 }) }}
                </span>
                <span v-if="testResults[k.id]!.result!.sample?.length" class="nest-mono nest-byok-test-sample">
                  {{ testResults[k.id]!.result!.sample!.join(', ') }}…
                </span>
              </div>
              <div v-else class="nest-byok-test-fail">
                <v-icon size="14" color="error">mdi-alert-circle</v-icon>
                <span class="nest-mono">{{ testResults[k.id]!.result!.error }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Add dialog. Fullscreen on phones so the password-type key field
         has space for long API keys (Anthropic ones are ~100 chars). -->
    <v-dialog v-model="addOpen" :max-width="smAndDown ? undefined : 500" :fullscreen="smAndDown" scrollable>
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
              :items="providers.map(p => ({ value: p.id, title: providerLabel(p.id) }))"
              item-title="title"
              item-value="value"
              density="compact"
              hide-details
            />
          </div>

          <!-- Base URL — pre-filled from the picked provider's default,
               but editable so power users can point at a custom proxy
               (e.g. self-hosted LiteLLM, a regional OpenAI endpoint, or
               a local llama.cpp server). Required when provider=custom. -->
          <div class="nest-field mt-3">
            <label class="nest-field-label">
              {{ t('byok.form.baseUrl') }}
              <span class="nest-field-hint-inline">
                {{ formProvider === 'custom'
                  ? t('byok.form.baseUrlRequired')
                  : t('byok.form.baseUrlHint') }}
              </span>
            </label>
            <v-text-field
              v-model="formBaseURL"
              :placeholder="currentProviderInfo?.default_url || 'https://api.openai.com/v1'"
              density="compact"
              hide-details
              spellcheck="false"
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

          <!-- Gentle notice for anthropic/google — their native APIs are
               not OpenAI-compat by default; the default URL we ship is
               their opt-in compat endpoint which may need extra headers. -->
          <v-alert
            v-if="formProvider === 'anthropic' || formProvider === 'google'"
            type="warning"
            variant="tonal"
            density="compact"
            class="mt-3"
            style="font-size: 11.5px"
          >
            {{ t('byok.form.compatNote') }}
          </v-alert>
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

.nest-byok-row-wrap {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.nest-byok-test-result {
  padding: 8px 12px;
  border-radius: var(--nest-radius-sm);
  font-size: 11.5px;
  display: flex;
  flex-direction: column;
  gap: 4px;

  &.ok {
    background: rgba(76, 175, 80, 0.08);
    border: 1px solid rgba(76, 175, 80, 0.25);
  }
  &.fail {
    background: rgba(244, 67, 54, 0.08);
    border: 1px solid rgba(244, 67, 54, 0.25);
  }
}
.nest-byok-test-success,
.nest-byok-test-fail {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--nest-text-secondary);
  flex-wrap: wrap;
}
.nest-byok-test-fail {
  color: var(--nest-text);
  word-break: break-word;

  span {
    font-size: 11px;
  }
}
.nest-byok-test-sample {
  font-size: 10.5px;
  opacity: 0.75;
  word-break: break-all;
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
.nest-byok-url {
  font-size: 10.5px;
  color: var(--nest-text-muted);
  opacity: 0.8;
  margin-top: 1px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
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
