import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  defaultsApi,
  presetsApi,
  type Preset,
  type PresetType,
  type SamplerData,
} from '@/api/presets'
import { useAuthStore } from '@/stores/auth'

/**
 * Presets store — holds every type of preset the user has saved, plus the
 * map of per-type *active* presets (server settings.default_presets JSONB;
 * exposed as user.active_presets on /api/me from M30).
 *
 * Active vs default: M30 "variant-1" treats the active preset as THE
 * global source for prompt assembly. There is one active preset per type
 * per user; it applies to every chat (existing + new) until the user
 * flips a different one. The legacy `default` naming on the server stays
 * for backward compat, but the UI concept is "active".
 *
 * Fetch is lazy: drawers / manager pages call fetchAll() on mount.
 */
export const usePresetsStore = defineStore('presets', () => {
  const items = ref<Preset[]>([])
  // active[type] = preset id (UUID string). Empty/missing = none active.
  // Kept in sync with auth.profile.active_presets after every server write
  // so the two views never diverge.
  const active = ref<Record<string, string>>({})
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
      // Pull the preset list alongside the active map. Active comes from
      // auth.profile (cached from /api/me) when available; falls back to
      // the dedicated defaultsApi call for callers that ran before auth
      // resolved.
      const list = await presetsApi.list()
      items.value = list.items
      const auth = useAuthStore()
      if (auth.profile?.active_presets) {
        active.value = { ...auth.profile.active_presets }
      } else {
        const defs = await defaultsApi.list()
        active.value = defs.default_presets ?? {}
      }
      loaded.value = true
    } catch (e) {
      error.value = (e as Error).message
    } finally {
      loading.value = false
    }
  }

  /**
   * Create a preset. If no preset of the same type is currently active
   * AND the caller didn't explicitly opt out (`autoActivate: false`), the
   * new preset is activated automatically — most users' intent after an
   * import is "use this now", not "save for later".
   *
   * Returns `{preset, activated}` so the caller can show an accurate
   * confirmation ("imported and applied" vs just "imported").
   */
  async function create(
    type: PresetType,
    name: string,
    data: unknown,
    options: { autoActivate?: boolean } = {},
  ): Promise<{ preset: Preset; activated: boolean }> {
    const p = await presetsApi.create({ type, name, data })
    items.value = [...items.value, p].sort((a, b) => a.name.localeCompare(b.name))

    let activated = false
    const shouldAutoActivate = options.autoActivate !== false
    if (shouldAutoActivate && !active.value[type]) {
      await setActive(type, p.id)
      activated = true
    }
    return { preset: p, activated }
  }

  async function createSampler(name: string, data: SamplerData): Promise<Preset> {
    const result = await create('sampler', name, data)
    return result.preset
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
    // Also drop the active pointer if we just nuked the current one.
    for (const [type, def] of Object.entries(active.value)) {
      if (def === id) delete active.value[type]
    }
    syncAuthMirror()
  }

  /**
   * Set `presetID` as the active preset for `type`, or clear it with null.
   * Writes through to the server and mirrors the new value into auth.profile
   * so any other consumer (chat header chips, etc.) reactively updates.
   */
  async function setActive(type: PresetType, presetID: string | null) {
    await defaultsApi.set(type, presetID)
    if (presetID) {
      active.value = { ...active.value, [type]: presetID }
    } else {
      const next = { ...active.value }
      delete next[type]
      active.value = next
    }
    syncAuthMirror()
  }

  function isActive(p: Preset): boolean {
    return active.value[p.type] === p.id
  }

  function activeID(type: PresetType): string | null {
    return active.value[type] ?? null
  }

  function activePreset(type: PresetType): Preset | null {
    const id = active.value[type]
    if (!id) return null
    return items.value.find(p => p.id === id) ?? null
  }

  function syncAuthMirror() {
    const auth = useAuthStore()
    if (auth.profile) {
      auth.profile.active_presets = { ...active.value }
    }
  }

  return {
    items, active, samplers, loading, loaded, error,
    byType, isActive, activeID, activePreset,
    fetchAll, create, createSampler, update, remove, setActive,
  }
})
