// Docs index — single source of truth for the /docs table of contents.
// Adding a new doc = add markdown file under src/docs/pages/ and append
// an entry to TOPICS. The Docs view renders ToC + content from this list.
//
// Markdown files are imported as raw strings via Vite's `?raw` syntax
// so Vue type-checks stay happy and production bundles the content
// directly (no runtime fetch).

// ─── Raw markdown imports ──────────────────────────────────────────
// Keep this block synced with TOPICS below. Each ?raw import becomes a
// plain string at build time.

import gettingStartedRu from './pages/getting-started.ru.md?raw'
import gettingStartedEn from './pages/getting-started.en.md?raw'
import charactersRu from './pages/characters.ru.md?raw'
import charactersEn from './pages/characters.en.md?raw'
import presetsRu from './pages/presets.ru.md?raw'
import presetsEn from './pages/presets.en.md?raw'
import lorebooksRu from './pages/lorebooks.ru.md?raw'
import lorebooksEn from './pages/lorebooks.en.md?raw'
import byokRu from './pages/byok.ru.md?raw'
import byokEn from './pages/byok.en.md?raw'
import themingRu from './pages/theming.ru.md?raw'
import themingEn from './pages/theming.en.md?raw'
import safeModeRu from './pages/safe-mode.ru.md?raw'
import safeModeEn from './pages/safe-mode.en.md?raw'
import mobileRu from './pages/mobile.ru.md?raw'
import mobileEn from './pages/mobile.en.md?raw'

// ─── TOC ───────────────────────────────────────────────────────────

/** A doc topic. Copy per locale is stored inline so the index can
 *  render the table of contents without a separate i18n file. */
export interface DocTopic {
  slug: string
  category: 'start' | 'content' | 'generation' | 'customization'
  title: { ru: string; en: string }
  summary: { ru: string; en: string }
  content: { ru: string; en: string }
}

export const TOPICS: DocTopic[] = [
  {
    slug: 'getting-started',
    category: 'start',
    title:   { ru: 'С чего начать',             en: 'Getting started' },
    summary: { ru: 'Первый чат за 5 минут.',   en: 'Your first chat in 5 minutes.' },
    content: { ru: gettingStartedRu, en: gettingStartedEn },
  },
  {
    slug: 'characters',
    category: 'content',
    title:   { ru: 'Персонажи',                           en: 'Characters' },
    summary: { ru: 'PNG/JSON-импорт, CHUB, создание с нуля.', en: 'PNG/JSON import, CHUB, creating from scratch.' },
    content: { ru: charactersRu, en: charactersEn },
  },
  {
    slug: 'lorebooks',
    category: 'content',
    title:   { ru: 'Лорбуки',                            en: 'Lorebooks' },
    summary: { ru: 'World info, записи, ключи, группы.', en: 'World info, entries, keys, groups.' },
    content: { ru: lorebooksRu, en: lorebooksEn },
  },
  {
    slug: 'presets',
    category: 'generation',
    title:   { ru: 'Пресеты',                                en: 'Presets' },
    summary: { ru: 'Сэмплеры, инструкт, контекст, sysprompt.', en: 'Samplers, instruct, context, sysprompt.' },
    content: { ru: presetsRu, en: presetsEn },
  },
  {
    slug: 'byok',
    category: 'generation',
    title:   { ru: 'Свои ключи (BYOK)',            en: 'Your own keys (BYOK)' },
    summary: { ru: 'OpenAI, OpenRouter, свои URL.', en: 'OpenAI, OpenRouter, custom URLs.' },
    content: { ru: byokRu, en: byokEn },
  },
  {
    slug: 'theming',
    category: 'customization',
    title:   { ru: 'Оформление и CSS',                          en: 'Theming & CSS' },
    summary: { ru: 'Импорт тем ST, свой CSS, scope, переменные.', en: 'ST theme import, custom CSS, scope, variables.' },
    content: { ru: themingRu, en: themingEn },
  },
  {
    slug: 'safe-mode',
    category: 'customization',
    title:   { ru: 'Безопасный режим',                      en: 'Safe mode' },
    summary: { ru: 'Если CSS сломал интерфейс — как вернуться.', en: 'If CSS broke the UI — how to recover.' },
    content: { ru: safeModeRu, en: safeModeEn },
  },
  {
    slug: 'mobile',
    category: 'start',
    title:   { ru: 'На мобильном',                      en: 'On mobile' },
    summary: { ru: 'Навигация, свайпы, особенности верстки.', en: 'Navigation, swipes, layout quirks.' },
    content: { ru: mobileRu, en: mobileEn },
  },
]

export function findTopic(slug: string): DocTopic | null {
  return TOPICS.find(t => t.slug === slug) ?? null
}

export const CATEGORY_ORDER: Array<DocTopic['category']> = [
  'start', 'content', 'generation', 'customization',
]

export const CATEGORY_LABEL: Record<DocTopic['category'], { ru: string; en: string }> = {
  start:         { ru: 'Начало',         en: 'Start here' },
  content:       { ru: 'Контент',        en: 'Content' },
  generation:    { ru: 'Генерация',      en: 'Generation' },
  customization: { ru: 'Оформление',     en: 'Customization' },
}
