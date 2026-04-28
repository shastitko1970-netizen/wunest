import { defineStore } from 'pinia'
import { ref } from 'vue'
import { worldsApi, type World, type WorldSummary, type WorldEntry } from '@/api/worlds'
import { isLimitReached } from '@/api/client'
import { useSubscriptionStore } from '@/stores/subscription'

/**
 * Worlds store — catalogue of user-owned lorebooks plus a lazy cache of full
 * books (entries) keyed by id. The list view uses `items` (summaries, small
 * payload); the editor hydrates a full `World` via `loadFull(id)`.
 */
export const useWorldsStore = defineStore('worlds', () => {
  const items = ref<WorldSummary[]>([])
  const cache = ref<Record<string, World>>({})
  const loading = ref(false)
  const loaded = ref(false)
  const error = ref<string | null>(null)

  async function fetchAll(force = false) {
    if (loaded.value && !force) return
    loading.value = true
    error.value = null
    try {
      const { items: list } = await worldsApi.list()
      items.value = list
      loaded.value = true
    } catch (e) {
      error.value = (e as Error).message
    } finally {
      loading.value = false
    }
  }

  async function loadFull(id: string, force = false): Promise<World> {
    if (!force && cache.value[id]) return cache.value[id]
    const full = await worldsApi.get(id)
    cache.value = { ...cache.value, [id]: full }
    return full
  }

  async function create(name: string, description = '', entries: WorldEntry[] = []): Promise<World> {
    try {
      const w = await worldsApi.create({ name, description, entries })
      cache.value = { ...cache.value, [w.id]: w }
      items.value = [toSummary(w), ...items.value]
      return w
    } catch (e) {
      if (isLimitReached(e)) {
        useSubscriptionStore().showLimitReached(e.detail)
      }
      throw e
    }
  }

  async function update(id: string, patch: { name?: string; description?: string; entries?: WorldEntry[] }): Promise<World> {
    const w = await worldsApi.update(id, patch)
    cache.value = { ...cache.value, [w.id]: w }
    const idx = items.value.findIndex(s => s.id === id)
    if (idx >= 0) items.value[idx] = toSummary(w)
    return w
  }

  async function remove(id: string) {
    await worldsApi.delete(id)
    items.value = items.value.filter(s => s.id !== id)
    const next = { ...cache.value }
    delete next[id]
    cache.value = next
  }

  async function importST(name: string, description: string, entries: unknown): Promise<World> {
    try {
      const w = await worldsApi.importST({ name, description, entries })
      cache.value = { ...cache.value, [w.id]: w }
      items.value = [toSummary(w), ...items.value]
      return w
    } catch (e) {
      if (isLimitReached(e)) {
        useSubscriptionStore().showLimitReached(e.detail)
      }
      throw e
    }
  }

  function toSummary(w: World): WorldSummary {
    return {
      id: w.id,
      name: w.name,
      description: w.description,
      entry_count: w.entries?.length ?? 0,
      updated_at: w.updated_at,
    }
  }

  return { items, cache, loading, loaded, error, fetchAll, loadFull, create, update, remove, importST }
})
