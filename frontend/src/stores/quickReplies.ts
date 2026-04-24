import { defineStore } from 'pinia'
import { ref } from 'vue'
import { quickRepliesApi, type QuickReply } from '@/api/quickReplies'

// Minimal Pinia store for quick-reply CRUD. Fetched once on first
// demand (from MessageInput or Settings), mutated in place via
// optimistic splice after server confirms.
export const useQuickRepliesStore = defineStore('quickReplies', () => {
  const items = ref<QuickReply[]>([])
  const loaded = ref(false)
  const loading = ref(false)

  async function fetchAll(force = false) {
    if (loaded.value && !force) return
    loading.value = true
    try {
      const res = await quickRepliesApi.list()
      items.value = res.items
      loaded.value = true
    } finally {
      loading.value = false
    }
  }

  async function create(input: { label?: string; text: string; send_now?: boolean }) {
    const rep = await quickRepliesApi.create(input)
    items.value.push(rep)
    items.value.sort((a, b) => a.position - b.position)
    return rep
  }

  async function update(id: string, patch: Partial<Pick<QuickReply, 'label' | 'text' | 'position' | 'send_now'>>) {
    const rep = await quickRepliesApi.update(id, patch)
    const idx = items.value.findIndex(x => x.id === id)
    if (idx >= 0) items.value[idx] = rep
    return rep
  }

  async function remove(id: string) {
    await quickRepliesApi.delete(id)
    items.value = items.value.filter(x => x.id !== id)
  }

  return { items, loaded, loading, fetchAll, create, update, remove }
})
