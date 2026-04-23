import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { charactersApi, type Character } from '@/api/characters'

export const useCharactersStore = defineStore('characters', () => {
  const items = ref<Character[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  const query = ref('')
  const activeTag = ref<string | null>(null)
  const favoriteOnly = ref(false)

  async function fetchAll() {
    loading.value = true
    error.value = null
    try {
      const { items: fetched } = await charactersApi.list()
      // Go marshals nil slices as `null`; coerce so empty-list reads
      // never hit `.length of null` in consumers.
      items.value = Array.isArray(fetched) ? fetched : []
    } catch (e) {
      error.value = (e as Error).message
    } finally {
      loading.value = false
    }
  }

  // Accepts PNG or JSON — backend sniffs the magic bytes.
  async function importCard(file: File): Promise<Character> {
    const created = await charactersApi.importCard(file)
    items.value = [created, ...items.value]
    return created
  }

  async function create(input: Partial<Character>): Promise<Character> {
    const created = await charactersApi.create(input)
    items.value = [created, ...items.value]
    return created
  }

  async function update(id: string, patch: Partial<Character>): Promise<Character> {
    const updated = await charactersApi.update(id, patch)
    const idx = items.value.findIndex(c => c.id === id)
    if (idx >= 0) items.value[idx] = updated
    return updated
  }

  async function remove(id: string): Promise<void> {
    await charactersApi.delete(id)
    items.value = items.value.filter(c => c.id !== id)
  }

  async function toggleFavorite(c: Character) {
    return update(c.id, { favorite: !c.favorite })
  }

  // Derived: all unique tags in the library, sorted by frequency.
  const allTags = computed(() => {
    const freq = new Map<string, number>()
    for (const c of items.value) {
      for (const tag of c.tags) freq.set(tag, (freq.get(tag) ?? 0) + 1)
    }
    return [...freq.entries()]
      .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
      .map(([tag, count]) => ({ tag, count }))
  })

  // Derived: filtered view.
  const filtered = computed(() => {
    const q = query.value.trim().toLowerCase()
    return items.value.filter(c => {
      if (favoriteOnly.value && !c.favorite) return false
      if (activeTag.value && !c.tags.includes(activeTag.value)) return false
      if (q && !c.name.toLowerCase().includes(q)) return false
      return true
    })
  })

  return {
    items, loading, error, query, activeTag, favoriteOnly,
    allTags, filtered,
    fetchAll, importCard, create, update, remove, toggleFavorite,
  }
})
