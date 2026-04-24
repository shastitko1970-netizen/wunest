import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { createVuetify } from 'vuetify'
import { aliases, mdi } from 'vuetify/iconsets/mdi'
import { i18n } from '@/plugins/i18n'
import { router } from '@/router'
import { theme } from '@/plugins/vuetify'
import App from '@/App.vue'
import { useThemeStore } from '@/stores/theme'

import '@mdi/font/css/materialdesignicons.css'
import 'vuetify/styles'
import '@/styles/global.scss'

// Safe mode: `?safe=1` in the URL bypasses user CSS and custom theme
// loading entirely. Emergency escape hatch for users whose theme has
// hidden every clickable element of the app shell. Must run BEFORE any
// theme store activity so the `isSafeMode()` guard takes effect.
if (typeof window !== 'undefined') {
  const params = new URLSearchParams(window.location.search)
  if (params.get('safe') === '1') {
    ;(window as unknown as { __NEST_SAFE_MODE__?: boolean }).__NEST_SAFE_MODE__ = true
    // eslint-disable-next-line no-console
    console.warn('[nest] Safe mode — user theme & CSS skipped.')
  }
}

// Restore persisted Vuetify base theme (nestDark / nestLight), if any.
// This is separate from the M42.1 theme store (5 preset files) —
// Vuetify's theme controls the semantic palette class at a base level,
// while the theme store layers a preset CSS file on top.
const savedTheme = localStorage.getItem('nest:theme')
if (savedTheme === 'nestDark' || savedTheme === 'nestLight') {
  theme.defaultTheme = savedTheme
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
