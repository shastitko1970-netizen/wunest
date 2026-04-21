<script setup lang="ts">
import { useRoute } from 'vue-router'

const route = useRoute()
const characterId = route.query.character as string | undefined
</script>

<template>
  <v-container class="nest-stub">
    <div class="nest-eyebrow">Chat</div>
    <h1 class="nest-h1 mt-1">Chat</h1>
    <v-alert
      class="mt-6"
      type="info"
      variant="tonal"
      text="Chat UI is the focus of M3. This placeholder confirms routing works."
    />
    <p v-if="characterId" class="nest-subtitle mt-4 nest-mono">
      Requested character: {{ characterId }}
    </p>
  </v-container>
</template>

<style lang="scss" scoped>
.nest-stub {
  max-width: 820px;
  padding: 32px 24px;
}
</style>
