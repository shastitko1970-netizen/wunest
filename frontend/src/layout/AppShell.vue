<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useTheme } from 'vuetify'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { useAuthStore } from '@/stores/auth'
import { useAccountStore } from '@/stores/account'
import { useI18n } from 'vue-i18n'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const account = useAccountStore()
const { profile } = storeToRefs(auth)
const { profile: accountProfile } = storeToRefs(account)
const { t, locale, availableLocales } = useI18n()
const vTheme = useTheme()

// Viewport detection via raw matchMedia. The previous v-navigation-drawer
// permanent-mode wasn't rendering reliably across setups, so on desktop we
// put nav directly in the topbar and drop the sidebar altogether. Mobile
// keeps the classic burger → overlay drawer; that path is battle-tested.
const isDesktop = ref(typeof window !== 'undefined'
  ? window.matchMedia('(min-width: 960px)').matches
  : true)
let mql: MediaQueryList | null = null
function handleMQ(e: MediaQueryListEvent) { isDesktop.value = e.matches }
onMounted(() => {
  if (typeof window === 'undefined') return
  mql = window.matchMedia('(min-width: 960px)')
  isDesktop.value = mql.matches
  mql.addEventListener('change', handleMQ)
})
onBeforeUnmount(() => mql?.removeEventListener('change', handleMQ))

// Mobile overlay drawer state; hidden entirely on desktop.
const drawerOpen = ref(false)

// Topbar pulls from the account store because that view has the fullest
// profile (and it's what Account.vue already refreshes). Auth store's
// `profile` stays as the lightweight version used by the login gate.
onMounted(() => {
  if (!accountProfile.value) void account.fetchProfile()
})

const displayProfile = computed(() => accountProfile.value ?? profile.value)

interface NavItem {
  to: string
  icon: string
  label: string
  disabled?: boolean
}

const navItems = computed<NavItem[]>(() => [
  { to: '/chat', icon: 'mdi-forum-outline', label: t('nav.chat') },
  { to: '/library', icon: 'mdi-bookshelf', label: t('nav.library') },
  { to: '/presets', icon: 'mdi-tune-variant', label: t('nav.presets') },
  { to: '/account', icon: 'mdi-account-circle-outline', label: t('nav.account') },
  { to: '/settings', icon: 'mdi-cog-outline', label: t('nav.settings') },
  { to: '/studio', icon: 'mdi-wrench-outline', label: t('nav.studio'), disabled: true },
])

// Subset of navItems shown directly in the topbar on desktop. We keep
// Account/Settings out of this strip because they live under the avatar
// menu — fewer items = more room for the core three (Chat/Library/Presets).
const topbarNav = computed<NavItem[]>(() =>
  navItems.value.filter(i => ['/chat', '/library', '/presets'].includes(i.to)),
)

const goldDisplay = computed(() =>
  displayProfile.value
    ? ((displayProfile.value as any).gold_balance_nano / 1_000_000_000).toFixed(2)
    : '—',
)

function isNavActive(to: string): boolean {
  if (to === '/chat') return route.path === '/chat' || route.path.startsWith('/chat/')
  return route.path === to || route.path.startsWith(to + '/')
}

// ─── Theme toggle ───────────────────────────────────────────
const isDark = computed(() => vTheme.global.name.value === 'nestDark')

function toggleTheme() {
  const next = isDark.value ? 'nestLight' : 'nestDark'
  vTheme.global.name.value = next
  localStorage.setItem('nest:theme', next)
}

// ─── Locale picker ──────────────────────────────────────────
function setLocale(code: string) {
  locale.value = code
  localStorage.setItem('nest:locale', code)
}

const localeLabel = (code: string) => {
  switch (code) {
    case 'ru': return 'Русский'
    case 'en': return 'English'
    default:   return code.toUpperCase()
  }
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
      <!-- Mobile burger: opens the overlay drawer. -->
      <v-app-bar-nav-icon
        v-if="!isDesktop"
        variant="text"
        @click="drawerOpen = !drawerOpen"
      />

      <!-- Logo → /chat -->
      <div class="d-flex align-center ga-2 cursor-pointer" @click="router.push('/chat')">
        <div class="nest-logo-mark">▲</div>
        <div class="nest-logo-text">WuNest</div>
      </div>

      <!-- Desktop nav strip — between logo and right-side controls. -->
      <nav v-if="isDesktop" class="nest-topnav">
        <button
          v-for="item in topbarNav"
          :key="item.to"
          class="nest-topnav-item"
          :class="{ active: isNavActive(item.to) }"
          @click="router.push(item.to)"
        >
          <v-icon size="18" class="mr-1">{{ item.icon }}</v-icon>
          {{ item.label }}
        </button>
      </nav>

      <v-spacer />

      <!-- Live quota chips (desktop only) -->
      <template v-if="displayProfile && isDesktop">
        <v-chip
          size="small"
          variant="tonal"
          color="secondary"
          class="mr-2 nest-mono"
          @click="router.push('/account')"
        >
          {{ goldDisplay }} gold
        </v-chip>
        <v-chip
          size="small"
          variant="tonal"
          class="mr-2 nest-mono"
          @click="router.push('/account')"
        >
          {{ (displayProfile as any).used_today }}/{{ (displayProfile as any).daily_limit || '∞' }}
        </v-chip>
      </template>

      <!-- Theme toggle. -->
      <v-btn
        :icon="isDark ? 'mdi-weather-sunny' : 'mdi-weather-night'"
        variant="text"
        size="small"
        :title="t('theme.toggle')"
        @click="toggleTheme"
      />

      <!-- Language picker. -->
      <v-menu location="bottom end" offset="4">
        <template #activator="{ props: menuProps }">
          <v-btn
            v-bind="menuProps"
            variant="text"
            size="small"
            class="nest-lang-btn"
          >
            <v-icon size="18" class="mr-1">mdi-translate</v-icon>
            {{ locale.toUpperCase() }}
          </v-btn>
        </template>
        <v-list density="compact">
          <v-list-item
            v-for="code in availableLocales"
            :key="code"
            :active="code === locale"
            @click="setLocale(code)"
          >
            <v-list-item-title>{{ localeLabel(code) }}</v-list-item-title>
          </v-list-item>
        </v-list>
      </v-menu>

      <!-- Avatar / account menu — holds Settings + Account + external link. -->
      <v-menu v-if="displayProfile" location="bottom end" offset="4">
        <template #activator="{ props: menuProps }">
          <v-btn icon v-bind="menuProps" size="small" variant="text">
            <v-avatar size="32" color="primary">
              <span class="text-caption">
                {{ ((displayProfile as any).first_name || (displayProfile as any).username || '?')[0] }}
              </span>
            </v-avatar>
          </v-btn>
        </template>
        <v-list density="compact" min-width="220">
          <v-list-item
            :title="(displayProfile as any).first_name || (displayProfile as any).username"
            :subtitle="(displayProfile as any).tier"
          />
          <v-divider />
          <v-list-item
            :title="t('nav.account')"
            prepend-icon="mdi-account-circle-outline"
            @click="router.push('/account')"
          />
          <v-list-item
            :title="t('nav.settings')"
            prepend-icon="mdi-cog-outline"
            @click="router.push('/settings')"
          />
          <v-list-item
            :title="t('nav.manageAccount')"
            prepend-icon="mdi-open-in-new"
            href="https://wusphere.ru/dashboard"
            target="_blank"
          />
        </v-list>
      </v-menu>
    </v-app-bar>

    <!-- Mobile-only overlay drawer. Not rendered on desktop at all so there's
         nothing to "get stuck hidden" in that layout. -->
    <v-navigation-drawer
      v-if="!isDesktop"
      v-model="drawerOpen"
      temporary
      width="260"
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
          :active="isNavActive(item.to)"
          rounded="lg"
          class="mb-1"
          @click="drawerOpen = false"
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

// Desktop topbar navigation — three primary destinations as pill buttons
// aligned horizontally after the logo. Lives in the app-bar itself, so
// there's no dependence on v-navigation-drawer for desktop layouts.
.nest-topnav {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-left: 24px;
  height: 100%;
}
.nest-topnav-item {
  all: unset;
  display: inline-flex;
  align-items: center;
  padding: 6px 12px;
  border-radius: var(--nest-radius-sm);
  font-family: var(--nest-font-body);
  font-size: 13.5px;
  color: var(--nest-text-secondary);
  cursor: pointer;
  transition: background var(--nest-transition-fast), color var(--nest-transition-fast);

  &:hover {
    background: var(--nest-bg-elevated);
    color: var(--nest-text);
  }
  &.active {
    color: var(--nest-text);
    background: var(--nest-bg-elevated);
    box-shadow: inset 0 -2px 0 var(--nest-accent);
  }
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

.nest-lang-btn {
  letter-spacing: 0.05em;
  font-family: var(--nest-font-mono);
  font-size: 12px;
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

// Hide the in-bar nav earlier than 960px, since chips take a lot of space
// between 960-1100. They collapse into the avatar menu item on mobile.
@media (max-width: 1100px) {
  .nest-topnav-item {
    padding: 6px 8px;
    font-size: 12.5px;
  }
}
</style>
