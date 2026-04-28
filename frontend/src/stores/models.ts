import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { modelsApi, type Model, type CatalogModel } from '@/api/models'
import type { Chat } from '@/api/chats'

// Active source of model lists.
//
//   'wuapi' — the shared WuApi pool, fetched via /v1/models.
//   'wueco' — virtual M55 provider: same models as WuApi, but mapped
//             to their `:lite` eco-mode variants. List comes from the
//             gold catalog filter (eco != null), no extra fetch.
//   { byokID } — live-fetched from a stored BYOK key's provider.
type ModelsSource = 'wuapi' | 'wueco' | { byokID: string }

function sourceKey(src: ModelsSource): string {
  return typeof src === 'string' ? src : `byok:${src.byokID}`
}

function sourceFromChat(chat: Chat | null | undefined): ModelsSource {
  const byokID = chat?.chat_metadata?.byok_id
  if (typeof byokID === 'string' && byokID.length > 0) return { byokID }
  // M55 — eco_mode in chat metadata switches the source to the
  // virtual WuEco provider. Survives chat reopens because the flag
  // lives on chat_metadata.
  if (chat?.chat_metadata?.eco_mode === true) return 'wueco'
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

  // Wu-gold catalog (full + WuNest-only `:lite`). Loaded once per
  // session and looked up by id when the SPA needs eco-limits or
  // pricing context.
  const catalog = ref<CatalogModel[]>([])
  const catalogLoaded = ref(false)
  const catalogByID = computed<Record<string, CatalogModel>>(() => {
    const out: Record<string, CatalogModel> = {}
    for (const m of catalog.value) out[m.id] = m
    return out
  })

  async function fetchCatalog(opts?: { refresh?: boolean }) {
    if (!opts?.refresh && catalogLoaded.value) return catalog.value
    try {
      const data = await modelsApi.catalog()
      catalog.value = data ?? []
      catalogLoaded.value = true
      return catalog.value
    } catch (e) {
      // Non-fatal: SPA still works without the eco metadata, picker
      // just won't show the dedicated section. Print warn for ops.
      console.warn('models.fetchCatalog', e)
      catalogLoaded.value = true
      return []
    }
  }

  /** Returns the catalog entry for the currently-selected model, or
   *  null if not in the gold catalog (wu-tier / BYOK / unknown id). */
  function catalogEntry(id: string): CatalogModel | null {
    return catalogByID.value[id] ?? null
  }

  /** True iff `id` is a `:lite` (eco-mode) variant. */
  function isEco(id: string): boolean {
    return !!catalogByID.value[id]?.eco
  }

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
      let models: Model[] = []
      if (src === 'wuapi') {
        const res = await modelsApi.list()
        models = res?.data ?? []
      } else if (src === 'wueco') {
        // M55 — WuEco virtual provider. List comes from the gold
        // catalog (already loaded once per session) filtered to
        // entries with `eco != null`. No extra HTTP request — the
        // catalog has everything we need.
        await fetchCatalog()
        models = catalog.value
          .filter(m => m.eco)
          .map(m => ({
            id: m.id,
            object: 'model',
            owned_by: 'wueco',
          }))
      } else {
        const res = await modelsApi.listForBYOK(src.byokID, opts?.refresh === true)
        models = res?.data ?? []
      }
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
    // M55.2 — eco catalog
    catalog,
    catalogLoaded,
    fetchCatalog,
    catalogEntry,
    isEco,
  }
})
