<script setup lang="ts">
import { computed, ref } from 'vue'
import { useDisplay } from 'vuetify'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { useAuthStore } from '@/stores/auth'
import { useI18n } from 'vue-i18n'

const { mdAndUp } = useDisplay()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const { profile } = storeToRefs(auth)
const { t, locale } = useI18n()

// On desktop the drawer is persistent; on mobile it's an overlay.
const drawerOpen = ref(false)

const navItems = computed(() => [
  { to: '/chat', icon: 'mdi-forum-outline', label: t('nav.chat') },
  { to: '/library', icon: 'mdi-bookshelf', label: t('nav.library') },
  { to: '/settings', icon: 'mdi-cog-outline', label: t('nav.settings') },
  { to: '/studio', icon: 'mdi-wrench-outline', label: t('nav.studio'), disabled: true },
])

const goldDisplay = computed(() =>
  profile.value ? (profile.value.gold_balance_nano / 1_000_000_000).toFixed(2) : '—',
)

function toggleLocale() {
  const next = locale.value === 'ru' ? 'en' : 'ru'
  locale.value = next
  localStorage.setItem('nest:locale', next)
}
</script>

<template>
  <v-layout class="nest-shell">
    <!-- Top bar -->
    <v-app-bar
      flat
      :elevation="0"
      class="nest-topbar px-2"
      height="56"
    >
      <v-app-bar-nav-icon
        v-if="!mdAndUp"
        variant="text"
        @click="drawerOpen = !drawerOpen"
      />
      <div class="d-flex align-center ga-2 cursor-pointer" @click="router.push('/')">
        <div class="nest-logo-mark">▲</div>
        <div class="nest-logo-text">WuNest</div>
      </div>

      <v-spacer />

      <template v-if="profile">
        <v-chip
          v-if="mdAndUp"
          size="small"
          variant="tonal"
          color="secondary"
          class="mr-2 nest-mono"
        >
          {{ goldDisplay }} gold
        </v-chip>
        <v-chip
          v-if="mdAndUp"
          size="small"
          variant="tonal"
          class="mr-2 nest-mono"
        >
          {{ profile.used_today }}/{{ profile.daily_limit || '∞' }}
        </v-chip>
      </template>

      <v-btn
        icon="mdi-translate"
        variant="text"
        size="small"
        @click="toggleLocale"
      />

      <v-menu v-if="profile">
        <template #activator="{ props }">
          <v-btn icon v-bind="props" size="small" variant="text">
            <v-avatar size="32" color="primary">
              <span class="text-caption">{{ (profile.first_name || profile.username || '?')[0] }}</span>
            </v-avatar>
          </v-btn>
        </template>
        <v-list density="compact">
          <v-list-item
            :title="profile.first_name || profile.username"
            :subtitle="profile.tier"
          />
          <v-divider />
          <v-list-item
            :title="t('nav.manageAccount')"
            prepend-icon="mdi-open-in-new"
            href="https://wusphere.ru/dashboard"
            target="_blank"
          />
        </v-list>
      </v-menu>
    </v-app-bar>

    <!-- Sidebar nav -->
    <v-navigation-drawer
      v-model="drawerOpen"
      :permanent="mdAndUp"
      :temporary="!mdAndUp"
      width="240"
      class="nest-sidebar"
      :elevation="0"
    >
      <v-list nav density="comfortable" class="pa-2">
        <v-list-item
          v-for="item in navItems"
          :key="item.to"
          :to="item.disabled ? undefined : item.to"
          :prepend-icon="item.icon"
          :title="item.label"
          :disabled="item.disabled"
          :active="route.path.startsWith(item.to)"
          rounded="lg"
          class="mb-1"
        />
      </v-list>
      <template #append>
        <div class="pa-3 nest-caption text-medium-emphasis">
          <div>WuNest <span class="nest-mono">v0.1</span></div>
          <div>{{ t('nav.byWusphere') }}</div>
        </div>
      </template>
    </v-navigation-drawer>

    <!-- Content -->
    <v-main>
      <router-view v-slot="{ Component }">
        <transition name="nest-fade" mode="out-in">
          <component :is="Component" />
        </transition>
      </router-view>
    </v-main>
  </v-layout>
</template>

<style lang="scss" scoped>
.nest-shell {
  min-height: 100vh;
  background: var(--nest-bg);
}

.nest-topbar {
  background: var(--nest-bg) !important;
  border-bottom: 1px solid var(--nest-border);
}

.nest-logo-mark {
  width: 28px;
  height: 28px;
  display: grid;
  place-items: center;
  color: var(--nest-accent);
  font-size: 18px;
  font-weight: 600;
  border: 1px solid var(--nest-accent);
  border-radius: 6px;
  font-family: var(--nest-font-mono);
}

.nest-logo-text {
  font-family: var(--nest-font-display);
  font-size: 18px;
  letter-spacing: -0.01em;
  color: var(--nest-text);
}

.nest-sidebar {
  background: var(--nest-bg-elevated) !important;
  border-right: 1px solid var(--nest-border) !important;
}

.cursor-pointer {
  cursor: pointer;
}

.nest-caption {
  font-size: 11px;
  line-height: 1.4;
  font-family: var(--nest-font-mono);
  letter-spacing: 0.05em;
}

.nest-fade-enter-active,
.nest-fade-leave-active {
  transition: opacity var(--nest-transition-base), transform var(--nest-transition-base);
}
.nest-fade-enter-from,
.nest-fade-leave-to {
  opacity: 0;
  transform: translateY(4px);
}
</style>
