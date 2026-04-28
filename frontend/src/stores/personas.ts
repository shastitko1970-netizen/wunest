import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { personasApi, type Persona, type PersonaCreateInput, type PersonaUpdatePatch } from '@/api/personas'
import { isLimitReached } from '@/api/client'
import { useSubscriptionStore } from '@/stores/subscription'

/**
 * Personas store — list of the user's personas. Exactly one may be the
 * default (enforced server-side via transaction). Shape is small enough
 * that we always load the full list, no summaries.
 */
export const usePersonasStore = defineStore('personas', () => {
  const items = ref<Persona[]>([])
  const loading = ref(false)
  const loaded = ref(false)
  const error = ref<string | null>(null)

  const defaultPersona = computed<Persona | null>(
    () => items.value.find(p => p.is_default) ?? null,
  )

  async function fetchAll(force = false) {
    if (loaded.value && !force) return
    loading.value = true
    error.value = null
    try {
      const { items: list } = await personasApi.list()
      items.value = list
      loaded.value = true
    } catch (e) {
      error.value = (e as Error).message
    } finally {
      loading.value = false
    }
  }

  async function create(input: PersonaCreateInput): Promise<Persona> {
    try {
      const p = await personasApi.create(input)
      // If newly-created is default, demote others locally.
      if (p.is_default) items.value = items.value.map(x => ({ ...x, is_default: false }))
      items.value = [p, ...items.value]
      return p
    } catch (e) {
      if (isLimitReached(e)) {
        useSubscriptionStore().showLimitReached(e.detail)
      }
      throw e
    }
  }

  async function update(id: string, patch: PersonaUpdatePatch): Promise<Persona> {
    const p = await personasApi.update(id, patch)
    const idx = items.value.findIndex(x => x.id === id)
    if (idx >= 0) items.value[idx] = p
    return p
  }

  async function setDefault(id: string | null) {
    if (id) {
      await personasApi.setDefault(id)
      items.value = items.value.map(p => ({ ...p, is_default: p.id === id }))
    } else {
      await personasApi.clearDefault()
      items.value = items.value.map(p => ({ ...p, is_default: false }))
    }
  }

  async function remove(id: string) {
    await personasApi.delete(id)
    items.value = items.value.filter(p => p.id !== id)
  }

  return { items, loading, loaded, error, defaultPersona, fetchAll, create, update, setDefault, remove }
})
