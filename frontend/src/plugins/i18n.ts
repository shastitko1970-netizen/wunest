import { createI18n } from 'vue-i18n'

const messages = {
  ru: {
    app: {
      title: 'WuNest',
      loading: 'Загрузка…',
    },
    home: {
      greeting: 'Привет, {name}!',
      anonymous: 'Добро пожаловать в WuNest',
      loginCta: 'Войти через WuSphere',
      tierLabel: 'Тариф',
      goldLabel: 'Золото',
      quotaLabel: 'Сегодня',
    },
  },
  en: {
    app: {
      title: 'WuNest',
      loading: 'Loading…',
    },
    home: {
      greeting: 'Hello, {name}!',
      anonymous: 'Welcome to WuNest',
      loginCta: 'Sign in via WuSphere',
      tierLabel: 'Tier',
      goldLabel: 'Gold',
      quotaLabel: 'Today',
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
