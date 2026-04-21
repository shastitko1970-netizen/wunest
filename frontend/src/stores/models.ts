import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { modelsApi, type Model } from '@/api/models'

// Fallback wu-tier models used when /api/models hasn't loaded yet (or
// fails). Order matches WuApi's mapping.go, so the picker defaults to the
// same "cheapest free" option as the chat handler.
const FALLBACK_MODELS: Model[] = [
  { id: 'wu-kitsune' },
  { id: 'wu-tanuki' },
  { id: 'wu-inari' },
  { id: 'wu-raijin' },
  { id: 'wu-ryujin' },
  { id: 'wu-amaterasu' },
]

const DEFAULT_MODEL = 'wu-kitsune'
const LS_LAST_MODEL = 'nest:last-model'

export const useModelsStore = defineStore('models', () => {
  const items = ref<Model[]>([])
  const loading = ref(false)
  const loaded = ref(false)
  const error = ref<string | null>(null)

  // Selected model — persisted in localStorage so it sticks across reloads.
  const selected = ref<string>(localStorage.getItem(LS_LAST_MODEL) ?? DEFAULT_MODEL)

  const options = computed<Model[]>(() => (items.value.length > 0 ? items.value : FALLBACK_MODELS))

  async function fetchList() {
    if (loading.value) return
    loading.value = true
    error.value = null
    try {
      const res = await modelsApi.list()
      items.value = res?.data ?? []
      loaded.value = true
      // If the previously-selected model isn't in the fresh list, fall back.
      if (items.value.length > 0 && !items.value.find(m => m.id === selected.value)) {
        select(items.value[0]!.id)
      }
    } catch (e) {
      error.value = (e as Error).message
      // Keep FALLBACK_MODELS via options computed — picker still works.
    } finally {
      loading.value = false
    }
  }

  function select(id: string) {
    selected.value = id
    localStorage.setItem(LS_LAST_MODEL, id)
  }

  return { items, options, loading, loaded, error, selected, fetchList, select }
})
