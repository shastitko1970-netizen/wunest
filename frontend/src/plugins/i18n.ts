import { createI18n } from 'vue-i18n'

const messages = {
  ru: {
    app: {
      title: 'WuNest',
      loading: 'Загрузка…',
    },
    nav: {
      chat: 'Чат',
      library: 'Библиотека',
      settings: 'Настройки',
      studio: 'Studio',
      manageAccount: 'Аккаунт на WuSphere',
      byWusphere: 'by WuSphere',
    },
    login: {
      headline: 'WuNest',
      tagline: 'Современный клиент для ролевой переписки с моделями. Ключи и подписка подтянутся из WuSphere.',
      cta: 'Войти через WuSphere',
      noAccountHint: 'Нет аккаунта? Зарегистрируйся на wusphere.ru — это бесплатно.',
    },
  },
  en: {
    app: {
      title: 'WuNest',
      loading: 'Loading…',
    },
    nav: {
      chat: 'Chat',
      library: 'Library',
      settings: 'Settings',
      studio: 'Studio',
      manageAccount: 'Account on WuSphere',
      byWusphere: 'by WuSphere',
    },
    login: {
      headline: 'WuNest',
      tagline: 'A modern client for roleplay with LLMs. Keys and subscription come from your WuSphere account.',
      cta: 'Sign in via WuSphere',
      noAccountHint: "Don't have an account? Sign up at wusphere.ru — it's free.",
    },
  },
}

export const i18n = createI18n({
  legacy: false,
  locale: detectLocale(),
  fallbackLocale: 'en',
  messages,
})

function detectLocale(): string {
  const saved = localStorage.getItem('nest:locale')
  if (saved === 'ru' || saved === 'en') return saved
  return navigator.language?.startsWith('ru') ? 'ru' : 'en'
}
