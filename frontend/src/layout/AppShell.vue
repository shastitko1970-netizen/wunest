<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useTheme } from 'vuetify'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { useAuthStore } from '@/stores/auth'
import { useAccountStore } from '@/stores/account'
import { useThemeStore } from '@/stores/theme'
import { useI18n } from 'vue-i18n'
import ChatSearchDialog from '@/components/ChatSearchDialog.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const account = useAccountStore()
const { profile, authenticated } = storeToRefs(auth)
const { profile: accountProfile } = storeToRefs(account)
const { t, locale, availableLocales } = useI18n()
const vTheme = useTheme()

// Sign-in goes through WuNest's own /auth/start so we get a server-side
// log entry for every login attempt (UA, IP, had-existing-session). That
// endpoint then 302s to WuApi's /auth/refresh with the same return_to.
// One extra redirect costs nothing and means "I can't sign in on mobile"
// reports are diagnosable from logs.
const loginUrl = computed(() => {
  const returnTo = encodeURIComponent(window.location.href)
  return `/auth/start?return_to=${returnTo}`
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

// Global Ctrl/⌘+K opens chat search from anywhere in the app. Gated
// to authenticated users — unauth would see empty results anyway.
// IME composition events (Chinese/Japanese input) are skipped via
// `!e.isComposing` so Cmd+K in a CJK compose buffer doesn't hijack.
const searchOpen = ref(false)
function onGlobalKeydown(e: KeyboardEvent) {
  if (e.isComposing || !authenticated.value) return
  if ((e.metaKey || e.ctrlKey) && (e.key === 'k' || e.key === 'K')) {
    e.preventDefault()
    searchOpen.value = true
  }
}
onMounted(() => {
  if (typeof window !== 'undefined') {
    window.addEventListener('keydown', onGlobalKeydown)
  }
})
onBeforeUnmount(() => {
  if (typeof window !== 'undefined') {
    window.removeEventListener('keydown', onGlobalKeydown)
  }
})

// Topbar pulls from the account store because that view has the fullest
// profile (and it's what Account.vue already refreshes). Only fire for
// authed users — the API call would 401 for anons and isn't useful.
onMounted(() => {
  if (authenticated.value && !accountProfile.value) void account.fetchProfile()
})

// Anon gate — no session AND route needs one. Routes open without a
// session: / and /docs*.
const PUBLIC_PATHS = ['/', '/docs']
const isProtectedRoute = computed(() =>
  !PUBLIC_PATHS.some(p => route.path === p || route.path.startsWith(p + '/')),
)
const showAuthPrompt = computed(() => !authenticated.value && isProtectedRoute.value)

// Beta-access banner — authed user without an activated code. This
// does NOT block the UI: the user can browse everywhere just like
// before. Two thin lines at the top say "system locked until you
// enter a code" with a link to Account. Actual generation endpoints
// will hard-fail server-side once we wire per-request gating — the
// banner is purely the "why doesn't anything work?" signpost.
const nestAccessGranted = computed(() => profile.value?.nest_access_granted === true)
const showAccessBanner = computed(() =>
  authenticated.value && !nestAccessGranted.value,
)

const displayProfile = computed(() => accountProfile.value ?? profile.value)

interface NavItem {
  to: string
  icon: string
  label: string
  disabled?: boolean
}

// Chat and Library both require an activated beta code — the server
// would reject any generation / library write anyway. Disable them
// visibly in the nav so users understand why clicks do nothing, and
// pass them through to the access banner's CTA (Account) for redemption.
const gatedDisabled = computed(() => authenticated.value && !nestAccessGranted.value)

const navItems = computed<NavItem[]>(() => [
  { to: '/chat',     icon: 'mdi-forum-outline',        label: t('nav.chat'),     disabled: gatedDisabled.value },
  { to: '/library',  icon: 'mdi-bookshelf',            label: t('nav.library'),  disabled: gatedDisabled.value },
  { to: '/docs',     icon: 'mdi-book-open-variant',    label: t('nav.docs') },
  { to: '/account',  icon: 'mdi-account-circle-outline', label: t('nav.account') },
  { to: '/settings', icon: 'mdi-cog-outline',          label: t('nav.settings') },
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

// ─── Theme store integration ────────────────────────────────
// The sun/moon toggle used to live in the topbar but tester asked
// to keep the topbar purely for navigation + context. Theme mode
// toggle moved to Settings; the picker for all 5 presets stays in
// AppearancePanel. The watcher below keeps Vuetify's palette class
// synced to whichever preset is currently active.
const themeStore = useThemeStore()
const { current: currentPreset } = storeToRefs(themeStore)

// Keep Vuetify theme name mirrored to the preset's kind.
//
// Background: two palette systems run side-by-side.
//   - Our DS tokens live in :root and `.v-theme--nest{Dark,Light}` blocks
//     in global.scss. They set --nest-* via --SmartTheme* fallbacks.
//   - Vuetify builds its own --v-theme-* variables from `theme.name`
//     and feeds them into v-card / v-btn / etc.
//
// Before this watcher, picking a "Cyber neon" (dark) preset from the
// picker would swap the <style id="nest-theme"> CSS (our tokens) but
// leave Vuetify on whatever theme.name was last — so the background
// went cyber-dark, but v-card-colour shades stayed "light", producing
// hybrid-ugly bubbles. Syncing on every preset change closes that gap.
//
// Writes `nest:theme` back to localStorage so the bootstrap step in
// main.ts continues to resolve the right theme on next reload.
watch(() => currentPreset.value.kind, (kind) => {
  const wanted = kind === 'dark' ? 'nestDark' : 'nestLight'
  if (vTheme.global.name.value !== wanted) {
    vTheme.global.name.value = wanted
    localStorage.setItem('nest:theme', wanted)
  }
}, { immediate: true })

// ─── Locale picker ──────────────────────────────────────────
function setLocale(code: string) {
  locale.value = code
  localStorage.setItem('nest:locale', code)
}

// ─── Safe-mode entry ────────────────────────────────────────
// Reload the current URL with ?safe appended. The appearance store
// reads SAFE_MODE at boot → customCss + bg image get skipped, and
// SafeModeBanner surfaces with "Clear CSS" / "Exit" actions. This is
// the escape hatch for a user whose imported ST theme broke the shell
// and who doesn't know about the ?safe URL trick.
function enterSafeMode() {
  const url = new URL(window.location.href)
  url.searchParams.set('safe', '1')
  window.location.replace(url.toString())
}

// ─── Mobile drawer nav click ──────────────────────────────
// Single path for every drawer tap: close drawer, then route. For gated
// items (Chat / Library while the user hasn't redeemed a code) we send
// them to /account where the redeem form lives — so even a tap on a
// locked row gets them somewhere useful instead of doing nothing.
function onMobileNavClick(item: NavItem) {
  drawerOpen.value = false
  const to = item.disabled ? '/account' : item.to
  router.push(to)
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
         SillyTavern's top bar selector also styles ours.

         Фиксированный (viewport-pinned) — default Vuetify поведение
         через layoutItemStyles. Бар остаётся сверху экрана при скролле,
         контент v-main смещается вниз на 56px (auto-padding от layout
         system'ы), чтобы не прятаться под баром.

         Ранее пробовали `scroll-behavior="hide"` и `position: static`
         для «уезжает с контентом» — оба варианта тестер отклонил:
         «зафиксируй его уже». Возвращаемся к классическому sticky-top. -->
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
           page a logo click lands on is never surprising.
           Mark is token-driven: `--nest-logo-text` (glyph, default "▲")
           is rendered via ::before, `--nest-logo-image` (url, default
           none) layers on top as a background — author chooses text
           OR image without touching the template. Wordmark text sits
           in its own span so authors can retokenize or hide independently. -->
      <div class="d-flex align-center ga-2 cursor-pointer nest-logo" @click="router.push('/')">
        <div class="nest-logo-mark" aria-hidden="true" />
        <div class="nest-logo-text">WuNest</div>
      </div>

      <!-- Desktop nav strip — between logo and right-side controls.
           Gated items (Chat/Library for not-yet-activated users) are
           rendered in a disabled visual state + a title tooltip so the
           reason is obvious; clicks routed to /account instead so users
           land on the redeem form. -->
      <nav v-if="isDesktop" class="nest-topnav">
        <button
          v-for="item in topbarNav"
          :key="item.to"
          class="nest-topnav-item"
          :class="{ active: isNavActive(item.to), disabled: item.disabled }"
          :title="item.disabled ? t('accessBanner.body') : undefined"
          @click="item.disabled ? router.push('/account') : router.push(item.to)"
        >
          <v-icon size="18" class="mr-1">{{ item.icon }}</v-icon>
          {{ item.label }}
          <v-icon
            v-if="item.disabled"
            size="12"
            class="ml-1 nest-topnav-lock"
          >mdi-lock</v-icon>
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

      <!-- Theme toggle moved to Settings by tester request. Topbar is
           for navigation + context; theme swaps live with the rest of
           Appearance controls in /settings. The kind-sync watcher below
           still keeps Vuetify's palette class aligned on preset change. -->

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
          <v-divider />
          <!-- Safe-mode recovery. Always visible so users with a broken
               theme always have a one-click escape without needing to
               remember the ?safe URL. Reloads with ?safe → CSS gets
               skipped, SafeModeBanner appears with a Clear-CSS button. -->
          <v-list-item
            :title="t('nav.safeMode')"
            :subtitle="t('nav.safeModeSubtitle')"
            prepend-icon="mdi-shield-refresh-outline"
            @click="enterSafeMode()"
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
        <!-- Explicit click-handler navigation. We used to set `:to` when
             item.disabled was false, but Vuetify + router-link bindings
             plus a parallel @click handler raced in some Android WebView
             builds and landed on neither navigate-nor-close. Now every
             item goes through one path: close drawer, then router.push —
             disabled rows are redirected to /account so the access-code
             form is one tap away. -->
        <v-list-item
          v-for="item in navItems"
          :key="item.to"
          :prepend-icon="item.icon"
          :title="item.label"
          :active="isNavActive(item.to)"
          :class="['mb-1', { 'nest-nav-gated': item.disabled }]"
          rounded="lg"
          @click="onMobileNavClick(item)"
        >
          <template v-if="item.disabled" #append>
            <v-icon size="14" class="nest-nav-lock">mdi-lock</v-icon>
          </template>
        </v-list-item>
      </v-list>
      <template #append>
        <!-- Safe-mode + version caption wrapped in a single container —
             Vuetify's #append slot consumes one root, extra siblings can
             get dropped silently on some versions. -->
        <div>
          <v-list density="compact" class="px-2 pb-1">
            <v-list-item
              :title="t('nav.safeMode')"
              prepend-icon="mdi-shield-refresh-outline"
              rounded="lg"
              @click="drawerOpen = false; enterSafeMode()"
            />
          </v-list>
          <div class="pa-3 nest-caption text-medium-emphasis">
            <div>WuNest <span class="nest-mono">v0.1</span></div>
            <div>{{ t('nav.byWusphere') }}</div>
          </div>
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
      <!-- Thin beta-gate banner. Shown when the user is signed in but
           hasn't redeemed an access code yet. The app itself stays
           navigable — generation just fails server-side until activation
           because downstream endpoints gate on the flag. Keeps the
           experience closer to "as before, but with a note at the top". -->
      <div v-if="showAccessBanner" class="nest-access-banner">
        <v-icon size="16" class="mr-2">mdi-ticket-confirmation-outline</v-icon>
        <span class="nest-access-banner-text">{{ t('accessBanner.body') }}</span>
        <router-link to="/account" class="nest-access-banner-link">
          {{ t('accessBanner.cta') }} →
        </router-link>
      </div>
      <router-view v-slot="{ Component }">
        <transition name="nest-fade" mode="out-in">
          <component :is="Component" />
        </transition>
      </router-view>
    </v-main>

    <!-- Global chat search — Ctrl/⌘+K anywhere in the app. -->
    <ChatSearchDialog v-model="searchOpen" />
  </v-layout>
</template>

<style lang="scss" scoped>
.nest-shell {
  // 100dvh reflows on mobile browser URL-bar collapse and on IME
  // keyboard show/hide. 100vh stays pinned to "bar collapsed" height,
  // leaving a dead strip. DS rule: always 100dvh.
  min-height: 100dvh;
  background: var(--nest-bg);
}

.nest-topbar {
  background: var(--nest-bg) !important;
  border-bottom: 1px solid var(--nest-border);
  // Notch-aware padding on iOS PWA / mobile Safari fullscreen. Без этого
  // логотип частично прячется за notch на iPhone 14+ в landscape. Additive:
  // логическое + safe-area, base 2px inline padding сохраняется.
  padding-top:   env(safe-area-inset-top, 0);
  padding-left:  env(safe-area-inset-left, 0);
  padding-right: env(safe-area-inset-right, 0);
}

// Logo cluster (mark + wordmark). Left-padding away from the topbar
// edge so the mark doesn't touch the safe-area inset when the notch
// is hidden; tester feedback: "дай небольшой отступ от края у
// логотипа". Keeps things visually balanced against the avatar menu
// on the right which has its own natural gap.
.nest-logo {
  padding-left: 8px;
}

// Logo mark — token-driven so modders can swap glyph OR image without
// forking the template. `--nest-logo-text` is a CSS string (including
// quotes) rendered via ::before; `--nest-logo-image` is a CSS url()
// that layers on top as background — when both are set the image wins
// and the glyph stays as a fallback for screen-readers / RSS previews.
.nest-logo-mark {
  width:  var(--nest-logo-size, 28px);
  height: var(--nest-logo-size, 28px);
  display: grid;
  place-items: center;
  color: var(--nest-accent);
  font-weight: 600;
  font-size: calc(var(--nest-logo-size, 28px) * 0.65);
  border: 1px solid var(--nest-accent);
  border-radius: var(--nest-radius-sm);
  font-family: var(--nest-font-mono);
  background-image: var(--nest-logo-image, none);
  background-size: contain;
  background-repeat: no-repeat;
  background-position: center;

  &::before {
    content: var(--nest-logo-text, '▲');
  }
  // When the author supplies an image, hide the glyph so it doesn't
  // peek through. Author sets `--nest-logo-text: ""` to suppress the
  // glyph in image-less themes too.
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
  // Beta-gate disabled state — dimmed + lock icon; click still routes
  // so users don't wonder why nothing happens (it goes to /account
  // where they can redeem a code).
  &.disabled {
    color: var(--nest-text-muted);
    opacity: 0.55;
    cursor: help;
    &:hover {
      background: transparent;
      color: var(--nest-text-muted);
    }
  }
}
.nest-topnav-lock { opacity: 0.7; }

// Mobile drawer — gated nav rows are dimmed + get a trailing lock icon.
// Kept clickable (routes to /account) so the tap always lands somewhere
// useful instead of feeling unresponsive.
.nest-nav-gated {
  opacity: 0.6;

  :deep(.v-list-item-title) {
    color: var(--nest-text-muted);
  }
}
.nest-nav-lock {
  color: var(--nest-text-muted);
  opacity: 0.7;
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
// Sign-in CTA for anon visitors. Background is accent; foreground is
// fixed white via keyword (selector contract forbids hex in components,
// `white` is a CSS keyword — portable and never drifts with theme).
// Authors can override via `.nest-topbar-login { color: ... }` or
// `--nest-btn-primary-fg` if we introduce it later.
.nest-topbar-login {
  display: inline-flex;
  align-items: center;
  padding: 6px 14px;
  font-size: 13px;
  font-weight: 500;
  color: white;
  background: var(--nest-accent);
  border-radius: var(--nest-radius-sm);
  text-decoration: none;
  transition: filter var(--nest-transition-fast);
  &:hover { filter: brightness(1.1); }
  .v-icon { color: white; }
}

// Inline "sign in to continue" card — shown instead of a protected
// route's content when the visitor is anonymous. Looks like /chat's
// empty-state hero so users recognise it as an "I need to do something
// to unblock" screen, not an error.
.nest-auth-prompt {
  // Use 100dvh so the hero centres correctly after the mobile URL bar
  // retracts on scroll (100vh would pin to the collapsed height and
  // leave a ghost-gap at the bottom).
  min-height: calc(100dvh - var(--nest-header-height) - 40px);
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

// Beta-access banner — shown below the topbar for authed users who
// haven't redeemed a code yet. Thin, always visible, clickable to
// Account. Amber colour so it reads as "something to do" without
// looking like a critical error.
.nest-access-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  background: rgba(201, 136, 42, 0.12);
  border-bottom: 1px solid rgba(201, 136, 42, 0.4);
  color: var(--nest-text);
  font-size: 13px;
  flex-wrap: wrap;

  .v-icon { color: #c9882a; flex-shrink: 0; }
}
.nest-access-banner-text { flex: 1 1 auto; min-width: 0; }
.nest-access-banner-link {
  color: #c9882a;
  font-weight: 600;
  text-decoration: none;
  white-space: nowrap;
  &:hover { filter: brightness(1.2); }
}
</style>
