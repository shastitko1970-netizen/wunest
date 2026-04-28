# Customization Surface

Полный список того, что пользователь может менять во WuNest. Вдохновлено JSON-профилями SillyTavern (`main_text_color`, `blur_strength`, `avatar_style`, `chat_display`, `custom_css` и т.д.) — но приведено к единой token-first архитектуре, без разрозненных полей.

**Правило:** всё, что видно на экране, должно быть либо CSS-переменной, либо селектором из `SELECTOR_CONTRACT.md`. Никаких inline-стилей в компонентах, никаких hardcoded цветов, размеров, радиусов, шрифтов, иконок, фонов.

---

## 1. Что можно менять (inventory)

| Группа | Что | Как |
|---|---|---|
| **Фон** | `body`, `#chat`, `.nest-msg`, loading screen | `--nest-bg`, `--nest-bg-image`, `--nest-bg-blur` |
| **Цвет текста** | main, italic, underline, quote, muted | `--nest-text`, `--nest-text-italic`, `--nest-text-underline`, `--nest-accent`, `--nest-text-muted` |
| **Сообщения** | user bubble, bot bubble, отдельные тинты | `--nest-msg-user-bg`, `--nest-msg-bot-bg`, `--nest-msg-border` |
| **Тип** | body font, display font, mono, scale | `--nest-font-body`, `--nest-font-display`, `--nest-font-mono`, `--nest-font-scale` |
| **Блюр/тени** | глубина размытия, цвет тени, ширина | `--nest-blur`, `--nest-shadow-color`, `--nest-shadow-width` |
| **Рамки** | цвет и радиус | `--nest-border`, `--nest-radius`, `--nest-radius-lg` |
| **Аватары** | круг / квадрат / портрет | `[data-nest-avatar-style="round\|square\|portrait"]` |
| **Режим чата** | bubbles / flat / document | `[data-nest-chat-display="bubbles\|flat\|document"]` |
| **Ширина чата** | % от viewport | `--nest-chat-width` |
| **Кнопки** | все primary/secondary/ghost/icon | `.nest-btn` + вариации через `--nest-btn-*` |
| **Toggle/switch** | цвет track, thumb, размер | `--nest-toggle-*` |
| **Слайдеры** | track, thumb (можно картинкой!) | `--nest-slider-*`, `--nest-slider-thumb-image` |
| **Логотип** | текст или картинка | `--nest-logo-image`, `--nest-logo-text` |
| **Loading screen** | фон, спиннер, текст | `.nest-loader`, `--nest-loader-*` |
| **Top-bar** | фон, картинка-оверлей | `#top-bar::before` через `--nest-topbar-image` |
| **Decorations** | картинки вокруг сообщений (как в Lilac Witch) | `.nest-msg::before`, `.nest-msg::after` + `--nest-msg-deco-*` |
| **Density** | comfortable / compact | `[data-nest-density="comfortable\|compact"]` |
| **Motion** | вкл/выкл анимации | `[data-nest-reduced-motion]` |
| **Scrollbar** | ширина, цвет track/thumb, радиус | `--nest-scrollbar-*` |
| **Cursor** | default / pointer / text (картинка!) | `--nest-cursor-default`, `--nest-cursor-pointer`, `--nest-cursor-text` |
| **Sounds** | send, receive, notification, error | `--nest-sound-*` (URL) + JS-хук |

---

## 2. Единый язык кнопок

> ⚠️ **ASPIRATIONAL — не реализовано (M51 audit, 2026-04-25)**: целевая форма `.nest-btn` системы. В коде сейчас используется `<v-btn>` от Vuetify. Этот раздел — design intent для будущего отдельного пакета `.nest-btn`, который заменит Vuetify-button'ы или сосуществует с ними. Не пиши тему опираясь на эти классы — их нет на DOM'е.

Все кнопки в продукте — один класс-корень `.nest-btn` + модификаторы. Это закрывает сразу две проблемы: (а) пользователь меняет `--nest-btn-*` один раз, темизуется всё; (б) разработчик не изобретает новые стили в каждом компоненте.

```html
<button class="nest-btn nest-btn--primary">Send</button>
<button class="nest-btn nest-btn--secondary">Cancel</button>
<button class="nest-btn nest-btn--ghost">Skip</button>
<button class="nest-btn nest-btn--danger">Delete</button>
<button class="nest-btn nest-btn--icon" aria-label="Reload">↻</button>
<button class="nest-btn nest-btn--pill">Filter</button>
```

```css
.nest-btn {
  --_bg: var(--nest-btn-bg, transparent);
  --_fg: var(--nest-btn-fg, var(--nest-text));
  --_border: var(--nest-btn-border, var(--nest-border));
  --_radius: var(--nest-btn-radius, var(--nest-radius-sm));

  font-family: var(--nest-font-body);
  font-size: 14px;
  font-weight: 500;
  padding: 8px 16px;
  border-radius: var(--_radius);
  border: 1px solid var(--_border);
  background: var(--_bg);
  color: var(--_fg);
  transition: all var(--nest-transition-fast);
  cursor: pointer;
}

.nest-btn--primary { --_bg: var(--nest-accent); --_fg: #fff; --_border: var(--nest-accent); }
.nest-btn--ghost   { --_border: transparent; --_fg: var(--nest-text-secondary); }
.nest-btn--danger  { --_fg: var(--nest-accent); --_border: var(--nest-accent); }
.nest-btn--icon    { width: 32px; height: 32px; padding: 0; }
.nest-btn--pill    { --_radius: var(--nest-radius-pill); }
```

Пользователь-модер переопределяет только переменные: `--nest-btn-radius: 0`, `--nest-btn-border: #ff00ff` — все кнопки в продукте меняют форму и цвет без единого селектора.

---

## 3. Единый toggle

> ⚠️ **ASPIRATIONAL — не реализовано**: `.nest-toggle` пока не существует. Все toggle'ы в WuNest — `<v-switch>` Vuetify. Раздел оставлен как design intent.

```html
<label class="nest-toggle">
  <input type="checkbox">
  <span class="nest-toggle-track"><span class="nest-toggle-thumb"></span></span>
  <span class="nest-toggle-label">Blur enabled</span>
</label>
```

Переменные: `--nest-toggle-track-off`, `--nest-toggle-track-on`, `--nest-toggle-thumb`, `--nest-toggle-width` (дефолт 36px), `--nest-toggle-height` (20px).

---

## 4. Логотип

Логотип — **не SVG в компоненте**, а переменная. Это позволяет пользователю подставить свой.

```css
.nest-logo {
  width: var(--nest-logo-size, 32px);
  height: var(--nest-logo-size, 32px);
  background: var(--nest-logo-image, none) center/contain no-repeat;
  color: var(--nest-accent);
  font-family: var(--nest-font-display);
  font-size: calc(var(--nest-logo-size, 32px) * 0.75);
  display: inline-flex; align-items: center; justify-content: center;
}
.nest-logo::before { content: var(--nest-logo-text, "▲"); }
.nest-logo[style*="--nest-logo-image"]::before { content: none; }
```

Мод:
```css
:root {
  --nest-logo-image: url('https://example.com/my-logo.svg');
}
```

Или через текст:
```css
:root { --nest-logo-text: "♛"; }
```

---

## 5. Loading screen (инициализация)

> ⚠️ **ASPIRATIONAL — не реализовано**: `<NestLoader>` компонента не существует. Сейчас при загрузке SPA Vue показывает пустой `<div id="app">` ~50ms пока mount'ится bundle, никакого custom loader'а. Раздел — design intent.

Критически важно: экран загрузки **тоже кастомизируется**, потому что это первое, что видит пользователь. Компонент `<NestLoader>` живёт в `index.html` ДО монтирования Vue — поэтому на нём работает тот же набор CSS-переменных из `tokens/colors_and_type.css` (подключён первым ресурсом).

```html
<div class="nest-loader">
  <div class="nest-loader-bg"></div>
  <div class="nest-loader-mark"></div>
  <div class="nest-loader-text">Инициализация…</div>
  <div class="nest-loader-progress"><div class="nest-loader-bar"></div></div>
</div>
```

Переменные:
- `--nest-loader-bg` — фон (по умолчанию `var(--nest-bg)`)
- `--nest-loader-bg-image` — опционально картинка
- `--nest-loader-mark` — URL иконки/картинки (по умолчанию логотип)
- `--nest-loader-accent` — цвет прогресс-бара
- `--nest-loader-text-color` — цвет текста

---

## 6. Backgrounds (фоны)

Три уровня, все управляются переменными:

```css
body          { background: var(--nest-bg) var(--nest-bg-image, none) center/cover no-repeat fixed; }
#chat         { background: var(--nest-chat-bg, transparent) var(--nest-chat-bg-image, none); backdrop-filter: blur(var(--nest-blur)); }
.nest-msg     { background: var(--nest-msg-bg); }
```

Мод задаёт картинку:
```css
:root {
  --nest-bg-image: url('https://i.postimg.cc/xyz/forest.jpg');
  --nest-blur: 6px;
  --nest-chat-bg: rgba(20, 10, 30, 0.6);
}
```

---

## 7. Декорации сообщений (ST-стиль)

Позволяем вешать картинки на `::before` / `::after` у сообщения — как в Lilac Witch делают ангельские/демонические рамки. Делаем это **вариативно**, без хардкода:

```css
.nest-msg::before {
  content: "";
  position: absolute;
  top: var(--nest-msg-deco-top, -130px);
  left: var(--nest-msg-deco-left, -2%);
  width: var(--nest-msg-deco-width, 110%);
  height: var(--nest-msg-deco-height, 270px);
  background: var(--nest-msg-deco-image, none) var(--nest-msg-deco-position, center) / var(--nest-msg-deco-size, contain) no-repeat;
  pointer-events: none;
  z-index: 2;
}
.nest-msg.is-user::before   { background-image: var(--nest-msg-deco-image-user,   var(--nest-msg-deco-image)); }
.nest-msg.is-assistant::before { background-image: var(--nest-msg-deco-image-bot, var(--nest-msg-deco-image)); }
```

Без мода — ничего не рисуется (none). С модом — можно повторить Lilac Witch за 4 строки.

---

## 8. Профиль темы как JSON (ST-совместимость)

> ⚠️ **STATUS NOTE (M51, 2026-04-25)** — этот раздел частично описывает целевое состояние, не текущее. Актуальный mapping см. в [`frontend/src/docs/pages/theming.ru.md`](../../../docs/pages/theming.ru.md) ("JSON-схема"). Здесь — расширенная таблица, к которой стремимся.

Помимо прямого CSS, поддерживаем импорт **JSON-профилей**. Цель — конвертировать ST-поля в наши переменные:

```
main_text_color         → --nest-text                    [✅ M42.4]
italics_text_color      → --nest-text-italic             [✅ M51 Sprint 1 wave 2 — wired в applyAppearance]
underline_text_color    → --nest-text-underline          [❌ не реализовано]
quote_text_color        → --nest-text-quote              [✅ M51 Sprint 1 wave 2 — wired (отдельное поле quoteColor, не italicsColor)]
blur_tint_color         → --nest-bg (rgba OK)            [❌ дропается на импорт]
chat_tint_color         → --nest-chat-bg                 [❌ не реализовано]
user_mes_blur_tint_color → --nest-msg-user-bg            [❌ не реализовано]
bot_mes_blur_tint_color  → --nest-msg-bot-bg             [❌ не реализовано]
shadow_color            → --nest-shadow-color            [❌ не реализовано]
shadow_width            → --nest-shadow-width (px)       [❌ не реализовано]
border_color            → --nest-border + --nest-accent  [✅ M42.4 — auto-promote в accent]
blur_strength           → --nest-blur (px)               [✅ M42.4]
font_scale              → --nest-chat-font-scale         [✅ M42.4 — chat-only scope]
avatar_style            → [data-nest-avatar-style]       [✅ M42.4 — 0=round, 1=square, 2=portrait]
chat_display            → [data-nest-chat-display]       [✅ M42.4 — 0=flat, 1=bubbles, 2=document]
chat_width              → --nest-chat-width (%)          [✅ M42.4]
reduced_motion          → [data-nest-reduced-motion]     [✅ M42.4]
custom_css              → inject в <style> через scope-engine  [✅ M42.4]
```

**Реальный парсер живёт в `frontend/src/api/appearance.ts`** — `fromST()` около строки 139, `toST()` около строки 184. НЕ в `frontend/src/lib/stProfileImport.ts` — этого файла не существует.

Неизвестные поля игнорируются молча. Числа за пределами диапазона клампятся. Невалидные цвета браузер отбрасывает на парсинге.

---

## 9. Экспорт

> ⚠️ **STATUS NOTE (M51, 2026-04-25)** — `nest_ext` блок ниже сейчас НЕ эмитится `toST()` (`api/appearance.ts:136`). Целевое состояние, не текущее. Текущий экспорт — обычная ST-форма без расширений (для max совместимости).

Кнопка "Export theme" в Settings → Appearance собирает все текущие значения CSS-переменных + custom_css в JSON. Профиль переносится на другое устройство или делится ссылкой. Целевая схема — как у ST (для совместимости с чужими пиками), плюс наш блок `nest_ext`:

```json
{
  "name": "My theme",
  "main_text_color": "rgba(...)",
  "...": "...",
  "nest_ext": {
    "logo_image": "url(...)",
    "loader_accent": "#ff00ff",
    "msg_deco_image_bot": "url(...)",
    "density": "compact"
  }
}
```

(Будет реализовано когда введём настоящий WuNest theme registry — Sprint 3 в M51-плане.)

---

## 10. Scrollbar

Скроллбар — тоже поверхность бренда. Стилизуем через `::-webkit-scrollbar` + fallback `scrollbar-color` для Firefox. Все параметры — через переменные:

```css
* {
  scrollbar-width: var(--nest-scrollbar-width-firefox, thin);
  scrollbar-color: var(--nest-scrollbar-thumb) var(--nest-scrollbar-track);
}
*::-webkit-scrollbar        { width: var(--nest-scrollbar-width, 10px); height: var(--nest-scrollbar-width, 10px); }
*::-webkit-scrollbar-track  { background: var(--nest-scrollbar-track, transparent); }
*::-webkit-scrollbar-thumb  {
  background: var(--nest-scrollbar-thumb, var(--nest-border));
  border-radius: var(--nest-scrollbar-radius, var(--nest-radius-pill));
  border: var(--nest-scrollbar-thumb-border, 2px solid var(--nest-bg));
}
*::-webkit-scrollbar-thumb:hover { background: var(--nest-scrollbar-thumb-hover, var(--nest-accent)); }
```

Мод может спрятать скроллбар (`--nest-scrollbar-width: 0`), сделать его жирным акцентом (`--nest-scrollbar-thumb: var(--nest-accent)`) или заменить картинкой-градиентом через `background-image`.

---

## 11. Cursor

> ⚠️ **ASPIRATIONAL — не реализовано**: `--nest-cursor-*` переменные на DOM не выставляются. Раздел — design intent.

Курсор — сильный элемент темы (особенно в fantasy / cyber направлениях). Три роли, все переопределяются URL'ом:

```css
html                              { cursor: var(--nest-cursor-default, auto); }
a, button, .nest-btn, [role="button"], .nest-clickable
                                  { cursor: var(--nest-cursor-pointer, pointer); }
input, textarea, [contenteditable]{ cursor: var(--nest-cursor-text, text); }
```

Мод:
```css
:root {
  --nest-cursor-default: url('https://example.com/sword.png') 0 0, auto;
  --nest-cursor-pointer: url('https://example.com/sword-glow.png') 0 0, pointer;
}
```

Fallback на системный курсор обязателен (вторым значением после URL) — иначе при 404 курсор пропадает.

---

## 12. Sounds

> ⚠️ **ASPIRATIONAL — не реализовано**: ни `frontend/src/lib/sounds.ts`, ни события message-life-cycle для звуков не существуют. Раздел — design intent.

Звуки — не CSS, но часть темы. Подход такой: в JSON-профиле декларируем URL'ы, фронт в `frontend/src/lib/sounds.ts` слушает события (`message:sent`, `message:received`, `notification`, `error`) и проигрывает `new Audio(url).play()`.

```json
{
  "nest_ext": {
    "sound_send":         "https://example.com/send.mp3",
    "sound_receive":      "https://example.com/receive.mp3",
    "sound_notification": "https://example.com/ding.mp3",
    "sound_error":        "https://example.com/error.mp3",
    "sound_volume":       0.5
  }
}
```

Правила:
- Громкость по умолчанию — 0, пользователь включает явно. Мод-автор **не может** заставить проигрывать звук без согласия.
- Settings → Appearance → Sounds показывает preview-кнопку для каждого звука.
- Поддерживаются только `.mp3`, `.ogg`, `.wav`. CDN-URL обязательно `https://`.
- Длина файла ограничена 5 секунд на клиенте — иначе труется.

---

## 13. Чеклист для нового UI-элемента

Когда добавляешь новую кнопку / toggle / слайдер — пройди пять вопросов:

1. **Есть ли готовый `.nest-*` класс?** (`.nest-btn`, `.nest-toggle`, `.nest-slider`) Если да — используй его + модификатор.
2. **Все значения — через `var(--nest-*)`?** Ни одного hex, ни одного px без переменной для ключевых параметров.
3. **Состояния через `.is-*` или `data-nest-*`?** Не через дополнительные классы с hardcoded значениями.
4. **Есть ли `--nest-<элемент>-*` переменная для главного параметра?** Если новый компонент — добавь её в `tokens/colors_and_type.css`.
5. **Работает в 3 темах?** Проверь в dark, light, tavern (это покрывает все крайние случаи палитры).

Если хоть один «нет» — не мержим.
