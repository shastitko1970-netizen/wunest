<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useDisplay, useTheme } from 'vuetify'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { useAuthStore } from '@/stores/auth'
import { useAccountStore } from '@/stores/account'
import { useI18n } from 'vue-i18n'

const { mdAndUp } = useDisplay()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const account = useAccountStore()
const { profile } = storeToRefs(auth)
const { profile: accountProfile } = storeToRefs(account)
const { t, locale, availableLocales } = useI18n()
const vTheme = useTheme()

// On desktop the drawer is persistent; on mobile it's an overlay.
const drawerOpen = ref(false)

// Topbar pulls from the account store because that view has the fullest
// profile (and it's what Account.vue already refreshes). Auth store's
// `profile` stays as the lightweight version used by the login gate.
onMounted(() => {
  if (!accountProfile.value) void account.fetchProfile()
})

const displayProfile = computed(() => accountProfile.value ?? profile.value)

const navItems = computed(() => [
  { to: '/chat', icon: 'mdi-forum-outline', label: t('nav.chat') },
  { to: '/library', icon: 'mdi-bookshelf', label: t('nav.library') },
  { to: '/presets', icon: 'mdi-tune-variant', label: t('nav.presets') },
  { to: '/account', icon: 'mdi-account-circle-outline', label: t('nav.account') },
  { to: '/settings', icon: 'mdi-cog-outline', label: t('nav.settings') },
  { to: '/studio', icon: 'mdi-wrench-outline', label: t('nav.studio'), disabled: true },
])

const goldDisplay = computed(() =>
  displayProfile.value
    ? ((displayProfile.value as any).gold_balance_nano / 1_000_000_000).toFixed(2)
    : '—',
)

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

      <!-- Live quota chips (desktop only) -->
      <template v-if="displayProfile && mdAndUp">
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

      <!-- Theme toggle: sun/moon flip depending on current mode. -->
      <v-btn
        :icon="isDark ? 'mdi-weather-sunny' : 'mdi-weather-night'"
        variant="text"
        size="small"
        :title="t('theme.toggle')"
        @click="toggleTheme"
      />

      <!-- Language picker: shows the current locale as a label, opens a
           menu of supported locales. Much clearer than a plain icon. -->
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

      <!-- Avatar / account menu -->
      <v-menu v-if="displayProfile">
        <template #activator="{ props: menuProps }">
          <v-btn icon v-bind="menuProps" size="small" variant="text">
            <v-avatar size="32" color="primary">
              <span class="text-caption">
                {{ ((displayProfile as any).first_name || (displayProfile as any).username || '?')[0] }}
              </span>
            </v-avatar>
          </v-btn>
        </template>
        <v-list density="compact">
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
</style>
