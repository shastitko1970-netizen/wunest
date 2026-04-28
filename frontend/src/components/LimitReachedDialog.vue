<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useSubscriptionStore } from '@/stores/subscription'

// Global "you've hit your slot cap" dialog (M54.2).
//
// Mounted once at the app shell so any failed Create call in any page
// can surface its 402 here without each page carrying its own dialog
// markup. Triggered via `subscription.showLimitReached(detail)`; the
// detail comes straight from the server's structured 402 envelope.

const { t } = useI18n()
const router = useRouter()
const sub = useSubscriptionStore()
const { limitReached } = storeToRefs(sub)

const open = computed({
  get: () => limitReached.value !== null,
  set: (v) => { if (!v) sub.dismissLimitReached() },
})

// Map the resource enum to a user-friendly label. The keys live in
// chat-translations bundle; future locales are added to the same set
// the rest of the SPA uses.
const resourceLabel = computed(() => {
  const r = limitReached.value?.resource
  if (!r) return ''
  return t(`subscription.limit.resource.${r}`)
})

function dismiss() {
  sub.dismissLimitReached()
}

function goUpgrade() {
  sub.dismissLimitReached()
  void router.push('/subscription')
}
</script>

<template>
  <v-dialog v-model="open" max-width="420">
    <v-card v-if="limitReached" class="nest-limit-card">
      <v-card-title class="nest-limit-title">
        <v-icon size="22" color="warning" class="mr-2">mdi-alert-octagon-outline</v-icon>
        {{ t('subscription.limit.title') }}
      </v-card-title>
      <v-card-text class="nest-limit-text">
        <p>
          {{ t('subscription.limit.body', {
            resource: resourceLabel,
            current: limitReached.current,
            max: limitReached.max,
          }) }}
        </p>
        <p class="nest-limit-hint">{{ t('subscription.limit.hint') }}</p>
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn variant="text" @click="dismiss">
          {{ t('common.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="flat"
          prepend-icon="mdi-rocket-launch-outline"
          @click="goUpgrade"
        >
          {{ t('subscription.limit.upgrade') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<style lang="scss" scoped>
.nest-limit-card {
  background: var(--nest-surface) !important;
  border: 1px solid var(--nest-border);
}
.nest-limit-title {
  display: flex;
  align-items: center;
  font-family: var(--nest-font-display);
  font-size: 1.05rem;
  letter-spacing: -0.005em;
}
.nest-limit-text {
  font-size: 14px;
  line-height: 1.55;
  p { margin: 0 0 10px; }
  p:last-child { margin-bottom: 0; }
}
.nest-limit-hint {
  color: var(--nest-text-secondary);
  font-size: 13px;
}
</style>
