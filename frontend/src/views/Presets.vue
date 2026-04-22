<script setup lang="ts">
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'

// Legacy redirect: the preset manager used to be a standalone page but now
// lives as a tab inside /library. Keep the route registered so old links /
// bookmarks don't 404, but immediately bounce to library?tab=presets so the
// correct tab is selected.
const router = useRouter()
onMounted(() => {
  router.replace({ path: '/library', query: { tab: 'presets' } })
})
</script>

<template>
  <div class="nest-legacy-redirect">
    <v-progress-circular indeterminate color="primary" size="24" />
  </div>
</template>

<style lang="scss" scoped>
.nest-legacy-redirect {
  padding: 60px;
  display: grid;
  place-items: center;
}
</style>
