# BYOK — bring your own provider keys

BYOK (Bring Your Own Key) lets you use your own OpenAI / Anthropic / OpenRouter / custom llama.cpp server key instead of wu-gold billing through WuApi.

## When you want this

- You already pay for ChatGPT Plus / Claude Pro and want to use that key
- You need a provider WuApi doesn't proxy
- Self-hosted LiteLLM / Ollama — works as a custom OpenAI-compat endpoint
- You want your requests to skip WuApi entirely (privacy)

## Adding a key

**Settings → BYOK → Add key**. Form:

- **Provider** — dropdown. Known providers (OpenAI, OpenRouter, DeepSeek, Mistral, Anthropic, Google) auto-fill their canonical Base URL.
- **Base URL** — editable so you can override for a regional endpoint, proxy, or self-hosted server.
- **Label** — optional — "personal OpenAI" / "work Anthropic" so the list stays readable.
- **API key** — password field. Stored encrypted (AES-GCM) on the server.

After save, plaintext is nowhere — only the ciphertext blob and a masked preview (`sk-…6411`) for the UI.

## Pinning to a chat

In the chat header — the key icon (`mdi-key-variant`). Opens a picker:

- "Use the WuApi key" — default, traffic via api.wusphere.ru, billed in wu-gold
- Your BYOK keys grouped by provider

The pick persists in `chat_metadata.byok_id`. On the next send, the stream goes **directly** to `{base_url}/chat/completions` with your key as the Bearer token. WuApi isn't in the request path.

The icon gets a primary-color tint when the chat is pinned, so you can see at a glance that this chat charges against your provider balance, not wu-gold.

## Supported providers

| Provider    | URL                                              | Compat              |
|-------------|--------------------------------------------------|---------------------|
| OpenAI      | `api.openai.com/v1`                              | Native OpenAI       |
| OpenRouter  | `openrouter.ai/api/v1`                           | OpenAI-compat       |
| DeepSeek    | `api.deepseek.com/v1`                            | OpenAI-compat       |
| Mistral     | `api.mistral.ai/v1`                              | OpenAI-compat       |
| Anthropic   | `api.anthropic.com/v1`                           | Compat layer (may need headers) |
| Google      | `generativelanguage.googleapis.com/v1beta/openai`| Compat layer        |
| Custom      | you provide                                      | Must expose `/chat/completions` in OpenAI format |

Anthropic's native API (`/v1/messages`) and Google Gemini's native format differ from OpenAI. The default URL points at their OpenAI-compat layer — may work, may need a proxy like OpenRouter instead.

## Security

- Keys are encrypted AES-GCM with a 32-byte master key from the server's `SECRETS_KEY` env. Per-row random 12-byte nonce.
- Rotating the master key invalidates every stored key — rotation tooling isn't built yet, so you'd need to re-enter.
- Plaintext is never returned in API responses — only the masked preview + base URL.
- Decryption only happens on the stream path, scoped by `user_id`.
- Deleting a key doesn't touch chats pinned to it — they silently fall back to WuApi on the next turn.

## Billing

BYOK chats don't draw wu-gold — billing is entirely provider-side. The WuApi balance widget in the header doesn't know about BYOK pins yet, so it might show usage that didn't happen. TODO.
