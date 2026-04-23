# Пресеты

Пресет — сохранённый набор параметров генерации, который можно применить к любому чату или сделать дефолтом.

## Типы

В WuNest пять типов пресетов (совместимо с SillyTavern):

- **Сэмплеры** — температура, top-p, top-k, min-p, max_tokens, penalties, seed, stop strings.
- **Инструкт** — обёртки турнов: `input_sequence`, `output_sequence`, `system_sequence`. Для text-completion моделей.
- **Контекст** — `story_string` с макросами `{{char}}`, `{{user}}`, `{{scenario}}`, и формат встройки истории.
- **Систем-промпт** — текст, заменяющий дефолтный system prompt персонажа. Поддерживает post_history_instructions.
- **Рассуждение** — prefix/suffix/separator для `<think>`-блоков. Применимо к o1, Claude thinking, DeepSeek-R1.

## Управление

**Библиотека → Пресеты** — список, сгруппированный по типу. Для каждого пресета:

- Карандаш — открыть в редакторе
- Code-braces — показать raw JSON
- Download — экспорт в ST-совместимый JSON
- Звезда — сделать дефолтом для этого типа (применяется когда нет явного выбора в чате)
- Корзина — удалить

## Импорт

**Пресеты → Импорт** принимает JSON. Тип определяется автоматически по signature-ключам:

- `input_sequence`, `output_sequence`, `wrap` → instruct
- `story_string`, `chat_start`, `example_separator` → context
- `temperature`, `top_p`, `max_tokens`, `frequency_penalty` → sampler
- `prefix`, `suffix`, `separator` (без sequence ключей) → reasoning
- `openai_model`, `temp_openai` → openai legacy

Если детектор не уверен — показывается свёрнутый override для ручного выбора.

## Привязка к чату

В drawer'е параметров генерации (`mdi-tune-variant` в шапке чата) можно выбрать preset из dropdown'а. Этот выбор сохраняется в `chat_metadata.sampler.preset_id` — применяется при каждой генерации в этом чате.

Можно также сделать preset дефолтом для своего пользователя — звёздочкой в списке пресетов. Дефолт применяется если в чате preset не выбран явно.

## Soft validation

При сохранении пресета сервер проверяет что `data` — это JSON объект (не null, не массив, не скаляр), и что базовые типы полей соответствуют (числа — числа, строки — строки, stop — `string[]` или одиночная строка). Неизвестные поля пропускаются без ошибок — чтобы ST-specific расширения не ломали импорт.
