# Оформление и CSS

WuNest поддерживает **четыре** уровня кастомизации:

1. Готовые пресеты (5 штук)
2. UI-тумблеры во «Внешнем виде»
3. ST-совместимые CSS-переменные
4. Свой CSS + scope

> Посмотреть как выглядят все 5 пресетов → [галерея тем](/themes) — публичная страница, можно шарить ссылкой.

---

## Встроенные пресеты (M42)

**Настройки → Внешний вид → Тема** — пять готовых тем:

| Preset | Kind | Описание |
|--------|:----:|----------|
| **Nest — dark** | 🌑 | Фирменная тёмная (уголь + coral-акцент) |
| **Nest — light** | ☀️ | Бумажно-светлая, вдохновлена Dossier CRM |
| **Cyber neon** | 🌑 | Тёмный фиолет + магента-свечение |
| **Minimal reader** | ☀️ | Максимальная плотность текста, без декора |
| **Tavern warm** | 🌑 | Тёплый янтарный, roadhouse-эстетика |

Быстрое переключение **светлая ↔ тёмная** — в **Настройки → Тема** (верхняя секция). Pair-logic запоминает, какой именно dark- и light-preset ты выбирал: cyber-neon ↔ minimal-reader round-trip'ит без потери выбора.

---

## Быстрые настройки (UI)

**Настройки → Внешний вид**:

- Размер шрифта (0.75× — 1.4×)
- Ширина чата (40–100%)
- Форма аватара (круглые / квадратные)
- Стиль сообщений (пузыри / плоский / документ)
- Акцентный цвет, цвет фона, текста, рамок
- Фоновая картинка + blur
- Тени, анимации, HTML-рендеринг

Это пишет CSS-переменные на `:root` inline — срабатывает мгновенно без перезагрузки.

---

## Импорт и экспорт

### SillyTavern-совместимый JSON

**Внешний вид → Импорт темы ST (.json)**. Из файла извлекаются:

- **Цвета**: `main_text_color`, `italics_text_color`, `quote_text_color`, `border_color`
- **Размеры**: `font_scale`, `chat_width`, `blur_strength`
- **Стили**: `avatar_style`, `chat_display`, `noShadows`, `reduced_motion`
- **`custom_css`** — применяется как свой CSS

Автоматически выставляется scope `chat` — CSS ST-темы не ломает меню. Если тема содержит правила для общих элементов (`body`, `textarea`, `input`), показывается info-уведомление.

### Raw CSS (.css)

**Внешний вид → Импорт .css файла** — загружает обычный `.css` без JSON-обёртки (M42.4). Удобно, когда тема ходит по комьюнити как просто `.css`. Scope тоже по умолчанию `chat`.

**Внешний вид → Экспорт в .css** — выгружает твой текущий custom CSS как standalone `.css`. Удобно делиться в Discord/форумах. Имя файла берётся из комментария `/* Name: ... */` если он есть.

---

## Свой CSS — полный гайд

**Внешний вид → Свой CSS** — textarea для ручного кода. Поддерживает полный CSS-синтаксис, включая `@import`, `@font-face`, `@media`, `@supports`, nesting, `:has()`, `@scope` и пр. Сохраняется на сервер (`user.settings.custom_css`) с debounce 400 ms и инжектится при загрузке как `<style id="nest-user-css">` в `<head>`.

### Философия: token-first

**Главная мысль:** не воюй с переменными — переопределяй их.

WuNest внутри построен на CSS-переменных. Любой цвет в интерфейсе — это `var(--nest-текст, fallback)`. Если ты поменял переменную на `:root`, **весь шелл** автоматически перекрасится, без селекторов, без `!important`, без хрупкости.

Сравни:

```css
/* ❌ Хрупко: крашится через обновление */
.nest-msg-body .nest-msg-content p {
  color: #c485ff !important;
}

/* ✅ Стабильно: работает всегда */
:root { --SmartThemeBodyColor: #c485ff; }
```

Первый вариант ломается, если мы переименуем класс или добавим новый wrapper. Второй — переживёт все рефакторы, пока контракт переменных жив.

### Scope — «Куда применять»

Под textarea'ой — тумблер с двумя режимами:

| Режим | Что захватывает | Когда выбирать |
|---|---|---|
| **Только чат** (default) | `#chat` и его потомки | ST-темы, пробы, правила с `body`/`textarea`/`input` не трогают меню |
| **На всю апу** | Весь шелл | Темы на базе `.nest-*`, когда реально нужно перекрасить топбар, sidebar, диалоги |

**Как работает «Только чат»:**

На современных браузерах (Chromium, Safari 18+) твой CSS оборачивается в native `@scope (#chat) { ... }`. На Firefox применяется ручной prefixer: каждое правило получает префикс `#chat `. Поведение в 95% случаев идентично.

Различия, о которых стоит знать:

- Nesting (`&`) и `:has()` могут вести себя чуть иначе в prefixer-fallback'е. Для публичных тем nesting лучше избегать.
- `@media` внутри user CSS работает везде — scope применяется снаружи media-query.
- `@import`, `@font-face`, `@keyframes` автоматически «поднимаются» в начало документа — они не могут быть scoped по спеку CSS.

**Audit опасных селекторов:** под textarea'ой система в реальном времени считает селекторы, которые перекрасят весь шелл (`body`, `html`, `textarea`, `input`, `.menu_button`, `#top-bar`, и т.д.). Если включён режим «На всю апу» и таких селекторов > 0, появляется предупреждение.

### Пять уровней кастомизации

Располагаются от самого стабильного к самому хрупкому. **Рекомендация: оставайся на уровнях 1–3.**

---

#### Уровень 1 — переменные `--SmartTheme*` (рекомендуется)

Это публичный API, совместимый с SillyTavern. **Самый стабильный уровень**: работает на всех версиях WuNest, не ломается при рефакторах.

```css
:root {
  --SmartThemeBlurTintColor: #0b0818;         /* главный фон */
  --SmartThemeChatTintColor: #130d15;         /* фон карточек и сообщений */
  --SmartThemeBorderColor:   #3c1e50;         /* границы */
  --SmartThemeBodyColor:     #f1d1ff;         /* основной текст */
  --SmartThemeQuoteColor:    #c485ff;         /* акцент: CTA, focus */
  --SmartThemeBodyFont:      'Inter';         /* шрифт тела */
  --SmartThemeEmColor:         #9d4edd;       /* курсив / <em> */
  --SmartThemeUnderlineColor:  #7a2fb8;       /* подчёркивания / ссылки */
  --SmartThemeShadowColor:     rgba(0,0,0,.4);/* тени под сообщениями */
  --SmartThemeUserMesBlurTintColor: #1c1228;  /* фон сообщения юзера */
  --SmartThemeBotMesBlurTintColor:  #150c1f;  /* фон сообщения AI */
}
```

Первые пять переменных покрывают 80% нужд темы. Остальные нужны для fine-tuning. Сохрани — смотри как перекрасился весь шелл.

---

#### Уровень 2 — `--nest-*` токены напрямую

Внутренние токены WuNest. Обычно не нужно трогать их напрямую — они сами наследуются из `--SmartTheme*`. Но если нужен **тонкий контроль** (например, задать кастомный radius для карточек отдельно от bubble'ов):

```css
:root {
  --nest-bg:             #0a0a0f;
  --nest-surface:        #141018;
  --nest-text:           #e8e0f0;
  --nest-accent:         #9d4edd;
  --nest-border:         #2a1f3a;
  --nest-border-subtle:  #1a1428;
  --nest-radius:         16px;
  --nest-radius-sm:      8px;
  --nest-font-body:      'Inter', sans-serif;
  --nest-font-display:   'Fraunces', serif;
  --nest-font-mono:      'JetBrains Mono', monospace;
  --nest-transition-fast: 0.15s ease;
}
```

⚠️ **Trade-off:** если мы когда-то переименуем токен, тема сломается. Для максимальной стабильности предпочитай уровень 1.

Полный список токенов — в `tokens/colors_and_type.css` и `tokens/customization.css` (лежит в `frontend/src/styles/`).

---

#### Уровень 3 — классы `.nest-*`

Публичные якоря WuNest (из **Selector Contract**). Их мы не меняем без changelog'а.

**Сообщения:**

| WuNest class | ST alias | Что это |
|---|---|---|
| `.nest-msg` | `.mes` | Строка сообщения |
| `.nest-msg-body` | `.mes_block` | Тело пузыря (фон, рамка) |
| `.nest-msg-content` | `.mes_text` | Содержимое (текст) |
| `.nest-msg-name` | `.mes_name` | Имя отправителя |
| `.nest-msg-avatar` | `.mes .avatar` | Аватар |
| `.nest-msg-time` | — | Timestamp |
| `.nest-msg-actions` | `.mes_buttons` | Row с кнопками (edit, swipe, delete) |

**Шелл:**

| WuNest class/id | ST alias | Что это |
|---|---|---|
| `.nest-chat-scroll` | `#chat` | Контейнер с историей |
| `.nest-chat-input` | `#send_form` | Композер (форма ввода) |
| input textarea | `#send_textarea` | Поле ввода (ID-хук) |
| send button | `#send_but` | Кнопка Send (ID-хук) |
| `.nest-topbar` | `#top-bar`, `.topbar` | Верхняя панель |
| `.nest-sidebar` | `#leftNavPanel` | Левый список чатов |

**Модификаторы состояния:**

| Класс | Где | Когда активен |
|---|---|---|
| `.is-user` | `.nest-msg` | сообщение от юзера (есть ещё ST-алиасы `.mes_user` / `.mes_char`) |
| `.is-streaming` | `.nest-msg` | пока сообщение streamится |
| `.is-error` | `.nest-msg-body` | когда в сообщении ошибка генерации |
| `.is-favorite` | `.nest-char-card` | когда карточка отмечена звёздочкой |

Пример:

```css
/* Fancy border для favorites в библиотеке */
.nest-char-card.is-favorite {
  border-left: 3px solid gold;
}

/* Pulse для streaming сообщения */
.nest-msg.is-streaming .nest-msg-content {
  animation: nest-pulse 1.2s ease-in-out infinite;
}
@keyframes nest-pulse {
  50% { opacity: 0.7; }
}
```

---

#### Уровень 4 — ST-алиасы (`.mes`, `#chat`, ...)

**Для совместимости с SillyTavern**-темами. Работают те же селекторы, что в ST. Если ты импортируешь `.json` или `.css` из ST — этот слой «just works».

```css
.mes { border-radius: 20px; }
.mes_name { font-style: italic; }
.mes_text { font-size: 16px; }
#send_textarea { background: #1a0f1f; color: #f1d1ff; }
#chat { padding: 24px; }
```

⚠️ Внутренняя структура у нас **чуть другая**: у `.mes` нет всех «правнуков» из ST. Что работает — в таблицах выше. Что не работает — см. раздел «Совместимость» ниже.

---

#### Уровень 5 — data-атрибуты (реакция на режим)

Продвинутый приём. Позволяет писать CSS, который **срабатывает только в определённом режиме** (document vs bubble, square avatar vs round, и т.д.).

Доступные атрибуты живут на `<html>` или `#chat`:

| Атрибут | Значения |
|---|---|
| `data-nest-chat-display` | `bubbles` / `flat` / `document` |
| `data-nest-avatar-style` | `round` / `square` / `portrait` |
| `data-nest-density` | `compact` (остальные планируются) |
| `data-nest-reduced-motion` | присутствует когда включено в настройках |

Пример: editorial-типографика только в document-режиме:

```css
[data-nest-chat-display='document'] .nest-msg-body {
  font-family: 'Fraunces', serif;
  font-size: 17px;
  line-height: 1.75;
  max-width: 68ch;
}

[data-nest-chat-display='document'] .nest-msg-name {
  font-variant-caps: small-caps;
  letter-spacing: 0.08em;
}
```

Пример: добавить тень к square-аватарам:

```css
[data-nest-avatar-style='square'] .nest-msg-avatar {
  border-radius: 8px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
}
```

---

## Рецепты

### Пузырь со свечением

```css
.nest-msg-body {
  background: rgba(30, 15, 45, 0.65);
  border: 1px solid #3c1e50;
  border-radius: 14px;
  box-shadow:
    0 2px 14px rgba(120, 60, 200, 0.25),
    inset 0 1px 0 rgba(255, 255, 255, 0.03);
  backdrop-filter: blur(6px);
}

.nest-msg-name {
  color: #c485ff;
  text-shadow: 0 0 12px rgba(196, 133, 255, 0.4);
}
```

### Пергамент / таверна

```css
:root {
  --SmartThemeBlurTintColor: #1a130a;
  --SmartThemeChatTintColor: #2a1f12;
  --SmartThemeBodyColor:     #f4e4c1;
  --SmartThemeBorderColor:   #6b4a2a;
  --SmartThemeQuoteColor:    #d4904a;
  --SmartThemeBodyFont:      'Fraunces';
}

.nest-msg-body {
  border-radius: 4px;
  box-shadow:
    0 1px 0 rgba(0, 0, 0, 0.3),
    inset 0 1px 0 rgba(255, 220, 160, 0.05);
}

.nest-msg-name {
  font-style: italic;
  letter-spacing: 0.03em;
}
```

### Reader-mode типографика

```css
[data-nest-chat-display='document'] .nest-msg-body {
  font-family: 'Fraunces', Georgia, serif;
  font-size: 17px;
  line-height: 1.78;
  max-width: 68ch;
}

[data-nest-chat-display='document'] .nest-msg-body em {
  font-style: italic;
  color: #555;
}

/* Drop-cap для первой буквы */
[data-nest-chat-display='document'] .nest-msg-content > p:first-child::first-letter {
  font-size: 3.2em;
  font-family: 'Fraunces', serif;
  float: left;
  line-height: 0.9;
  margin: 6px 8px 0 0;
  color: var(--nest-accent);
}
```

### Разные акценты для юзера и персонажа

Для разграничения сторон диалога — используем модификатор `.is-user` (у сообщений юзера он есть, у AI — нет):

```css
/* Ты — холодный cyan */
.nest-msg.is-user .nest-msg-name {
  color: #4dd0e1;
}

/* AI — тёплая магента */
.nest-msg:not(.is-user) .nest-msg-name {
  color: #c485ff;
}

/* Тонкая рамка слева — цветная по стороне */
.nest-msg.is-user .nest-msg-body {
  border-left: 3px solid #4dd0e1;
}
.nest-msg:not(.is-user) .nest-msg-body {
  border-left: 3px solid #c485ff;
}

/* Альтернатива: ST-алиасы .mes_user / .mes_char */
.mes_user .mes_name { color: #4dd0e1; }
.mes_char .mes_name { color: #c485ff; }
```

### Скрыть timestamps

```css
.nest-msg-time { display: none; }
```

(В scope `chat` глобальный топбар не тронешь.)

### Кастомный шрифт с Google Fonts

```css
@import url('https://fonts.googleapis.com/css2?family=Andika&display=swap');

:root {
  --SmartThemeBodyFont: 'Andika';
}
```

`@import` система автоматически поднимает в начало документа — это работает даже в scope mode.

### Свой шрифт с CDN

```css
@font-face {
  font-family: 'MyFont';
  src: url('https://my-cdn.com/my-font.woff2') format('woff2');
  font-display: swap;
}

:root { --SmartThemeBodyFont: 'MyFont'; }
```

### Тонкое редактирование композера

```css
/* Более воздушный композер */
.nest-chat-input {
  padding: 18px 20px;
  background: rgba(20, 10, 35, 0.5);
  backdrop-filter: blur(10px);
}

#send_textarea {
  font-size: 15px;
  line-height: 1.5;
  color: #f1d1ff;
}

#send_textarea::placeholder {
  color: rgba(241, 209, 255, 0.35);
  font-style: italic;
}
```

---

## Чего избегать

### ❌ Правила на `body` / `html` в режиме global

```css
/* СЛОМАЕТ топбар и меню — цвет, скролл, всё */
body { background: black !important; }
```

В scope `chat` такие правила **игнорируются** (по спеку `@scope` — глобальные элементы выносятся). Но в global они сломаются. **Альтернатива:** меняй переменные (уровень 1).

### ❌ `!important` без нужды

Каждый `!important` — это гвоздь в будущую поддержку темы. Ты конкурируешь сам с собой при апдейтах WuNest. Используй только когда реально нет другого выхода (например, нужно убить `transition: none` из системного reduced-motion).

### ❌ Абсолютные позиции на ключевых элементах

```css
/* НЕ делай: сломает мобильную раскладку, keyboard handling, IME */
#chat { position: absolute !important; top: 100px; }
```

### ❌ Фиксированная ширина в px

```css
/* Плохо: ломает мобильный */
.nest-msg-body { width: 600px !important; }

/* Хорошо */
.nest-msg-body { max-width: 600px; width: 100%; }
```

### ❌ Изменение `display` на ключевых контейнерах

Не ставь `display: block` / `flex` / `grid` на `#chat`, `#sheld`, `.nest-shell` — это ломает внутренние расчёты высоты (`100dvh`, flex-based viewport).

### ❌ `vh` / `vw` юниты

Используй `dvh` / `svh` / `lvh` — они корректно реагируют на мобильную клавиатуру. `vh` → на iOS/Android чат «прыгает» когда появляется клавиатура.

### ❌ Скрывать интерактивные элементы

```css
/* Убьёт edit / swipe / delete кнопки */
.nest-msg-actions { display: none; }
```

Если решил скрыть — оставь доступ через контекстное меню или long-tap. Иначе user не сможет редактировать/перегенерировать сообщения.

---

## Дебаг

1. **DevTools → Console** (F12) — синтаксические ошибки CSS показываются там. В WuNest работает оффлайн-валидация через `CSSStyleSheet.replaceSync()`: если в CSS есть `}` без `{` — под textarea'ой сразу покажется красная полоска с текстом ошибки, **до** сохранения.

2. **Посмотри injected CSS:** в DevTools найди в `<head>` тег `<style id="nest-user-css">`. Видно, как именно твой CSS скопирован (включая scope-обёртку).

3. **Specificity wars:** `DevTools → Inspector → Computed` → hover по правилу → видно, что его «перебивает».

4. **Тема работает локально, ломается после релоада:** CSS пишется в localStorage + на сервер (debounce 400 ms). Подожди секунду после последнего символа перед F5. Индикатор сохранения — `«saving…»` в углу панели Appearance.

5. **Разное поведение в Chrome / Firefox:** Chromium/Safari используют native `@scope`, Firefox — ручной prefixer. Селекторы с `&` и CSS nesting могут вести себя по-разному. Для публичных тем — без nesting.

6. **Mobile:** проверь на окне < 960 px (DevTools → Device toolbar). Мобильный breakpoint у WuNest — один (960 px). Всё, что ниже — одна колонка.

7. **Playground:** в `frontend/src/styles/design-system/playground.html` есть standalone-страница с полным набором compоnentов. Открой её локально (без WuNest) → напиши CSS в любом Editor → быстрая итерация без сохранения на сервер.

---

## Защитные механизмы

### Safe mode

Тема сломала шелл так, что нельзя открыть Settings? **Добавь `?safe` в URL** → твой CSS и фоновая картинка не применятся, появится жёлтый баннер «Safe mode: custom CSS отключён». Нажми «Clear CSS» или отредактируй — выходи из safe mode перезагрузкой без параметра.

Safe mode есть в меню аватара: **Avatar → Безопасный режим**.

См. [подробно](/docs/safe-mode).

### Валидация при импорте

При импорте ST JSON:

- Scope автоматически ставится в `chat` (даже если в теме были broad selectors — они скопятся).
- Показывается notice: «Импортирована тема с N опасными селекторами — они применились только к чату».

При импорте `.css` файла — аналогично. Парсер проверяет синтаксис, предупредит о проблемах.

---

## Совместимость с SillyTavern

| Что работает | Что не работает |
|---|---|
| `--SmartTheme*` переменные | `.drawer-content > *` с проприетарными контейнерами ST |
| `.mes`, `.mes_block`, `.mes_text`, `.mes_name` | `.mes .avatar` с ST-специфичной вложенностью |
| `#chat`, `#send_form`, `#send_textarea`, `#send_but` | `#expression-image`, `#bg1` (wallpaper system ST) |
| `#top-bar`, `.topbar`, `#leftNavPanel`, `#sheld` | ST-specific `.drawer-content` layout rules |
| `@import`, `@font-face`, `@keyframes` | Прямые правки `body { background }` в scope `chat` |
| `[style*="...."]` attribute selectors | `window.extension_settings.*` (нет такого API) |

Импорт/экспорт ST-темы — через **Внешний вид → Импорт / Экспорт** (JSON формат ST).

---

## Публикация темы

Пока в WuNest нет публичной галереи, но формат готов к шарингу:

1. **Export `.css`** (Внешний вид → Экспорт в .css) сохранит твою тему как standalone-файл.
2. **Или JSON** — сохраняет CSS + все UI-настройки (размер шрифта, ширина чата, и пр.).
3. Поделись файлом — получатель импортирует через Внешний вид → Импорт.

### Хорошая тема:

- **Уровень 1 или 2** (переменные), минимум hard-coded селекторов.
- Работает в scope `chat` — не трогает топбар/sidebar без надобности.
- **Явное имя и автор** в первом комментарии:
  ```css
  /* Name: Purple Tavern */
  /* Author: you */
  /* Version: 1.0 */
  /* License: MIT */
  ```
- Проверена на **mobile** (< 960 px) и **desktop**.
- Проверена на обеих kind'ах (если тема light — проверь что совместима с dark-content; если dark — наоборот).
- Graceful degradation: если твоя тема использует `backdrop-filter`, добавь fallback для старых браузеров.
- Без внешних ассетов большого размера (шрифты с CDN — ок, картинки > 500 KB — не ок).

### Планируется:

- Внутренняя галерея пользовательских тем (M43+) — upload, rating, install в один клик.
- Theme CLI — валидатор, линтер, автоматические скриншоты.
