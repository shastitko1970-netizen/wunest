import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { createVuetify } from 'vuetify'
import { aliases, mdi } from 'vuetify/iconsets/mdi'
import { i18n } from '@/plugins/i18n'
import { router } from '@/router'
import { theme } from '@/plugins/vuetify'
import App from '@/App.vue'
import { useThemeStore } from '@/stores/theme'
import { useAppearanceStore } from '@/stores/appearance'

import '@mdi/font/css/materialdesignicons.css'
import 'vuetify/styles'
import '@/styles/global.scss'

// Safe mode: `?safe=1` (or `?safe`, or `?nest-safe`) in the URL bypasses
// user CSS, the persisted preset CSS chunk, AND the persisted Vuetify
// base palette — so even a user who broke their UI by switching to
// nestLight + a hostile custom theme can recover by appending `?safe`.
// Must run BEFORE any theme store activity so the guard takes effect.
const SAFE_MODE = (() => {
  if (typeof window === 'undefined') return false
  try {
    const params = new URLSearchParams(window.location.search)
    return params.has('safe') || params.has('nest-safe')
  } catch { return false }
})()
if (SAFE_MODE) {
  ;(window as unknown as { __NEST_SAFE_MODE__?: boolean }).__NEST_SAFE_MODE__ = true
  // eslint-disable-next-line no-console
  console.warn('[nest] Safe mode — preset CSS, custom CSS and persisted Vuetify palette skipped.')
}

// M52.12 — global unhandled-rejection sink.
//
// MetaMask / Phantom / similar crypto-wallet browser extensions inject
// `lockdown-install.js` (the SES sandboxing primitive). SES installs
// its own unhandledrejection listener that logs `SES_UNCAUGHT_EXCEPTION:
// <reason>` whenever a Promise rejects without a catch — even when the
// rejection is benign (Vuetify transition aborts, navigation cancels,
// `null` rejects from third-party promises).
//
// We can't fix MetaMask. But we CAN claim the rejection ourselves:
// `event.preventDefault()` tells the browser the rejection is handled,
// which short-circuits SES's logger. We log to our own console.warn
// with the reason + stack so real bugs aren't silenced — they just
// look like our log line instead of the wallet extension's red error.
//
// Skipped in SSR (no window).
if (typeof window !== 'undefined') {
  window.addEventListener('unhandledrejection', (event) => {
    // eslint-disable-next-line no-console
    console.warn('[nest] unhandled rejection:', event.reason ?? '(null)', {
      promise: event.promise,
    })
    // Mark as handled — stops SES_UNCAUGHT_EXCEPTION noise from
    // wallet extensions. If you're debugging a real rejection that
    // matters, the warn line above carries the reason + stack.
    event.preventDefault()
  })
}

// Restore persisted Vuetify base theme (nestDark / nestLight), if any.
// This is separate from the M42.1 theme store (5 preset files) —
// Vuetify's theme controls the semantic palette class at a base level,
// while the theme store layers a preset CSS file on top.
//
// In SAFE_MODE we deliberately ignore the persisted choice and keep
// whatever the bundled `theme.defaultTheme` is (currently nestDark).
// Otherwise a user who flipped to nestLight + then broke the shell
// couldn't get back to a known-good dark state via `?safe`.
if (!SAFE_MODE) {
  const savedTheme = localStorage.getItem('nest:theme')
  if (savedTheme === 'nestDark' || savedTheme === 'nestLight') {
    theme.defaultTheme = savedTheme
  }
}

const vuetify = createVuetify({
  theme,
  icons: { defaultSet: 'mdi', aliases, sets: { mdi } },
  defaults: {
    VBtn: { variant: 'flat', rounded: 'lg' },
    VCard: { rounded: 'lg' },
    VTextField: { variant: 'outlined', density: 'comfortable', rounded: 'lg', color: 'primary' },
    VTextarea: { variant: 'outlined', density: 'comfortable', rounded: 'lg', color: 'primary' },
    VSelect: { variant: 'outlined', density: 'comfortable', rounded: 'lg' },
  },
})

const app = createApp(App)
const pinia = createPinia()

app
  .use(pinia)
  .use(router)
  .use(vuetify)
  .use(i18n)
  .mount('#app')

// Kick off the theme store AFTER Pinia is wired so the store can be
// resolved. Fire-and-forget — dynamic CSS chunk loads without blocking
// first paint, and the default (nestDark) is already applied via the
// inline :root block in global.scss.
useThemeStore(pinia).init()

// M51 Sprint 1 — pull the user's saved Appearance from the server on
// boot. Previously this only fired when the user opened Settings, so a
// login on a new device painted with empty localStorage Appearance and
// the user's saved accent / customCss / bg / etc. only appeared after
// they navigated to Settings → Appearance. Cross-device theme parity is
// now part of cold-load. Skipped in safe mode (we don't want the saved
// customCss from the server bypassing the recovery flow).
if (!SAFE_MODE) {
  // Fire-and-forget — applyAppearance runs synchronously when the store
  // mutates, so the page repaints as soon as the response lands. No
  // need to await before mount.
  void useAppearanceStore(pinia).fetchFromServer()
}
