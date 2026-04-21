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
      account: 'Аккаунт',
      settings: 'Настройки',
      studio: 'Studio',
      manageAccount: 'Аккаунт в WuApi',
      byWusphere: 'часть wusphere.ru',
    },
    login: {
      headline: 'WuNest',
      tagline: 'Современный клиент для ролевой переписки с моделями. Ключи и подписка подтянутся из твоего WuApi-аккаунта.',
      cta: 'Войти через WuApi',
      signInWith: 'Войти через {provider}',
      alreadyLogged: 'Уже залогинен на WuApi?',
      continueSession: 'Продолжить',
      or: 'или войти заново',
      noAccountHint: 'Нет аккаунта? Зарегистрируйся на wusphere.ru — это бесплатно.',
    },
    settings: {
      title: 'Настройки',
      theme: 'Тема',
      language: 'Язык',
      byok: {
        title: 'BYOK (свои ключи провайдеров)',
        tagline: 'Используй свои собственные API-ключи вместо тех, что даёт WuApi. Полезно если у тебя уже оплачен OpenAI / Claude.',
        coming: 'Управление BYOK появится в следующей итерации.',
      },
    },
    theme: {
      dark: 'Тёмная',
      light: 'Светлая',
      toggle: 'Переключить тему',
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
      account: 'Account',
      settings: 'Settings',
      studio: 'Studio',
      manageAccount: 'WuApi account',
      byWusphere: 'part of wusphere.ru',
    },
    login: {
      headline: 'WuNest',
      tagline: 'A modern client for roleplay with LLMs. Keys and subscription come from your WuApi account.',
      cta: 'Sign in with WuApi',
      signInWith: 'Sign in with {provider}',
      alreadyLogged: 'Already signed in on WuApi?',
      continueSession: 'Continue',
      or: 'or sign in fresh',
      noAccountHint: "Don't have an account? Sign up at wusphere.ru — it's free.",
    },
    settings: {
      title: 'Settings',
      theme: 'Theme',
      language: 'Language',
      byok: {
        title: 'BYOK (bring your own keys)',
        tagline: 'Use your own provider API keys instead of WuApi. Useful if you already have an OpenAI or Claude plan.',
        coming: 'BYOK management is coming in a next iteration.',
      },
    },
    theme: {
      dark: 'Dark',
      light: 'Light',
      toggle: 'Toggle theme',
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
