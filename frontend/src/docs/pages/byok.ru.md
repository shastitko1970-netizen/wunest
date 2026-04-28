# BYOK — Свои ключи провайдеров

BYOK (Bring Your Own Key) — использовать свой API-ключ у OpenAI / Anthropic / OpenRouter / собственного llama.cpp сервера вместо wu-gold billing'а через WuApi.

## Когда это нужно

- У тебя уже оплачен план ChatGPT Plus / Claude Pro — хочется использовать тот же ключ
- Нужен провайдер которого нет в нашем каталоге
- Self-hosted (LiteLLM, Ollama, llama.cpp с OpenAI-compat) — работают как custom endpoint
- Хочется чтобы запросы не проходили через WuApi (приватность)

## Добавление ключа

**Настройки → BYOK → Добавить ключ**. Форма:

- **Провайдер** — dropdown. Известным провайдерам (OpenAI, OpenRouter, DeepSeek, Mistral, Anthropic, Google) подставляется канонический Base URL автоматически.
- **Base URL** — можно переопределить дефолт (для regional endpoint'а, прокси, self-hosted).
- **Метка** — опциональная подпись «личный OpenAI», «рабочий Anthropic» чтобы различать в списке.
- **API-ключ** — вставляется в password-поле. Сохраняется шифрованным (AES-GCM) на сервере.

После сохранения плейнтекст нигде не хранится — только зашифрованный blob и маска (`sk-…6411`) для UI.

## Применение к чату

В шапке чата — иконка ключа (`mdi-key-variant`). Открывается picker:

- «Использовать ключ WuApi» — дефолт, трафик через api.wusphere.ru, оплата в wu-gold
- Твои BYOK-ключи сгруппированы по провайдеру

Выбор сохраняется в `chat_metadata.byok_id`. При следующей отправке стрим идёт **напрямую** к `{base_url}/chat/completions` с Bearer заголовком твоего ключа. WuApi не задействован.

Иконка ключа подсвечивается primary цветом, когда чат пришпилен к BYOK — видно на глаз что этот чат тратит твой баланс у провайдера, а не wu-gold.

## Поддерживаемые провайдеры

| Провайдер   | URL                                             | Совместимость        |
|-------------|-------------------------------------------------|----------------------|
| OpenAI      | `api.openai.com/v1`                             | Нативно OpenAI       |
| OpenRouter  | `openrouter.ai/api/v1`                          | OpenAI-compat        |
| DeepSeek    | `api.deepseek.com/v1`                           | OpenAI-compat        |
| Mistral     | `api.mistral.ai/v1`                             | OpenAI-compat        |
| Anthropic   | `api.anthropic.com/v1`                          | Compat endpoint (может нужен header) |
| Google      | `generativelanguage.googleapis.com/v1beta/openai` | Compat endpoint |
| Custom      | ты указываешь                                    | OpenAI-compat endpoint требуется |

Для Anthropic и Google нативные API отличаются (Anthropic `/v1/messages`, Google Gemini format). Дефолтный URL у нас ведёт на их **OpenAI-compat** слой — `api.anthropic.com/v1` (Anthropic Compat) и `generativelanguage.googleapis.com/v1beta/openai` (Google). Оба работают со стримом и usage-токенами, без прокси через OpenRouter. Для custom провайдера ожидается `/chat/completions` с OpenAI-style payload.

## Безопасность

- Ключи шифруются AES-GCM с 32-байтным master key из `SECRETS_KEY` env. 12-байтный random nonce на запись.
- Master key инвалидирует все ключи — rotation не реализован, если перегенерируешь, придётся перевводить ключи
- Ключи никогда не возвращаются в API ответах — только маска и base URL
- Decrypt happens только в момент стрима, scoped по user_id
- Удаление ключа не trimит историю чатов — если удалишь ключ пришпилённый к чату, он молча откатится на WuApi для следующего сообщения

## Биллинг

BYOK-чаты **не расходуют wu-gold** — биллинг полностью на стороне провайдера, по тарифам твоего ключа. История запросов в Account → История золота показывает только wu-gold-расход (т.е. чаты на WuApi-пуле). BYOK-токены считаются у самого провайдера в его dashboard (OpenAI usage, Anthropic console и т.д.).

**Известное ограничение:** widget баланса в топбаре не отделяет BYOK-вызовы — если все чаты у тебя BYOK, а wu-gold не двигается, виджет может показывать стайл «активность есть, баланс не падает». Это нормально, не баг — просто widget агрегирует общую активность, не различая источники.
