<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useSubscriptionStore } from '@/stores/subscription'

// Compact "X of Y" usage chip shown in Library panel headers (M54.3).
// Mounts in CharactersPanel / WorldsPanel / PersonasPanel / PresetsPanel
// and surfaces "you've used 3 of 10 slots" so the user sees the cap
// before they hit it. Clickable: navigates to /subscription so users
// who max out have a one-click route to the upgrade page.
//
// Pulls slot_limit from the subscription store (no extra API call —
// AppShell already loads /api/me/subscription on init in M54.4 once).
//
// Hidden when slotLimit is unlimited (Pro tier) — there's nothing to
// nag about.

const props = defineProps<{
  /** Caller-supplied current count. Each panel knows its own length
   *  via its store; we don't try to be clever and re-count. */
  used: number
}>()

const { t } = useI18n()
const sub = useSubscriptionStore()
const { state, slotLimit } = storeToRefs(sub)

onMounted(() => {
  // Lazy-fetch state if no one populated it yet. Fire-and-forget: if
  // it fails we just hide the chip until next page-mount retry.
  if (!state.value) void sub.fetchState()
})

/** Hide entirely on Pro / unlimited — no cap to surface. */
const visible = computed(() => Number.isFinite(slotLimit.value))

const label = computed(() => {
  if (!Number.isFinite(slotLimit.value)) {
    return t('subscription.usage.chipUnlimited', { used: props.used })
  }
  return t('subscription.usage.chip', {
    used: props.used,
    total: slotLimit.value,
  })
})

/** Approaching cap — last slot. Triggers the warning style so the
 *  user sees it's nearly full at a glance. */
const nearCap = computed(() => {
  if (!Number.isFinite(slotLimit.value)) return false
  return props.used >= (slotLimit.value as number) - 0
})
</script>

<template>
  <router-link
    v-if="visible"
    to="/subscription"
    class="nest-usage-chip"
    :class="{ 'is-near-cap': nearCap }"
    :title="t('subscription.account.upgrade')"
  >
    <v-icon size="13" class="mr-1">mdi-account-multiple-outline</v-icon>
    <span>{{ label }}</span>
  </router-link>
</template>

<style lang="scss" scoped>
.nest-usage-chip {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  padding: 3px 9px;
  font-size: 11.5px;
  font-family: var(--nest-font-mono);
  letter-spacing: 0.02em;
  color: var(--nest-text-secondary);
  background: var(--nest-bg-elevated);
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius-pill);
  text-decoration: none;
  transition: border-color var(--nest-transition-fast), color var(--nest-transition-fast);

  &:hover {
    color: var(--nest-text);
    border-color: var(--nest-accent);
  }

  &.is-near-cap {
    color: var(--nest-text);
    border-color: rgba(255, 184, 0, 0.5);
    background: rgba(255, 184, 0, 0.08);
  }
}
</style>
