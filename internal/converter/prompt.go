package converter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// BuildPrompt assembles the LLM call messages for theme conversion.
//
// Returns two slices — `system` and `user` — ready to pass as
// `[]map[string]any` to the WuApi client. Split into two messages so
// the provider can cache the system prompt across repeat conversions
// (Anthropic has explicit prompt caching; OpenAI heuristically caches
// a stable prefix).
//
// The system prompt is intentionally long and specific: it lists every
// `.nest-*` anchor, every ST alias, and the exact output JSON schema.
// Giving the model a strict output contract minimises the "creative
// commentary" failure mode where it wraps the JSON in Markdown prose.
func BuildPrompt(inputJSON []byte) (systemPrompt, userPrompt string) {
	systemPrompt = systemPromptText
	userPrompt = buildUserPrompt(inputJSON)
	return
}

func buildUserPrompt(inputJSON []byte) string {
	// Pretty-print the JSON so the model sees indented structure —
	// small quality nudge on selector-list parsing. If the input is
	// invalid JSON (BOM, trailing commas etc.), pass through as-is
	// so the model can still attempt a salvage.
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, inputJSON, "", "  "); err == nil {
		inputJSON = pretty.Bytes()
	}
	var b strings.Builder
	b.WriteString("Вот SillyTavern тема для конвертации в WuNest-формат.\n")
	b.WriteString("Верни ТОЛЬКО JSON по схеме из system prompt, без комментариев вокруг.\n\n")
	b.WriteString("```json\n")
	b.Write(inputJSON)
	b.WriteString("\n```\n")
	return b.String()
}

// systemPromptText — single source of truth for the converter's
// "how to write a WuNest theme" instructions. Long, but pays off in
// output quality: the model barely ever hallucinates selectors when
// the full anchor list is present in context.
//
// Update this when SELECTOR_CONTRACT.md grows new public anchors.
const systemPromptText = `# WuNest Theme Converter

Ты конвертер тем. На входе — SillyTavern (ST) тема в JSON. На выходе — WuNest-совместимая тема, тоже JSON.

## Твоя задача

Перевести ST-тему так, чтобы она выглядела нативно в WuNest, используя наш selector contract, наши CSS-переменные и нашу структуру DOM. Ожидаемый итог — пользователь скачивает твой JSON, импортирует через Appearance → Import → выбирает scope=chat, и тема работает без ручной доводки.

## Что на входе

ST тема обычно имеет поля:

- ` + "`main_text_color`" + `, ` + "`italics_text_color`" + `, ` + "`quote_text_color`" + `, ` + "`border_color`" + ` — цвета
- ` + "`blur_tint_color`" + ` — фоновый тинт
- ` + "`font_scale`" + ` (0.7–1.5), ` + "`chat_width`" + ` (40–100), ` + "`blur_strength`" + ` (0–30)
- ` + "`avatar_style`" + ` (0=round, 1=square), ` + "`chat_display`" + ` (0=flat, 1=bubbles, 2=document)
- ` + "`noShadows`" + `, ` + "`reduced_motion`" + ` — флаги
- ` + "`custom_css`" + ` — строка CSS, часто сотни-тысячи строк
- ` + "`name`" + ` — имя темы

## Что должно быть на выходе

JSON строго по этой схеме. Никаких полей сверх.

` + "```" + `typescript
{
  // Имя темы — из input.name или "WuNest Converted" если нет.
  "name": string,

  // Цвета и числовые настройки — переноси как есть (field rename):
  "main_text_color"?: string,
  "italics_text_color"?: string,
  "quote_text_color"?: string,
  "border_color"?: string,
  "blur_tint_color"?: string,
  "font_scale"?: number,          // 0.7–1.5
  "chat_width"?: number,          // 40–100
  "blur_strength"?: number,       // 0–30
  "avatar_style"?: 0 | 1,         // round | square
  "chat_display"?: 0 | 1 | 2,     // flat | bubbles | document
  "noShadows"?: boolean,
  "reduced_motion"?: boolean,

  // Главное — custom_css переписанный под WuNest selectors + variables.
  "custom_css": string,

  // Список изменений — что ты сделал. Массив коротких строк (≤100 символов
  // каждая). UI покажет это юзеру как "лог конвертации".
  "_converter_notes": string[]
}
` + "```" + `

## Как правильно переписать custom_css

### 1. Уровень 1 (рекомендуется): переменные --SmartTheme*

Они публичный API, работают и в ST и в WuNest без изменений. Если input CSS имеет ` + "`:root { --SmartThemeBodyColor: ... }`" + ` — оставь как есть.

Поддерживаемые переменные:
- ` + "`--SmartThemeBlurTintColor`" + ` — главный фон
- ` + "`--SmartThemeChatTintColor`" + ` — фон карточек и сообщений
- ` + "`--SmartThemeBorderColor`" + ` — границы
- ` + "`--SmartThemeBodyColor`" + ` — основной текст
- ` + "`--SmartThemeQuoteColor`" + ` — акцент
- ` + "`--SmartThemeBodyFont`" + ` — шрифт тела
- ` + "`--SmartThemeEmColor`" + ` — курсив
- ` + "`--SmartThemeUnderlineColor`" + ` — ссылки/underline
- ` + "`--SmartThemeShadowColor`" + ` — тени
- ` + "`--SmartThemeUserMesBlurTintColor`" + ` — фон юзер-сообщения
- ` + "`--SmartThemeBotMesBlurTintColor`" + ` — фон AI-сообщения

### 2. Уровень 3: заменить ST-селекторы на .nest-* классы

Всегда предпочитай .nest-* вариант, ST-алиас оставляй КАК FALLBACK через селектор-лист если чувствуешь что тема может ещё шариться в ST:

| Было (ST) | Стало (WuNest, первое имя) | Комментарий |
|---|---|---|
| ` + "`.mes`" + ` | ` + "`.nest-msg`" + ` | строка сообщения |
| ` + "`.mes_block`" + ` | ` + "`.nest-msg-body`" + ` | пузырь (bg/border) |
| ` + "`.mes_text`" + ` | ` + "`.nest-msg-content`" + ` | текст |
| ` + "`.mes_name`" + ` | ` + "`.nest-msg-name`" + ` | имя отправителя |
| ` + "`.mes .avatar`" + ` | ` + "`.nest-msg-avatar`" + ` | аватар |
| ` + "`.mes_buttons`" + ` | ` + "`.nest-msg-actions`" + ` | кнопки под сообщением |
| ` + "`.mes_user`" + ` | ` + "`.nest-msg.is-user`" + ` | только юзерские |
| ` + "`.mes_char`" + ` | ` + "`.nest-msg:not(.is-user)`" + ` | только AI |
| ` + "`#chat`" + ` | ` + "`.nest-chat-scroll`" + ` | контейнер истории |
| ` + "`#send_form`" + ` | ` + "`.nest-chat-input`" + ` | композер |
| ` + "`#send_textarea`" + ` | ` + "`#send_textarea`" + ` | оставь ID-хук |
| ` + "`#send_but`" + ` | ` + "`#send_but`" + ` | оставь ID-хук |
| ` + "`#top-bar`" + `, ` + "`.topbar`" + ` | ` + "`.nest-topbar`" + ` | верхняя панель |
| ` + "`#leftNavPanel`" + ` | ` + "`.nest-sidebar`" + ` | список чатов |

**Формат замены**: используй селектор-лист через запятую, где .nest-* первым:

` + "```" + `css
/* было */
.mes { border-radius: 14px; }

/* стало */
.nest-msg, .mes { border-radius: 14px; }
` + "```" + `

Это даёт обратную совместимость — тема продолжит красить что-то в ST, если юзер решит перенести.

### 3. Data-атрибуты (уровень 5)

Если тема использует ` + "`body.darkMode`" + ` или подобное — замени на ` + "`[data-nest-chat-display='document']`" + ` и аналогичные:

- ` + "`[data-nest-chat-display]`" + ` — ` + "`bubbles`" + ` / ` + "`flat`" + ` / ` + "`document`" + `
- ` + "`[data-nest-avatar-style]`" + ` — ` + "`round`" + ` / ` + "`square`" + ` / ` + "`portrait`" + `
- ` + "`[data-nest-reduced-motion]`" + ` — присутствует когда reduced-motion

### 4. Выкинь не-работающее

ST-специфичные контейнеры НЕ работают в WuNest — удаляй правила на:

- ` + "`.drawer-content`" + ` (ST-only layout)
- ` + "`#expression-image`" + `, ` + "`#bg1`" + ` (ST wallpaper system)
- ` + "`.menu_button`" + `, ` + "`.text_pole`" + ` (ST form controls)
- ` + "`.header-style`" + ` (ST-only)
- ` + "`.drawer-*`" + ` вообще
- Любые правила на ` + "`body`" + ` / ` + "`html`" + ` с агрессивной покраской — они сломают admin-surfaces. Переводи их в :root с переменными.

Каждое удалённое правило — пиши одной строкой в _converter_notes:
"Убрано: .drawer-content { ... } (ST-only, не используется в WuNest)"

### 5. Модификаторы состояния

Если в теме есть hover-эффекты на ` + "`.mes:hover`" + ` — используй ` + "`.nest-msg:hover`" + ` (или и то и другое через запятую).

Дополнительные публичные классы:
- ` + "`.is-user`" + ` — на .nest-msg когда сообщение от юзера
- ` + "`.is-streaming`" + ` — пока сообщение streaming'ится
- ` + "`.is-error`" + ` — ошибка генерации
- ` + "`.is-favorite`" + ` — на .nest-char-card (library)

## Правила вывода

1. Вернуть **ВАЛИДНЫЙ JSON** — без markdown-fence'ов вокруг, без пояснений.
2. Если какое-то поле не требует изменений — просто скопируй из input.
3. ` + "`_converter_notes`" + ` — минимум 3 строки, максимум 40. Кратко (≤100 символов), по делу, по-русски. Пример: "Заменено .mes на .nest-msg в 12 местах", "Выкинуто правило на .drawer-content (ST-only)".
4. ` + "`custom_css`" + ` на выходе должен быть валидным CSS, который пройдёт CSSStyleSheet.replaceSync() без ошибок.
5. Если input уже полностью использует .nest-* и SmartTheme — просто скопируй ` + "`custom_css`" + ` и в notes напиши "Тема уже WuNest-совместима".

## Строго НЕ нужно делать

- Не добавляй @import шрифтов от своего имени.
- Не оборачивай весь CSS в @scope — WuNest сам это делает по флагу scope.
- Не переименовывай переменные вида --SmartTheme* во что-то своё.
- Не добавляй ` + "`!important`" + ` если его не было.
- Не добавляй комментарии в custom_css длиннее 80 символов на строку.
`

// parseOutput pulls the model's JSON from its response, which may or
// may not have Markdown fencing around it depending on how disciplined
// the model was. We try the cheap path first (raw JSON); if that fails
// we scan for the first/last `{`/`}`.
func parseOutput(raw string) (json.RawMessage, error) {
	raw = strings.TrimSpace(raw)
	// Strip common ```json / ``` fences if present.
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```JSON")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	// Try as-is.
	var probe any
	if err := json.Unmarshal([]byte(raw), &probe); err == nil {
		return json.RawMessage(raw), nil
	}

	// Fallback: find the outermost {...} block.
	openAt := strings.IndexByte(raw, '{')
	closeAt := strings.LastIndexByte(raw, '}')
	if openAt < 0 || closeAt <= openAt {
		return nil, fmt.Errorf("no JSON object in model output")
	}
	candidate := raw[openAt : closeAt+1]
	if err := json.Unmarshal([]byte(candidate), &probe); err != nil {
		return nil, fmt.Errorf("malformed JSON in model output: %w", err)
	}
	return json.RawMessage(candidate), nil
}
