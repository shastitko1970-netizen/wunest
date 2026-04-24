# CSS Mods Guide

Это руководство для **пользователя**, который хочет сделать свою тему WuNest. Открой Settings → Appearance → Custom CSS и вставь сюда снипеты.

---

## Быстрый старт

Самая быстрая тема — переопределить пять переменных:

```css
:root {
  --SmartThemeBlurTintColor: #0b0818;   /* главный фон */
  --SmartThemeChatTintColor: #130d15;   /* фон карточек и сообщений */
  --SmartThemeBorderColor:   #3c1e50;   /* границы */
  --SmartThemeBodyColor:     #f1d1ff;   /* текст */
  --SmartThemeQuoteColor:    #c485ff;   /* акцент: CTA, focus */
}
```

Сохрани → смотри, как меняется весь шелл. Без селекторов, без `!important`, без рисков сломать интерфейс.

---

## Как это работает

WuNest держит всю внешность на **CSS-переменных**. Твой CSS идёт в `<style id="nest-custom-appearance-css">` в `<head>` и по умолчанию оборачивается в `@scope(#chat)` — всё, что ты написал, применяется **только внутри области чата**.

Два режима scope:

| Режим | Что захватывает | Когда выбирать |
|---|---|---|
| **Chat** (default) | Только `#chat` и его потомки | Безопасно: ST-темы, пробы, все правила с `body`, `textarea` не трогают меню |
| **Global** | Весь шелл | Когда тема реально должна перекрасить топбар, sidebar, dialogs |

Переключатель — прямо под textarea с CSS. Если в CSS есть опасные селекторы (`body`, `input`, `textarea`), система предупредит.

---

## Пять уровней модификации

### Уровень 1 — только переменные (рекомендуется)

```css
:root {
  --SmartThemeBlurTintColor: #1a0f1f;
  --SmartThemeChatTintColor: #241530;
  --SmartThemeQuoteColor:    #e07aff;
  --SmartThemeBodyColor:     #f0e5ff;
  --SmartThemeBorderColor:   #4a2a60;
}
```

Работает на всех будущих версиях WuNest. Не ломается при обновлениях.

### Уровень 2 — nest-токены напрямую

```css
:root {
  --nest-bg:             #0a0a0f;
  --nest-surface:        #141018;
  --nest-text:           #e8e0f0;
  --nest-accent:         #9d4edd;
  --nest-border:         #2a1f3a;
  --nest-radius:         16px;
  --nest-font-body:      'Inter', sans-serif;
}
```

Прямой доступ к внутренним токенам. Чуть больше контроля, но если мы переименуем токен — тема может перестать работать. Для стабильности предпочитай уровень 1.

### Уровень 3 — nest-классы

```css
.nest-msg {
  background: linear-gradient(180deg, #1a0f1f 0%, #0f0a14 100%);
  border: 1px solid #3a2050;
}

.nest-msg-name {
  color: #c485ff;
  letter-spacing: 0.02em;
}

.nest-msg-body {
  font-family: 'Fraunces', serif;
  line-height: 1.7;
}
```

Цепляешься за стабильные классы `.nest-*`. Их список и обещания — в **SELECTOR_CONTRACT.md**.

### Уровень 4 — ST-алиасы

```css
.mes { border-radius: 20px; }
.mes_name { font-style: italic; }
.mes_text { font-size: 16px; }
#send_textarea { background: #1a0f1f; color: #f1d1ff; }
#chat { padding: 24px; }
```

Если ты импортируешь тему из SillyTavern — они уже написаны так. Работает.

### Уровень 5 — data-атрибуты

```css
[data-nest-chat-display='document'] .nest-msg-body {
  font-family: 'Fraunces', serif;
  font-size: 17px;
  line-height: 1.75;
}

[data-nest-avatar-style='portrait'] .nest-msg-avatar {
  border-radius: 8px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
}
```

Позволяет реагировать на режим. Например: только для document-режима рендерь body как editorial-текст.

---

## Рецепты

### Кастомный шрифт

```css
@import url('https://fonts.googleapis.com/css2?family=Andika&display=swap');

:root {
  --SmartThemeBodyFont: 'Andika';
}
```

`@import` система автоматически поднимает в начало — это работает. Если шрифт свой:

```css
@font-face {
  font-family: 'MyFont';
  src: url('https://my-cdn.com/my-font.woff2') format('woff2');
  font-display: swap;
}

:root { --SmartThemeBodyFont: 'MyFont'; }
```

### Сообщение с «свечением»

```css
.mes {
  background: rgba(30, 15, 45, 0.65);
  border: 1px solid #3c1e50;
  border-radius: 14px;
  box-shadow: 0 2px 14px rgba(120, 60, 200, 0.2);
  backdrop-filter: blur(6px);
}
```

### Пергамент/таверна

```css
:root {
  --SmartThemeBlurTintColor: #1a130a;
  --SmartThemeChatTintColor: #2a1f12;
  --SmartThemeBodyColor:     #f4e4c1;
  --SmartThemeBorderColor:   #6b4a2a;
  --SmartThemeQuoteColor:    #d4904a;
  --SmartThemeBodyFont:      'Fraunces';
}

.mes {
  border-radius: 4px;
  box-shadow: 0 1px 0 rgba(0,0,0,0.3), inset 0 1px 0 rgba(255,220,160,0.05);
}
```

### Скрыть timestamps

```css
.nest-msg-time { display: none; }
```

(scope = chat — глобальный топбар не тронешь.)

---

## Чего избегать

### ❌ Правила на `body`/`html` в global scope

```css
/* СЛОМАЕТ топбар и меню */
body { background: black !important; }
```

В chat-scope такие правила игнорируются системой (они «глобальные» по спеку, их выносят наверх без оборачивания). Но если включишь Global scope — сломается.

**Альтернатива:** меняй переменные.

### ❌ `!important` без нужды

Ты конкурируешь с самим собой при обновлениях. Используй `!important` только если токен не помогает (например, нужно переопределить `transition: none`).

### ❌ Абсолютные позиции на ключевых элементах

```css
/* НЕ делай: сломает мобильную раскладку */
#chat { position: absolute !important; top: 100px; }
```

### ❌ Блоки по ширине в px

```css
/* Плохо: ломает мобильный */
.mes { width: 600px !important; }

/* Хорошо */
.mes { max-width: 600px; width: 100%; }
```

### ❌ Изменение `display` у ключевых контейнеров

Не ставь `display: block`/`flex`/`grid` на `#chat`, `#sheld`, `.nest-shell` — это ломает внутренние расчёты высоты.

---

## Защитные механизмы

### Safe mode

Тема сломала шелл так, что нельзя открыть Settings?

**Добавь `?safe` в URL** → твой CSS и фон не применятся, появится жёлтый баннер «Safe mode: custom CSS отключён». Нажми «Clear CSS» или отредактируй — выходи из safe mode.

Safe mode есть в меню аватара: **Avatar → Safe mode**.

### Audit

Под textarea система в реальном времени считает «опасные селекторы» (`body`, `html`, `textarea`, `input`, `.menu_button`, и т.п.). Если таких больше нуля и ты включил Global scope — появится предупреждение.

### Import ST JSON

Если импортируешь SillyTavern тему (`.json`):
- Scope автоматически ставится в `chat` (даже если в теме были broad selectors — они скопятся).
- Показывается notice: «Импортирована тема с N опасными селекторами — они применились только к чату».

---

## Совместимость с SillyTavern

| Что работает | Что не работает |
|---|---|
| `--SmartTheme*` переменные | `.drawer-content > *` с проприетарными контейнерами ST |
| `.mes`, `.mes_block`, `.mes_text`, `.mes_name` | `.mes .avatar` — у нас структура чуть другая |
| `#chat`, `#send_form`, `#send_textarea`, `#send_but` | `#expression-image`, `#bg1` (wallpaper system) |
| `#top-bar`, `.topbar`, `#leftNavPanel`, `#sheld` | `.drawer-content` специфичные layout-правила |
| `@import`, `@font-face`, `@keyframes` | Прямые правки `body { background }` в scope=chat |

Импорт через **Appearance → Import** поддерживает формат ST theme JSON. Экспорт в `.json` доступен там же.

---

## Дебаг

1. **Открой DevTools** (F12) → Console: ошибки парсинга CSS покажутся там.
2. Найди в `<head>` `<style id="nest-custom-appearance-css">` — видно, как именно CSS был скопирован.
3. Проверь selector specificity: `Inspector → Computed` → hover по правилу → «Overridden by».

### Тема работает локально, ломается после релоада

Причина: тема пишется в localStorage + на сервер (debounce 400ms). Подожди сек после последнего ввода перед релоадом. Индикатор сохранения — `«saving…»` в углу Appearance.

### Тема выглядит по-разному в двух браузерах

Chromium/Safari используют нативный `@scope`, Firefox — ручной prefixer. Селекторы типа `&` и CSS nesting могут вести себя по-разному. Избегай nesting в публичных темах.

---

## Публикация темы

Пока в WuNest нет публичной галереи, но формат готов:
1. Export (`.json`) сохранит всю твою конфигурацию + CSS.
2. Поделись файлом с другом — он импортирует через Appearance → Import.
3. Планируется: внутренняя галерея тем. Формат не изменится.

**Хорошая тема**:
- Уровень 1 или 2 (переменные), без hard-coded селекторов.
- Работает в scope=chat.
- Явное имя в первом комментарии:
  ```css
  /* Name: Purple Tavern */
  /* Author: you */
  ```
- Проверена на mobile (`< 960px`) и desktop.
