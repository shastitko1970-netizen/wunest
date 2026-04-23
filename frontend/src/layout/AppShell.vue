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
const { profile, authenticated } = storeToRefs(auth)
const { profile: accountProfile } = storeToRefs(account)
const { t, locale, availableLocales } = useI18n()
const vTheme = useTheme()

// WuApi base URL. When the topbar "Sign in" button is clicked, we hit
// /auth/refresh with a return_to of the current URL so the user lands
// back where they were after login.
const WUAPI_BASE = 'https://api.wusphere.ru'
const loginUrl = computed(() => {
  const returnTo = encodeURIComponent(window.location.href)
  return `${WUAPI_BASE}/auth/refresh?return_to=${returnTo}`
})

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
// profile (and it's what Account.vue already refreshes). Only fire for
// authed users — the API call would 401 for anons and isn't useful.
onMounted(() => {
  if (authenticated.value && !accountProfile.value) void account.fetchProfile()
})

// Routes reachable without a session OR without an activated access
// code. `/account` is on the list so users who haven't redeemed a
// code can still reach the form to enter one.
const ALWAYS_ACCESSIBLE = ['/', '/docs', '/account']
const isGatedRoute = computed(() =>
  !ALWAYS_ACCESSIBLE.some(p => route.path === p || route.path.startsWith(p + '/')),
)
const showAuthPrompt = computed(() => !authenticated.value && isGatedRoute.value)
// Authed but hasn't redeemed an access code yet — show the beta gate
// prompt on any protected route. Public routes + Account are exempt.
const nestAccessGranted = computed(() => profile.value?.nest_access_granted === true)
const showAccessPrompt = computed(() =>
  authenticated.value && !nestAccessGranted.value && isGatedRoute.value,
)

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
  { to: '/docs', icon: 'mdi-book-open-variant', label: t('nav.docs') },
  { to: '/account', icon: 'mdi-account-circle-outline', label: t('nav.account') },
  { to: '/settings', icon: 'mdi-cog-outline', label: t('nav.settings') },
  // /studio dropped — it was a permanently-disabled stub that only added
  // noise to the mobile drawer. If we ship the debug panel (regex tester,
  // raw prompts, logs) we'll add a real menu entry then.
])

// Subset of navItems shown directly in the topbar on desktop. Presets used
// to be here as a separate destination; folded into /library as a tab so
// the topbar stays lean (Chat + Library + Docs cover everything users
// need from the topbar; Account + Settings live in the avatar menu).
const topbarNav = computed<NavItem[]>(() =>
  navItems.value.filter(i => ['/chat', '/library', '/docs'].includes(i.to)),
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
    <!-- Top bar. ST-compat: `#top-bar` + `.topbar` so user CSS targeting
         SillyTavern's top bar selector also styles ours. -->
    <v-app-bar
      id="top-bar"
      flat
      :elevation="0"
      class="nest-topbar topbar px-2"
      height="56"
    >
      <!-- Mobile burger: opens the overlay drawer. -->
      <v-app-bar-nav-icon
        v-if="!isDesktop"
        variant="text"
        @click="drawerOpen = !drawerOpen"
      />

      <!-- Logo → home (landing). Consistent for authed and anon so the
           page a logo click lands on is never surprising. -->
      <div class="d-flex align-center ga-2 cursor-pointer" @click="router.push('/')">
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

      <!-- Avatar / account menu — only for authed users. -->
      <v-menu v-if="authenticated && displayProfile" location="bottom end" offset="4">
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

      <!-- Sign In CTA — replaces the avatar menu for anonymous visitors.
           Keeps the topbar visually balanced and gives anons a constant
           way to log in from wherever they are. returnTo = current URL
           so they land back on the same page after login. -->
      <a
        v-else
        :href="loginUrl"
        class="nest-topbar-login"
      >
        <v-icon size="16" class="mr-1">mdi-login</v-icon>
        {{ t('welcome.ctaLogin') }}
      </a>
    </v-app-bar>

    <!-- Mobile-only overlay drawer. Not rendered on desktop at all so there's
         nothing to "get stuck hidden" in that layout. ST-compat id
         `#leftNavPanel` for theme authors. -->
    <v-navigation-drawer
      v-if="!isDesktop"
      id="leftNavPanel"
      v-model="drawerOpen"
      temporary
      width="260"
      class="nest-sidebar drawer-content"
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

    <!-- Content. ST-compat: `#sheld` is the main content root in
         SillyTavern; users sometimes target it to style the reading
         surface overall. -->
    <v-main id="sheld">
      <!-- Anons landing on a protected route see an inline sign-in
           prompt INSIDE the shell, not a standalone page — so they can
           still navigate to /, /docs, and see the topbar. -->
      <div v-if="showAuthPrompt" class="nest-auth-prompt">
        <v-icon size="48" color="surface-variant">mdi-lock-outline</v-icon>
        <h2 class="nest-h2 mt-4">{{ t('authPrompt.title') }}</h2>
        <p class="nest-subtitle mt-2">{{ t('authPrompt.body') }}</p>
        <div class="nest-auth-prompt-ctas mt-4">
          <a :href="loginUrl" class="nest-auth-prompt-cta-primary">
            <v-icon size="18" class="mr-2">mdi-login</v-icon>
            {{ t('welcome.ctaLogin') }}
          </a>
          <button class="nest-auth-prompt-cta-secondary" @click="router.push('/')">
            <v-icon size="18" class="mr-2">mdi-arrow-left</v-icon>
            {{ t('authPrompt.backHome') }}
          </button>
        </div>
      </div>
      <!-- Authed but hasn't redeemed a beta access code yet. Same inline
           pattern as the anon prompt but CTA sends them to Account where
           the code form lives, not to login. -->
      <div v-else-if="showAccessPrompt" class="nest-auth-prompt">
        <v-icon size="48" color="surface-variant">mdi-ticket-confirmation-outline</v-icon>
        <h2 class="nest-h2 mt-4">{{ t('accessGate.title') }}</h2>
        <p class="nest-subtitle mt-2">{{ t('accessGate.body') }}</p>
        <div class="nest-auth-prompt-ctas mt-4">
          <button class="nest-auth-prompt-cta-primary" @click="router.push('/account')">
            <v-icon size="18" class="mr-2">mdi-key-variant</v-icon>
            {{ t('accessGate.goToAccount') }}
          </button>
          <button class="nest-auth-prompt-cta-secondary" @click="router.push('/docs')">
            <v-icon size="18" class="mr-2">mdi-book-open-variant</v-icon>
            {{ t('accessGate.readDocs') }}
          </button>
        </div>
      </div>
      <router-view v-else v-slot="{ Component }">
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

// Topbar Sign In — takes the avatar menu's slot for anon users. Same
// right-edge rhythm as the avatar so the topbar doesn't shift when
// the user logs in.
.nest-topbar-login {
  display: inline-flex;
  align-items: center;
  padding: 6px 14px;
  font-size: 13px;
  font-weight: 500;
  color: #fff;
  background: var(--nest-accent);
  border-radius: var(--nest-radius-sm);
  text-decoration: none;
  transition: filter var(--nest-transition-fast);
  &:hover { filter: brightness(1.1); }
  .v-icon { color: #fff; }
}

// Inline "sign in to continue" card — shown instead of a protected
// route's content when the visitor is anonymous. Looks like /chat's
// empty-state hero so users recognise it as an "I need to do something
// to unblock" screen, not an error.
.nest-auth-prompt {
  min-height: calc(100vh - var(--nest-header-height) - 40px);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px 24px;
  text-align: center;
  gap: 4px;
}
.nest-auth-prompt-ctas {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  justify-content: center;
}
.nest-auth-prompt-cta-primary,
.nest-auth-prompt-cta-secondary {
  all: unset;
  display: inline-flex;
  align-items: center;
  padding: 12px 22px;
  font-size: 14px;
  font-weight: 500;
  letter-spacing: 0.01em;
  border-radius: var(--nest-radius);
  cursor: pointer;
  transition: transform var(--nest-transition-fast), filter var(--nest-transition-fast);
  &:hover { transform: translateY(-1px); }
}
.nest-auth-prompt-cta-primary {
  background: var(--nest-accent);
  color: #fff;
  text-decoration: none;
  &:hover { filter: brightness(1.1); }
  .v-icon { color: #fff; }
}
.nest-auth-prompt-cta-secondary {
  border: 1px solid var(--nest-border);
  color: var(--nest-text);
  &:hover { border-color: var(--nest-accent); }
}
</style>
