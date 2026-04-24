# Developer Guide

Как писать компоненты WuNest так, чтобы пользовательские CSS-моды с ними работали, и никто не боялся обновлений.

---

## 1. Принципы

1. **Всё красится через переменные.** Hex-коды только внутри `tokens/colors_and_type.css` и `global.scss`.
2. **Состояние — это атрибут.** `is-loading`, `is-error`, `is-hidden`, `data-nest-*` — не inline-стили.
3. **Якорь ≠ стиль.** Публичный класс `.nest-msg` — контракт. Styling делается в scoped-стилях Vue + через токены. Публичный класс не несёт `flex-direction: ...` как единственное правило.
4. **ST-алиас там, где это главный блок.** Chat, message, composer, top-bar — да. Внутренние детали — нет.
5. **Адаптив — в одном месте.** Компонент сам знает, как вести себя на mobile. Брейкпоинт `960px`, вторичные — только если обоснованы.

---

## 2. Шаблон компонента

```vue
<script setup lang="ts">
// Минимальный импорт. Vue reactivity + i18n + типы API. Никаких локальных
// цветов/размеров — всё идёт через CSS-переменные.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const props = defineProps<{
  title: string
  variant?: 'default' | 'emphasis'
  /** Скрывает компонент без анмаунта — уважает фокус/анимации. */
  hidden?: boolean
}>()

const variantClass = computed(() => `is-${props.variant ?? 'default'}`)
</script>

<template>
  <!-- Корень:
       1. Класс `.nest-<component>` — стабильный якорь.
       2. ST-алиас если это главный блок (здесь не нужен).
       3. Модификаторы через `is-*` (не через отдельные классы).
       4. data-* если нужно состояние, на которое могут завязаться моды. -->
  <section
    class="nest-notice"
    :class="[variantClass, { 'is-hidden': hidden }]"
    role="status"
  >
    <h3 class="nest-notice-title nest-h3">{{ title }}</h3>
    <p class="nest-notice-body nest-subtitle">
      <slot />
    </p>
  </section>
</template>

<style lang="scss" scoped>
.nest-notice {
  padding: var(--nest-space-4);
  background: var(--nest-surface);
  border: 1px solid var(--nest-border-subtle);
  border-radius: var(--nest-radius);
  box-shadow: var(--nest-shadow-sm);
  transition: border-color var(--nest-transition-fast);

  &:hover { border-color: var(--nest-border); }

  &.is-emphasis {
    border-color: var(--nest-accent);
    background: color-mix(in srgb, var(--nest-accent) 8%, var(--nest-surface));
  }

  &.is-hidden {
    opacity: 0.5;
    border-style: dashed;
  }
}

.nest-notice-title { margin: 0; }
.nest-notice-body  { margin: var(--nest-space-2) 0 0; }

// Мобильный: меньше padding, крупнее межстрочный.
@media (max-width: 959.98px) {
  .nest-notice { padding: var(--nest-space-3); }
}
</style>
```

Замечания:
- `scoped` у `<style>` — всегда. Исключение: если класс должен быть виден пользовательскому CSS (а он **должен** — `.nest-notice` публичный), scoped не ломает это: `.nest-notice` остаётся обычным классом, лишь внутренности компонента получают `[data-v-xxx]`.
- Не используй `:deep()` в скоупе `.nest-msg *` — это создаёт неявный контракт.

---

## 3. Правила про CSS

### ✅ Можно

- `var(--nest-bg)`, `var(--nest-accent)`, и т.д.
- `color-mix(in srgb, var(--nest-accent) 10%, transparent)` для тонированных фонов.
- `clamp()` для fluid-типографики.
- `@media (max-width: 959.98px)` — единственный обязательный брейкпоинт.
- `@media (hover: none)` — для touch-оптимизаций (actions visible always).
- Один уровень nesting в SCSS.

### ❌ Нельзя

- Хардкодить цвет/шрифт/радиус/спейсинг.
- Писать `!important`, кроме перекрытия Vuetify-дефолта в `global.scss`.
- Менять `display`, `position`, `width`, `height` у публичных якорей (`#chat`, `.nest-msg` и т.п.) в компонентах выше — это ломает моды.
- `position: fixed` без `env(safe-area-inset-*)`.
- `100vh` — всегда `100dvh`.
- Два и более уровня nesting в SCSS (`.a .b .c { ... }`).

---

## 4. Добавление state: data-атрибут vs class

**Правило:** если состояние меняет *структуру/режим* компонента — `data-*`. Если меняет *локальный вид* — `is-*` класс.

- `data-nest-chat-display='document'` на `:root` — меняет режим всего чата. `data-*`.
- `.nest-msg.is-user` — user-vs-assistant. `is-*`.
- `data-message-id='uuid'` — идентичность. `data-*`.
- `.nest-msg.is-streaming` — временное визуальное состояние. `is-*`.

Моды всегда могут зацепиться за `data-nest-*` и `is-*`. Нельзя менять семантику существующего `data-*` без обновления SELECTOR_CONTRACT.

---

## 5. Адаптив в компоненте

Не пиши отдельные mobile/desktop-компоненты. Один компонент — одна раскладка с медиазапросом.

```scss
.nest-thing {
  // desktop-first
  display: grid;
  grid-template-columns: 240px 1fr;
  gap: var(--nest-space-6);
}

@media (max-width: 959.98px) {
  .nest-thing {
    grid-template-columns: 1fr;
    gap: var(--nest-space-4);
  }
}
```

Если нужно принципиально другое поведение (не только размеры) — используй JS через `matchMedia('(min-width: 960px)')`, как в AppShell. Это исключение, не правило.

---

## 6. Доступность

- Каждая кнопка — либо `<button>`, либо `<v-btn>`, но не `<div @click>`.
- `aria-label` у icon-only кнопок.
- `role="status"` / `role="alert"` для баннеров.
- Focus indicator: если заменяешь дефолтный outline, поставь альтернативу (border-color или box-shadow).
- Не делай анимацию обязательной для понимания (reducedMotion выключит её).

---

## 7. i18n

Все строки — через `t('key')`. Новые строки — добавлять в `src/plugins/i18n.ts` (RU + EN одновременно). Пустой placeholder или hardcoded string в UI — блокер для PR.

---

## 8. Как НЕ сломать пользовательские моды

- Не переименовывай публичный якорь. Если очень надо — оставь старый класс как alias минимум на один релиз.
- Не меняй значение уже существующего `data-*` (например, не переименовывай `bubbles` → `default`).
- Не удаляй переменную из `tokens/colors_and_type.css` без migration notice.
- Новый компонент — новый якорь (OK). Новый якорь в существующем компоненте — OK.
- Перед рефакторингом компонента, у которого есть публичные якоря, загляни в SELECTOR_CONTRACT.md.

---

## 9. Проверка перед PR

- [ ] Все цвета/размеры — через `var(--nest-*)`.
- [ ] Корневой якорь соответствует SELECTOR_CONTRACT.md.
- [ ] Проверено на mobile (< 960) и desktop.
- [ ] Safe mode не ломается (`?safe` грузит страницу).
- [ ] i18n ключи добавлены в RU + EN.
- [ ] scoped стили; никаких `:deep()` на чужих якорях.
- [ ] `reducedMotion` не ломает пользовательский опыт.
