import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { modelsApi, type Model } from '@/api/models'
import type { Chat } from '@/api/chats'

// Active source of model lists. `wuapi` means the shared WuApi pool; a
// byok_id string means the catalogue is live-fetched from that saved key's
// provider (/api/byok/{id}/models).
type ModelsSource = 'wuapi' | { byokID: string }

function sourceKey(src: ModelsSource): string {
  return typeof src === 'string' ? src : `byok:${src.byokID}`
}

function sourceFromChat(chat: Chat | null | undefined): ModelsSource {
  const byokID = chat?.chat_metadata?.byok_id
  if (typeof byokID === 'string' && byokID.length > 0) return { byokID }
  return 'wuapi'
}

// Persisted "last model picked" per source, so switching provider and coming
// back doesn't snap you back to a bogus wu-kitsune on openai.
const LS_PREFIX = 'nest:last-model:'
function loadRemembered(src: ModelsSource): string | null {
  return localStorage.getItem(LS_PREFIX + sourceKey(src))
}
function remember(src: ModelsSource, model: string) {
  localStorage.setItem(LS_PREFIX + sourceKey(src), model)
}

// Cached catalogues so re-opening a chat doesn't re-fetch for no reason.
// Keyed by sourceKey(src). Cleared on explicit refresh() or on app reload.
type CacheRow = { models: Model[]; loaded: boolean }

export const useModelsStore = defineStore('models', () => {
  const cache = ref<Record<string, CacheRow>>({})
  const loading = ref(false)
  const error = ref<string | null>(null)

  // The currently-active source (set by Chat view on chat open / BYOK switch).
  const activeSource = ref<ModelsSource>('wuapi')

  const items = computed<Model[]>(() => {
    const row = cache.value[sourceKey(activeSource.value)]
    return row?.models ?? []
  })
  const loaded = computed<boolean>(() => {
    const row = cache.value[sourceKey(activeSource.value)]
    return !!row?.loaded
  })

  // The currently-selected model id for the active source. Falls back to the
  // first available model when the remembered value isn't in the fresh list
  // (e.g. provider removed a model, or first visit to a new provider).
  const selected = ref<string>('')

  async function fetchFor(src: ModelsSource, opts?: { refresh?: boolean }) {
    const key = sourceKey(src)
    if (!opts?.refresh && cache.value[key]?.loaded) {
      return cache.value[key].models
    }
    loading.value = true
    error.value = null
    try {
      let res: { data?: Model[] }
      if (src === 'wuapi') {
        res = await modelsApi.list()
      } else {
        res = await modelsApi.listForBYOK(src.byokID, opts?.refresh === true)
      }
      const models = res?.data ?? []
      cache.value = { ...cache.value, [key]: { models, loaded: true } }
      return models
    } catch (e) {
      error.value = (e as Error).message
      // On failure we still mark the row as loaded (with empty list) so the
      // picker can show "no models available" instead of a perpetual spinner.
      cache.value = { ...cache.value, [key]: { models: [], loaded: true } }
      return []
    } finally {
      loading.value = false
    }
  }

  // Pick a source (wuapi or a specific BYOK), fetch its catalogue, and select
  // either the remembered model for that source or the first available one.
  async function setActiveSource(src: ModelsSource) {
    activeSource.value = src
    const models = await fetchFor(src)
    const remembered = loadRemembered(src)
    const pick =
      (remembered && models.find(m => m.id === remembered)?.id) ||
      models[0]?.id ||
      ''
    selected.value = pick
    if (pick) remember(src, pick)
  }

  // Convenience for the Chat view: derive the source from the chat's metadata.
  async function setForChat(chat: Chat | null | undefined) {
    await setActiveSource(sourceFromChat(chat))
  }

  function select(id: string) {
    selected.value = id
    remember(activeSource.value, id)
  }

  async function refresh() {
    await fetchFor(activeSource.value, { refresh: true })
    // Keep the selected value if still present; otherwise reselect first.
    if (!items.value.find(m => m.id === selected.value)) {
      const first = items.value[0]?.id ?? ''
      selected.value = first
      if (first) remember(activeSource.value, first)
    }
  }

  return {
    activeSource,
    items,
    loaded,
    loading,
    error,
    selected,
    setActiveSource,
    setForChat,
    select,
    refresh,
  }
})
