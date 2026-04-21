import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { createVuetify } from 'vuetify'
import { i18n } from '@/plugins/i18n'
import { router } from '@/router'
import { theme } from '@/plugins/vuetify'
import App from '@/App.vue'

import '@mdi/font/css/materialdesignicons.css'
import 'vuetify/styles'
import '@/styles/global.scss'

const vuetify = createVuetify({
  theme,
  defaults: {
    VBtn: { variant: 'flat', rounded: 'lg' },
    VCard: { rounded: 'lg' },
    VTextField: { variant: 'outlined', density: 'comfortable', rounded: 'lg', color: 'primary' },
    VTextarea: { variant: 'outlined', density: 'comfortable', rounded: 'lg', color: 'primary' },
    VSelect: { variant: 'outlined', density: 'comfortable', rounded: 'lg' },
  },
})

createApp(App)
  .use(createPinia())
  .use(router)
  .use(vuetify)
  .use(i18n)
  .mount('#app')
