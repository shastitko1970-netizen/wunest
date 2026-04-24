# Adaptive Rules

Один код — три контекста: **desktop browser**, **mobile browser**, **mobile app** (webview-обёртка того же фронта). Правила ниже гарантируют, что все три ощущаются «нативно».

---

## 1. Брейкпоинт

**Единственный primary breakpoint: `960px`.**

```css
@media (max-width: 959.98px) { /* mobile web + mobile app */ }
@media (min-width: 960px)    { /* desktop */ }
```

Вторичные (только для дотюна):
- `1100px` — collapse плотности chip-ов в топбаре.
- `640px` — grid two-column → one-column в Appearance.
- `520px` — скрываем second-line хинты в CSS-editor header.

**Не заводи новые брейкпоинты без обсуждения.** Если UI не влезает — проблема в компоненте, не в шкале.

### Почему 960

Vuetify `md` грид-точка. Совпадает с физической шириной, на которой пропадает удобный хит-тест hover-актоинов. Ниже — тач-first поведение, выше — hover-first.

---

## 2. Три контекста, одна кодовая база

| Контекст | Как определяется | Ключевые отличия |
|---|---|---|
| **Desktop browser** | `matchMedia('(min-width: 960px)').matches === true` | Топнав в v-app-bar, hover-actions, Ctrl+K |
| **Mobile browser** | `matchMedia('(min-width: 960px)').matches === false` | Overlay-drawer, always-visible actions, chat-width 100% |
| **Mobile app** | всё то же + `navigator.standalone` ИЛИ webview user-agent | + safe-area insets, IME-aware высоты, блок pull-to-refresh в чате |

### Детекция mobile app

```ts
export const isMobileApp = (() => {
  if (typeof navigator === 'undefined') return false
  // iOS PWA / Capacitor
  if ((navigator as any).standalone) return true
  // Android webview (Capacitor/Cordova добавляют маркер в UA)
  if (/wv|wunest-app/i.test(navigator.userAgent)) return true
  return false
})()
```

Маркер пишется в `<html data-nest-platform="app">` на старте — моды могут целиться через `:root[data-nest-platform='app'] .nest-topbar { ... }`.

---

## 3. Safe areas (mobile app)

**Правило:** любой `position: fixed` элемент, касающийся края экрана, обязан учитывать safe-area.

```css
.nest-topbar {
  padding-top: env(safe-area-inset-top, 0);
}

/* Композер прижат к низу — нужен нижний inset */
#send_form {
  padding-bottom: calc(var(--nest-space-3) + env(safe-area-inset-bottom, 0));
}

/* Drawer слева */
#leftNavPanel {
  padding-left: env(safe-area-inset-left, 0);
}
```

Для динамической IME-высоты используй `100dvh`, не `100vh`. `100vh` на мобильном Safari считает высоту **без** адресной строки и ломает раскладку при скролле.

---

## 4. Правила раскладки по контекстам

### Топбар

| Breakpoint | Содержимое |
|---|---|
| ≥ 960 | Logo · TopNav (Chat/Library/Docs) · Spacer · Gold chip · Quota chip · Theme toggle · Lang · Avatar menu |
| < 960 | Burger · Logo · Spacer · Theme toggle · Lang · Avatar menu |

Burger **рендерится только на мобильном** (`v-if="!isDesktop"`) — не полагайся на `hidden` класс, это убирает элемент из focus-trap у drawer.

### Nav

- **Desktop:** плоские кнопки в v-app-bar (`.nest-topnav`). Sidebar **не рендерится**.
- **Mobile:** overlay `v-navigation-drawer` с `temporary` + burger.

### Чат

| Свойство | Desktop | Mobile |
|---|---|---|
| max-width сообщения | `var(--nest-chat-width)` (60% default) | `100%` |
| actions сообщения | `position: absolute`, появляются на hover | `position: static`, всегда видны |
| composer | inline внизу `#sheld` | `position: sticky; bottom: 0` с safe-area |
| swipe navigator | chevrons + count | chevrons + count, **крупнее** (min hit 44px) |

Правило для actions в MessageBubble:

```css
@media (hover: none) {
  .nest-msg-actions {
    opacity: 1;
    pointer-events: auto;
    position: static;
    margin-top: 4px;
    justify-content: flex-end;
  }
}
```

`(hover: none)` лучше, чем width-медиа — ловит touch-only ноуты и планшеты тоже.

### Dialogs / Drawers

- Desktop: центрированный модал, max-width 600px.
- Mobile: full-screen (`fullscreen` prop у `v-dialog`). Заголовок с кнопкой закрыть в левом верхнем, кнопка-действие в правом верхнем.

### Settings и Account

- Desktop: 2-колоночный grid (`grid-template-columns: 1fr 1fr`), gap 18px.
- Mobile (`< 640px`): 1 колонка.

---

## 5. Хит-таргеты

- **Touch:** минимум **44 × 44 px**. Icon-кнопки получают `padding: 10px` вокруг иконки, даже если иконка 14px.
- **Desktop:** 32 × 32 минимум для icon-кнопок, 36 high для кнопок с текстом.

Composer send-button (`#send_but`) всегда 44×44 независимо от платформы — это primary action.

---

## 6. Клавиатура и фокус

- **Cmd/Ctrl+K** — глобальный поиск чатов (десктоп; на мобильном не мешает, IME-guarded через `e.isComposing`).
- **Esc** — закрывает dialog, drawer, cancel edit в MessageBubble.
- **Cmd/Ctrl+Enter** — save в inline-edit сообщения.
- **Tab**-навигация должна обходить все интерактивные элементы без ловушек. `outline: none` — только если добавлен альтернативный focus-indicator (box-shadow или border-color).

---

## 7. IME и мультиязычность

- Всегда проверяй `e.isComposing` в keydown-хендлерах, прежде чем перехватывать клавишу. CJK-ввод в composer обязан работать.
- Не клади иконку правее input'а в RTL-контексте без `logical property` (`inset-inline-end`), но поскольку WuNest сейчас только EN/RU, RTL не блокирующий.

---

## 8. Плотность

Один уровень плотности на контекст. **Не** даём юзеру выбор «compact/comfortable».

- Desktop: `density="compact"` у таблиц и списков в Settings/Library, `density="comfortable"` у пунктов nav и диалогов.
- Mobile: `density="comfortable"` везде — пальцу нужен простор.

Font scale (0.75×–1.4×) — отдельная ось, доступная пользователю.

---

## 9. Картинки и медиа

- Аватар персонажа: ширина 40px в сообщении, 64px в списке, 128+ в профиле. Всегда `object-fit: cover`.
- Background image: `cover, center, fixed`. На mobile `fixed` не всегда работает (Safari) — fallback `scroll`. Браузер сам выбирает.
- Никаких `<video autoplay>` без пользовательского действия.

---

## 10. Контент

- Текст в кнопках топбара укорачивается на mobile: «Library» → иконка без текста. Через `v-if="isDesktop"` вокруг label, а не через CSS `display: none` (чтобы не мерцал при ресайзе).
- Длинные имена персонажей обрезаются `text-overflow: ellipsis` с max-width относительно контейнера, не абсолютным числом.

---

## 11. Тест-матрица

Любое изменение в AppShell / Chat.vue / MessageBubble проверяй минимум на:

- iPhone 12 (390 × 844) — mobile app + mobile Safari.
- Pixel 6 (412 × 915) — Android Chrome.
- iPad Mini (768 × 1024) — **считается mobile** (< 960). Проверь, что drawer не ломается.
- 1280 × 800 — laptop.
- 1920 × 1080 — desktop.

Тест: поверни iPad mini в landscape → станет 1024 > 960 → layout должен переключиться на desktop без перезагрузки.

---

## 12. Правила для модеров (адаптив)

Пользовательский CSS **не должен** нарушать адаптив. В CSS_MODS_GUIDE описан безопасный паттерн, но для напоминания:

```css
/* ПЛОХО — замедляет драг слайдера, роняет мобильную раскладку */
.mes { width: 600px !important; }

/* ХОРОШО — масштабируется */
.mes { max-width: 600px; width: 100%; }

/* ОЧЕНЬ ХОРОШО — уважает пользовательскую настройку ширины чата */
.mes { max-width: var(--nest-chat-width, 60%); }
```
