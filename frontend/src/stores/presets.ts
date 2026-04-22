import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  defaultsApi,
  presetsApi,
  type Preset,
  type PresetType,
  type SamplerData,
} from '@/api/presets'

/**
 * Presets store — holds every type of preset the user has saved, plus the
 * map of per-type defaults (settings.default_presets on the server).
 *
 * Fetch is lazy: drawers / manager pages call fetchAll() on mount.
 */
export const usePresetsStore = defineStore('presets', () => {
  const items = ref<Preset[]>([])
  const defaults = ref<Record<string, string>>({})
  const loading = ref(false)
  const loaded = ref(false)
  const error = ref<string | null>(null)

  const samplers = computed<Preset[]>(() =>
    items.value.filter(p => p.type === 'sampler'),
  )

  function byType(type: PresetType): Preset[] {
    return items.value.filter(p => p.type === type)
  }

  async function fetchAll(force = false) {
    if (loaded.value && !force) return
    loading.value = true
    error.value = null
    try {
      const [list, defs] = await Promise.all([
        presetsApi.list(),
        defaultsApi.list(),
      ])
      items.value = list.items
      defaults.value = defs.default_presets ?? {}
      loaded.value = true
    } catch (e) {
      error.value = (e as Error).message
    } finally {
      loading.value = false
    }
  }

  async function create(type: PresetType, name: string, data: unknown): Promise<Preset> {
    const p = await presetsApi.create({ type, name, data })
    items.value = [...items.value, p].sort((a, b) => a.name.localeCompare(b.name))
    return p
  }

  async function createSampler(name: string, data: SamplerData): Promise<Preset> {
    return create('sampler', name, data)
  }

  async function update(id: string, patch: { name?: string; data?: unknown }) {
    const p = await presetsApi.update(id, patch)
    const idx = items.value.findIndex(x => x.id === id)
    if (idx >= 0) items.value[idx] = p
    items.value = [...items.value].sort((a, b) => a.name.localeCompare(b.name))
    return p
  }

  async function remove(id: string) {
    await presetsApi.delete(id)
    items.value = items.value.filter(x => x.id !== id)
    // Also drop the default if we just nuked the active one.
    for (const [type, def] of Object.entries(defaults.value)) {
      if (def === id) delete defaults.value[type]
    }
  }

  /** Set (or clear) the user's default preset for `type`. */
  async function setDefault(type: PresetType, presetID: string | null) {
    await defaultsApi.set(type, presetID)
    if (presetID) {
      defaults.value = { ...defaults.value, [type]: presetID }
    } else {
      const next = { ...defaults.value }
      delete next[type]
      defaults.value = next
    }
  }

  function isDefault(p: Preset): boolean {
    return defaults.value[p.type] === p.id
  }

  return {
    items, defaults, samplers, loading, loaded, error,
    byType, isDefault,
    fetchAll, create, createSampler, update, remove, setDefault,
  }
})
