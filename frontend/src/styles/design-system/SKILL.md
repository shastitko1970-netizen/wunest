---
name: wunest-design
description: Use this skill to generate well-branded interfaces and assets for WuNest (a SillyTavern-style AI roleplay platform built on Vue 3 + Vuetify), either for production or throwaway prototypes/mocks. Contains essential design guidelines, colors, type, fonts, selector contract, adaptive rules, CSS-mod system, and ready-to-copy tokens for prototyping across desktop browser, mobile browser, and mobile app webview contexts.
user-invocable: true
---

Read the `README.md` file within this skill, and explore the other available files:

- `tokens/colors_and_type.css` — всё, что нужно вставить в любой standalone HTML, чтобы получить фирменные переменные и типографские классы `.nest-h1`, `.nest-body`, `.nest-mono`, и т.д.
- `docs/VISUAL_FOUNDATIONS.md` — визуальная ДНК (цвет, тип, спейсинг, тени, радиусы, анимации, состояния).
- `docs/ADAPTIVE_RULES.md` — правила адаптива: desktop / mobile web / mobile app (webview).
- `docs/SELECTOR_CONTRACT.md` — стабильные якоря (id, классы, data-атрибуты, переменные). **Не придумывай новые, используй эти.**
- `docs/CSS_MODS_GUIDE.md` — как пользователь создаёт тему (важно понимать, если делаешь пример темы).
- `docs/DEVELOPER_GUIDE.md` — шаблон Vue-компонента, правила токенов, что МОЖНО и что НЕЛЬЗЯ.
- `docs/CONTENT_FUNDAMENTALS.md` — тон, RU/EN copy, правила эмодзи и casing.
- `themes/*.css` — 5 эталонных тем как референс кастомизации.

## Как использовать

Если ты создаёшь **визуальный артефакт** (слайд, мок, прототип, превью темы):
1. Скопируй `tokens/colors_and_type.css` в проект (или inline в `<style>`).
2. Используй якоря из `SELECTOR_CONTRACT.md`: `#chat`, `.nest-msg`, `.nest-msg-body`, `.nest-composer`, и т.п.
3. Любой цвет/размер — через `var(--nest-*)`. Хардкодить нельзя.
4. Если что-то адаптивное — брейкпоинт `960px`, mobile получает overlay-drawer и actions-always-visible.
5. Для шрифтов: display — Fraunces, body — Outfit, mono — JetBrains Mono. Импорт уже лежит в `colors_and_type.css`.
6. Эмодзи в шелле **не используй**. Иконки — `mdi-*` (Material Design Icons). Unicode dingbat'ы только: `▲` (logo), `▍` (stream cursor), `·` (separator).
7. RU-текст — «вы» без капитализации, sentence case заголовки. EN — imperative, sentence case.

Если ты работаешь над **production-кодом**:
1. Читай `DEVELOPER_GUIDE.md` целиком — там шаблон компонента.
2. Любой новый компонент получает `.nest-<name>` класс-якорь.
3. Состояние → `data-nest-*` атрибут (для переключений режимов) или `.is-*` класс (для локального вида).
4. Scoped SCSS, один уровень nesting, `@media (max-width: 959.98px)` для mobile.
5. i18n: `t('key')`, добавлять в RU и EN одновременно.
6. Никогда не меняй значение существующего селектора из `SELECTOR_CONTRACT.md` — это ломает пользовательские моды.

Если пользователь **запустил этот скилл без контекста**, спроси, что он хочет создать: тему? UI-компонент? прототип экрана? промо-мок? И задай уточняющие вопросы о: поверхности (chat / library / settings), устройстве (desktop / mobile / обе), языке (RU/EN/обе), желаемой кастомизации (только цвета / плюс типографика / полная перерисовка). Действуй как эксперт-дизайнер — выдавай HTML-артефакты или production-код, в зависимости от задачи.

## Важное

- **Никогда не перерисовывай айдентику с нуля.** Если нужен логотип — используй `▲ WuNest` в `var(--nest-font-display)`. Если нужен иконсет — подключи `@mdi/font` с CDN, не рисуй SVG руками.
- **Не изобретай новые токены.** Если нужного не хватает — flag it, не молча добавляй `--my-color`.
- **WuNest совместим с SillyTavern-темами.** Если пользователь просит тему — результат должен быть валидным `:root { --SmartTheme*: ... }` блоком, опционально с `.mes`, `#chat`, `#send_textarea` селекторами.
