# Visual Foundations

Визуальная ДНК WuNest. Читай сверху вниз, если нужно понять «как должно выглядеть», или ныряй в якорную секцию через Ctrl+F.

---

## 1. Палитра

Две базовые темы: **nestDark** (дефолт) и **nestLight** (Dossier-paper). Обе синхронизированы по смысловым токенам, отличаются только значениями.

### Семантика

| Токен | Роль | Правило |
|---|---|---|
| `--nest-bg` | главный фон приложения | занимает `body`, топбар, пустые области |
| `--nest-bg-elevated` | чуть приподнятая поверхность | sidebar, меню, drawer, inputs hover |
| `--nest-surface` | карточка/модалка/сообщение | MessageBubble, dialog, CharacterCard |
| `--nest-border` | обычная граница | между областями, divider |
| `--nest-border-subtle` | тонкая граница | внутри карточки, dashed separators |
| `--nest-text` | основной текст | заголовки, body |
| `--nest-text-secondary` | вторичный | подписи, .nest-subtitle |
| `--nest-text-muted` | третьестепенный | timestamp, caption, placeholder |
| `--nest-accent` | бренд-акцент | CTA, focus, active state |
| `--nest-gold` / `--nest-green` / `--nest-blue` / `--nest-ember` | семантические декоры | quota, success, info, warmth |

### Правила использования

- **Никогда** не пиши hex-цвет напрямую в компоненте. Используй токен. Если нужного оттенка нет — обсуди добавление нового токена.
- Фон текста и сам текст берутся **из одной пары**: `bg + text`, `surface + text`, `bg-elevated + text`. Не миксуй.
- Для hover фона используй `--nest-bg-elevated`. Для hover текста — не меняй, меняй фон/границу.
- Для danger не заводи отдельную переменную, используй `--nest-accent` (он красный в обеих темах) или `color-mix(in srgb, var(--nest-accent) 12%, transparent)` для тонированного фона.

### Цвет акцента — один на всю тему

В nestDark и nestLight `--nest-accent = #ef4444` (coral/red). Это сделано сознательно: идентичность бренда одна в обеих палитрах. Пользовательские темы **могут** менять через `--SmartThemeQuoteColor`.

---

## 2. Типографика

Три семейства, каждая с одной функциональной ролью:

| Семейство | Роль | Когда использовать |
|---|---|---|
| **Fraunces** (`--nest-font-display`) | editorial/display | `nest-h1`, `nest-h2`, `nest-kpi`, имя персонажа в `.nest-msg-name` |
| **Outfit** (`--nest-font-body`) | body | весь обычный текст, кнопки, формы |
| **JetBrains Mono** (`--nest-font-mono`) | mono | `nest-caption`, `nest-eyebrow`, timestamps, код, числовые поля, tokens-count |

### Шкала

- Заголовки fluid: `clamp(2rem, 4vw, 3rem)` для `h1`, `clamp(1.5rem, 3vw, 2rem)` для `h2` — адаптируются без медиазапросов.
- Body 15px, line-height 1.55.
- Caption 11px, letter-spacing 0.05em — tracked uppercase = визуальный сигнал «мета-информация».
- Eyebrow (надзаголовок секции): 0.72rem, 0.12em letter-spacing, uppercase.

### Трекинг и вес

- Display (Fraunces) — `font-weight: 400`, `letter-spacing: -0.02em` для `h1`, `-0.015em` для `h2`. Никогда `bold` (500+) в display.
- Body — 400 обычный, 500 для заголовков внутри UI.
- Mono — 400/500. Tracking `0.05em` для caption, `0.1em` для eyebrow.

---

## 3. Спейсинг

Сетка на **4px**. Используй переменные, не пиши числа.

```
--nest-space-1  = 4px    ← между иконкой и меткой
--nest-space-2  = 8px    ← между элементами формы в ряду
--nest-space-3  = 12px   ← отступ карточки изнутри по короткой оси
--nest-space-4  = 16px   ← стандартный padding карточки
--nest-space-6  = 24px   ← gap между секциями Settings
--nest-space-8  = 32px   ← между больших блоков
--nest-space-12 = 48px   ← между section hero и контентом
--nest-space-16 = 64px   ← vertical rhythm на landing
```

Правило: если нужен `5px`, `7px`, `10px` — стоп. Вероятно, ты не по сетке. Выбери ближайшее число из шкалы.

---

## 4. Скругления

```
--nest-radius-sm    =  6px   ← chips, small buttons, swatches, textarea
--nest-radius       = 12px   ← cards, MessageBubble, dialogs
--nest-radius-lg    = 16px   ← hero cards, big images
--nest-radius-pill  = 100px  ← капсулы, теги, количественные пилюли
```

Аватарки — отдельная ось через `--nest-avatar-radius` (меняется `data-nest-avatar-style`): `round` = 50%, `square` = 4px, `portrait` = 12px + aspect-ratio 3/4.

---

## 5. Тени

Три шага:

```
--nest-shadow-sm  = 0 1px 2px rgba(0,0,0,0.08)   ← edge-lift: chips, subtle
--nest-shadow     = 0 2px 8px rgba(0,0,0,0.15)   ← карточка, MessageBubble
--nest-shadow-lg  = 0 8px 24px rgba(0,0,0,0.25)  ← dialog, popover
```

`--nest-shadow` можно отключить глобально через Appearance toggle (пишется `--nest-shadow: none`). Компоненты, у которых тень критична (dialog), должны использовать `--nest-shadow-lg` — он не отключается.

---

## 6. Бордеры

- Дефолт: `1px solid var(--nest-border)`.
- Тонкий разделитель внутри карточки: `1px solid var(--nest-border-subtle)` или `1px dashed var(--nest-border-subtle)` (для «advisory» разделителей типа token-info в MessageBubble).
- Hidden/muted состояние: `border-style: dashed` + `opacity: 0.5`.
- Focus: `border-color: var(--nest-accent)` (текстовые поля), `box-shadow: inset 0 -2px 0 var(--nest-accent)` (активная вкладка в топнаве).

---

## 7. Фоны и изображения

- Дефолтный фон — **плоский** `var(--nest-bg)`. Никаких декоративных градиентов на шелле.
- Пользователь может загрузить **chat background image** (`bgImageUrl`) через Appearance. Изображение уходит в `body.style.backgroundImage` с `cover, center, fixed`.
- Вместе с фоном используется **blur**: `--nest-blur` от 0 до 20px. Применяется к поверхностям с `backdrop-filter: blur(var(--nest-blur))` — только там, где это осмысленно (surface над фоном).
- Паттерны, текстуры, hand-drawn иллюстрации — **не используем**. Дизайн «editorial-tech»: чистый, с одним типографическим акцентом.
- Full-bleed изображения — только в Library (preview карточек персонажей) и в character-card aspect 3/4 когда `avatarStyle = portrait`.

---

## 8. Анимации

**Два токена**, не более:

```
--nest-transition-fast  = 0.15s ease   ← hover, focus, micro-interactions
--nest-transition-base  = 0.2s ease    ← открытие меню, route transitions
```

- Кастомных кривых (cubic-bezier) в продукте **нет**, кроме `nest-pulse` для boot-spinner и `nest-blink` для курсора стриминга.
- Переходы между роутами — `fade + translateY(4px)` через `<transition name="nest-fade">`.
- Ничего не «прыгает», ничего не «отскакивает». Bounces, overshoot, spring-физика — запрещены в шелле.
- При `Appearance.reducedMotion = true` оба токена переписываются в `0s` → вся анимация выключается. Учитывай это: не делай анимацию **обязательной** для понимания состояния.

### Состояния

| Состояние | Эффект |
|---|---|
| Hover (кнопка, пункт меню) | `background: var(--nest-bg-elevated)` + `color: var(--nest-text)` |
| Hover (primary CTA) | `filter: brightness(1.1)` |
| Focus (input) | `border-color: var(--nest-accent)` |
| Active (route, tab) | фон `--nest-bg-elevated` + `box-shadow: inset 0 -2px 0 var(--nest-accent)` |
| Press | `transform: translateY(-1px)` (только для CTA); без scale |
| Disabled | `opacity: 0.55` + `cursor: help` или `cursor: not-allowed` |

---

## 9. Прозрачность и блюр

- Модальный скрим — `rgba(0, 0, 0, 0.6)` + `backdrop-filter: blur(2px)`. Темнее Vuetify-дефолта, потому что на тёмной теме 0.32 читается как тень, а не как «сверху что-то».
- Поверхности поверх `bgImageUrl` должны иметь backdrop-filter (если пользователь выставил blur > 0). Используй `var(--nest-blur)` — сам токен ставит нужный размер.
- Никогда не используй `opacity < 0.5` на тексте. Вместо этого — `--nest-text-muted`.

---

## 10. Карточка — эталон

```css
.nest-card {
  padding: var(--nest-space-4);
  background: var(--nest-surface);
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius);
  box-shadow: var(--nest-shadow-sm);
  transition: border-color var(--nest-transition-fast);
}
.nest-card:hover { border-color: var(--nest-border); }
```

MessageBubble, CharacterCard, любой content-container в Settings — всё идёт от этого снипа.

---

## 11. Иконки

- Иконочная система — **Material Design Icons** (`@mdi/font` 7.x), префикс `mdi-`. Используются через Vuetify: `<v-icon>mdi-forum-outline</v-icon>`.
- Outline-варианты предпочтительны (`mdi-account-circle-outline`, `mdi-cog-outline`). Filled — только для активных состояний.
- Размер: 14 для actions внутри карточек, 16 для topbar/nav, 18 для primary-CTA, 48 для пустых состояний.
- Emoji используется **только в пользовательском контенте** (имя персонажа, сообщения). В шелле — никогда.

---

## 12. Layout grid

- Max-width основного контента: `--nest-content-max = 820px` (читаемая мера).
- Ширина чата: `--nest-chat-width`, дефолт 60%, юзер меняет слайдером 40–100%.
- Grid карточек в Library: `repeat(auto-fill, minmax(240px, 1fr))`, gap `--nest-space-4`.

---

## 13. Что запрещено

- Фиолетово-синие градиенты на фонах и кнопках.
- Карточки с цветной левой границей.
- Жирные drop shadows (> 30% opacity).
- Иконки и эмодзи-плейсхолдеры на пустых состояниях (используй mdi-outline + короткий текст).
- `border-radius: 50px` на прямоугольных кнопках — либо `pill`, либо скругления с шагом шкалы.
- `font-family: Inter` или `Arial` где-либо в продукте.
