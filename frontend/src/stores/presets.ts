import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { presetsApi, type Preset, type SamplerData } from '@/api/presets'

/**
 * Sampler-preset store. Only `type=sampler` presets in v1 — the other
 * types (instruct, context, sysprompt, reasoning) exist in the schema
 * but don't yet have a UI. The store fetches lazily on first drawer open.
 */
export const usePresetsStore = defineStore('presets', () => {
  const items = ref<Preset[]>([])
  const loading = ref(false)
  const loaded = ref(false)
  const error = ref<string | null>(null)

  const samplers = computed<Preset[]>(() =>
    items.value.filter(p => p.type === 'sampler'),
  )

  async function fetchAll(force = false) {
    if (loaded.value && !force) return
    loading.value = true
    error.value = null
    try {
      const res = await presetsApi.list('sampler')
      items.value = res.items
      loaded.value = true
    } catch (e) {
      error.value = (e as Error).message
    } finally {
      loading.value = false
    }
  }

  async function create(name: string, data: SamplerData): Promise<Preset> {
    const p = await presetsApi.create({ type: 'sampler', name, data })
    items.value = [...items.value, p].sort((a, b) => a.name.localeCompare(b.name))
    return p
  }

  async function update(id: string, patch: { name?: string; data?: SamplerData }) {
    const p = await presetsApi.update(id, patch)
    const idx = items.value.findIndex(x => x.id === id)
    if (idx >= 0) items.value[idx] = p
    items.value = [...items.value].sort((a, b) => a.name.localeCompare(b.name))
    return p
  }

  async function remove(id: string) {
    await presetsApi.delete(id)
    items.value = items.value.filter(x => x.id !== id)
  }

  return { items, samplers, loading, loaded, error, fetchAll, create, update, remove }
})
