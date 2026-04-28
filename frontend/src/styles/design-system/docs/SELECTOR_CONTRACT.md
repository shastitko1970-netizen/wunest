# Selector Contract

**Публичный контракт WuNest с авторами тем.** Всё, что перечислено здесь, мы обещаем сохранять между версиями. Если селектор/переменная/атрибут удаляется или переименовывается — это breaking change, объявленный в CHANGELOG заранее.

Всё, чего здесь нет — **внутреннее**. Использовать можно, но не гарантируем стабильности.

---

## 1. CSS-переменные (стабильные)

### WuNest-native (`--nest-*`)

```
Палитра (значение меняется темой):
  --nest-bg                 главный фон
  --nest-bg-elevated        приподнятая поверхность (sidebar, hover)
  --nest-surface            карточка/сообщение/dialog
  --nest-border             стандартная граница
  --nest-border-subtle      тонкая граница
  --nest-text               основной текст
  --nest-text-secondary     вторичный
  --nest-text-muted         третичный
  --nest-accent             бренд-акцент
  --nest-text-on-accent     читаемый цвет поверх accent (CTA текст)
  --nest-text-italic        курсив (em/i) — fallback inherit (M51)
  --nest-text-quote         blockquote — fallback на --nest-text-secondary (M51)
  --nest-gold
  --nest-green
  --nest-blue
  --nest-ember

Шрифты:
  --nest-font-display
  --nest-font-body
  --nest-font-mono
  --nest-font-scale         (legacy, не используется — оставлен для compat)
  --nest-chat-font-scale    multiplier чата (slider 0.7–1.5, M51)

Геометрия:
  --nest-radius-scale       multiplier (M51 — slider 0.5–2×)
  --nest-radius-sm          calc(6px  * var(--nest-radius-scale, 1))
  --nest-radius             calc(12px * var(--nest-radius-scale, 1))
  --nest-radius-lg          calc(16px * var(--nest-radius-scale, 1))
  --nest-radius-pill        100px (НЕ scaled — семантика «всегда max-round»)
  --nest-avatar-radius

Спейсинг:
  --nest-space-1 … --nest-space-16

Тени и эффекты:
  --nest-shadow-sm
  --nest-shadow
  --nest-shadow-lg
  --nest-blur

Анимации:
  --nest-transition-fast
  --nest-transition-base

Layout:
  --nest-header-height
  --nest-sidebar-width
  --nest-content-max
  --nest-chat-width
```

### ST-совместимые (fallback-chain)

Поддерживаются **для чтения**: WuNest токены используют их как дефолт через `var(--SmartTheme*, fallback)`. Пишешь `--SmartTheme*` — подменяется WuNest-эквивалент.

```
--SmartThemeBodyColor        → --nest-text / --nest-text-secondary / --nest-text-muted
--SmartThemeBorderColor      → --nest-border / --nest-border-subtle
--SmartThemeQuoteColor       → --nest-accent
--SmartThemeBlurTintColor    → --nest-bg
--SmartThemeChatTintColor    → --nest-bg-elevated / --nest-surface
--SmartThemeBodyFont         → --nest-font-body
```

---

## 2. ID-якоря (стабильные)

Чувствительны к переименованию — используй только ID из этого списка.

| ID | Где | Что таргетировать |
|---|---|---|
| `#top-bar` | AppShell v-app-bar | топбар целиком |
| `#sheld` | v-main | главная контентная область |
| `#leftNavPanel` | mobile drawer | навигационный drawer |
| `#chat` | Chat view | контейнер чата, scope для кастомного CSS |
| `#send_form` | composer wrapper | форма отправки |
| `#send_textarea` | composer input | поле ввода сообщения |
| `#send_but` | composer submit | кнопка отправки |

Все эти ID одновременно существуют в SillyTavern → импорт ST-темы работает без маппинга.

---

## 3. Классы-якоря `.nest-*`

Стабильные классы. Именование: `.nest-<componentName>[-<part>][-<modifier>]`.

### Shell
- `.nest-shell` — корневой контейнер
- `.nest-topbar`
- `.nest-topnav` — nav-стрип в топбаре (desktop)
- `.nest-topnav-item`
- `.nest-sidebar` — мобильный drawer
- `.nest-logo-mark`, `.nest-logo-text`

### MessageBubble
- `.nest-msg` — корневой элемент сообщения
- `.nest-msg.is-user` / `.nest-msg.is-streaming` / `.nest-msg.is-hidden`
- `.nest-msg-header`
- `.nest-msg-name`
- `.nest-msg-time`
- `.nest-msg-body`
- `.nest-msg-content`
- `.nest-msg-footer`
- `.nest-msg-actions`
- `.nest-action-btn`
- `.nest-reasoning`, `.nest-reasoning-summary`, `.nest-reasoning-body`
- `.nest-swipe-count`
- `.nest-cursor` — мигающий курсор стриминга

### Типографика
- `.nest-h1`, `.nest-h2`, `.nest-h3`, `.nest-h4`
- `.nest-subtitle`
- `.nest-body`
- `.nest-caption`
- `.nest-eyebrow`
- `.nest-kpi`, `.nest-kpi--gold`
- `.nest-mono`

### Card/Grid
- `.nest-card` (где встречается)
- `.nest-grid` — 2-колоночный, сворачивается в 1 на < 640
- `.nest-field` — блок поля формы
- `.nest-field-label`
- `.nest-hint`

### Avatar
- `.nest-avatar--forced-round` — опт-аут из portrait-режима

---

## 4. ST-алиасы (живут параллельно)

Ставятся **одновременно** с nest-классами, чтобы ST-темы работали без миграции.

| ST-класс | WuNest-эквивалент |
|---|---|
| `.mes` | `.nest-msg` |
| `.mes_user` | `.nest-msg.is-user` |
| `.mes_char` | `.nest-msg:not(.is-user)` |
| `.mes_block` | `.nest-msg-body` |
| `.mes_text` | `.nest-msg-content` |
| `.mes_name` | `.nest-msg-name` |
| `.last_mes` | `:last-of-type` на `.nest-msg` (пока не используется активно) |
| `.topbar` | `.nest-topbar` |
| `.drawer-content` | `.nest-sidebar` (mobile drawer) |

---

## 5. `data-*` атрибуты

Устанавливаются на `:root` (или на корневом элементе компонента). Моды цепляются селекторами `[data-nest-…='...']`.

### На `:root`

| Атрибут | Значения | Источник |
|---|---|---|
| `data-nest-avatar-style` | `round` / `square` / `portrait` | AppearancePanel |
| `data-nest-chat-display` | `bubbles` / `flat` / `document` | AppearancePanel |
| `data-nest-theme` | `dark` / `light` | планируется как альтернативный путь к Vuetify-классу |
| `data-nest-platform` | `web` / `app` | детектится на boot (см. ADAPTIVE_RULES) |

### На сообщении `.nest-msg`
| Атрибут | Значение |
|---|---|
| `data-message-id` | UUID сообщения |

Не добавляй новые `data-nest-*` атрибуты без добавления в этот список. При первой публичной фиксации — PR в этот файл.

---

## 6. Версионирование

- Текущая версия контракта: **v1** (апрель 2026).
- Breaking changes идут через major bump + migration notes в CHANGELOG.
- Добавление **нового** якоря не breaking. Удаление или переименование — breaking.

### Что НЕ стабильно (не используй)

- Внутренние классы Vuetify (`.v-field__outline`, `.v-btn__content`, etc.) — могут меняться с обновлением Vuetify.
- Scoped-стили Vue (`data-v-xxxxxx` атрибуты) — генерятся билдером.
- Любые классы, начинающиеся с `nest-__internal-*` (если появятся).

---

## 7. Checklist при добавлении нового компонента

Если ты разработчик WuNest:

- [ ] Корневой элемент получил класс `.nest-<component>`.
- [ ] Если это главный блок продукта — добавлен ST-алиас.
- [ ] Важные состояния — модификаторы класса (`is-*`) или data-атрибуты, не inline-стили.
- [ ] Все цвета, радиусы, спейсинги — через `var(--nest-*)`.
- [ ] Якорь, который обещаем модерам, добавлен в этот файл.
- [ ] PR-ревью упоминает «contract update».
