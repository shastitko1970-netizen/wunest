# WuNest Design System

Контракт: как устроен интерфейс WuNest, что в нём стабильно, и как пользователи меняют его через CSS — не ломая шелл.

Это **не** рефакторинг фронта. Это набор правил, который делает существующий код предсказуемо кастомизируемым: ты знаешь, за какие селекторы можно цепляться, какие токены гарантированно существуют, и что случится с темой после апдейта.

---

## Продукт в одном абзаце

**WuNest** — веб-платформа ролевых AI-чатов, серверная переосмысленная версия SillyTavern. Vue 3 + Vuetify 4 + Pinia + vue-i18n. Ядро — экран чата; всё остальное (Library, Settings, Account, Docs) — вокруг него. Один и тот же фронт запускается в трёх контекстах: десктоп-браузер, мобильный браузер, мобильный webview внутри приложения.

### Поверхности

| Экран | Роут | Роль |
|---|---|---|
| Landing | `/` | Публичная витрина |
| **Chat** | `/chat`, `/chat/:id` | Главный экран |
| Library | `/library` | Персонажи, worlds, персоны, пресеты |
| Settings | `/settings` | Внешний вид, модели, язык |
| Account | `/account` | Баланс, подписка, BYOK-ключи |
| Docs | `/docs`, `/docs/:page` | Справка |

### Целевые устройства

1. **Desktop browser** (≥ 960 px) — нав-стрип в топбаре, hover-состояния, ⌘K/Ctrl+K.
2. **Mobile browser** (< 960 px) — overlay-drawer, actions-always-visible, `env(safe-area-inset-*)`, `100dvh`.
3. **Mobile app** (webview) — тот же код плюс блокировка pull-to-refresh в чате и адаптация к IME.

---

## Два читателя

Документация рассчитана на двоих, и каждому дорога начинается со своего TL;DR.

### Для разработчика

1. Цвет, размер, шрифт — **только через `var(--nest-*)`**. Никогда не пиши hex в компоненте.
2. Корень важного компонента получает якорь `.nest-<name>`. Главные блоки — ещё и ST-алиас (`#chat`, `.mes`, `#send_textarea`). Список — в `docs/SELECTOR_CONTRACT.md`.
3. Состояние — `data-nest-*` атрибут (режимы: bubbles/flat/document) или `.is-*` класс (локальные модификаторы).
4. Брейкпоинт **`960px`**, один. Ниже — мобильный flow.
5. Не стилизуй `body`, `html`, `input` без префикса — ты ломаешь пользовательские моды.
6. Шаблон компонента + чеклист PR — `docs/DEVELOPER_GUIDE.md`.

### Для пользователя (модера)

1. Settings → Appearance → Custom CSS.
2. Дефолтный режим — **scope: chat**: CSS оборачивается в `@scope (#chat)` и красит только чат.
3. **Global** — красит весь шелл. Осторожно с `body`, `input`, `textarea` — задеваешь меню.
4. Всё сломалось? Добавь `?safe` к URL — шелл загрузится без пользовательского CSS.
5. Полный гайд с примерами — `docs/CSS_MODS_GUIDE.md`. Живая песочница — `playground.html`.

---

## Карта репозитория

### Правила
| Файл | Про что |
|---|---|
| `docs/VISUAL_FOUNDATIONS.md` | Визуальная ДНК: цвет, тип, спейсинг, тени, радиусы, анимации, состояния |
| `docs/ADAPTIVE_RULES.md` | Правила адаптива для desktop / mobile web / mobile webview |
| `docs/SELECTOR_CONTRACT.md` | **Публичный контракт** якорей — стабильно между версиями |
| `docs/CSS_MODS_GUIDE.md` | Туториал пользователя: первая тема за 10 минут |
| `docs/CUSTOMIZATION_SURFACE.md` | **Полный inventory** точек кастомизации: кнопки, toggles, логотип, loader, фоны, декорации |
| `docs/DEVELOPER_GUIDE.md` | Шаблон компонента, правила, чеклист PR |
| `docs/CONTENT_FUNDAMENTALS.md` | Тон, RU/EN копирайт, casing, эмодзи |

### Токены + примеры тем
| Файл | Что даёт |
|---|---|
| `tokens/colors_and_type.css` | Все CSS-переменные + семантические классы типографики (копируй в любой файл — работает) |
| `tokens/customization.css` | Расширенный набор: кнопки, toggles, слайдеры, loader, декорации, ST-профиль mapping |
| `themes/nest-default-dark.css` | Эталон тёмной темы |
| `themes/nest-default-light.css` | Эталон светлой (Dossier-inspired) |
| `themes/tavern-warm.css` | Пример: пергамент + бронза |
| `themes/cyber-neon.css` | Пример: фиолет + магента |
| `themes/minimal-reader.css` | Пример: editorial reader mode |

### Интерактив и агенты
- **`playground.html`** — переключатель тем + live-редактор CSS. Открывай первым, если ты видишь эту систему впервые.
- **`SKILL.md`** — точка входа для Claude / Claude Code. Совместимо с Agent Skills.
- **`preview/`** — карточки для вкладки Design System (не трогай вручную, генерируются).

---

## SillyTavern-совместимость

WuNest понимает ST-темы «из коробки». Читаются следующие переменные:

```
--SmartThemeBodyColor        → --nest-text
--SmartThemeBorderColor      → --nest-border
--SmartThemeQuoteColor       → --nest-accent
--SmartThemeBlurTintColor    → --nest-bg
--SmartThemeChatTintColor    → --nest-surface
--SmartThemeBodyFont         → --nest-font-body
```

Доступные ST-алиасы: `.mes`, `.mes_block`, `.mes_text`, `.mes_name`, `#chat`, `#top-bar`, `#sheld`, `#leftNavPanel`, `#send_form`, `#send_textarea`, `#send_but`. При импорте ST-темы по JSON scope автоматически выставляется в `chat`. Полный список и поведение при конфликте приоритетов — в `docs/SELECTOR_CONTRACT.md`.

---

## Источники (для автора этой документации)

Всё, что ниже, — внутренняя ссылка на фронт. Читателю документа доступ к этим файлам **не нужен**; значения и правила перенесены в `tokens/` и `docs/`.

- `frontend/src/styles/global.scss` — источник токенов
- `frontend/src/plugins/vuetify.ts` — Vuetify-темы `nestDark` / `nestLight`
- `frontend/src/lib/cssScope.ts` — движок `@scope(#chat)` для пользовательского CSS
- `frontend/src/stores/appearance.ts` — применение токенов + safe-mode
- `frontend/src/components/AppearancePanel.vue` — UI редактора мода
- `frontend/src/layout/AppShell.vue` — топбар, drawer, nav
- `frontend/src/components/MessageBubble.vue` — эталонный компонент

---

## Статус документа

Эта дизайн-система — **контракт**, не рефакторинг. Часть якорей из `SELECTOR_CONTRACT.md` уже есть в коде, часть предложена к внедрению. Перед финализацией контракта пройдись по списку и отметь: "есть", "добавить", "не нужен". После этого замороженный список становится обещанием авторам тем.
