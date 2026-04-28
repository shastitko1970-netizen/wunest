# WuNest

Modern web client for LLM roleplay, part of the [WuSphere](https://wusphere.ru) ecosystem.

**Status:** ✅ live at [`nest.wusphere.ru`](https://nest.wusphere.ru) — actively iterating.

## What is it

WuNest is a SaaS web client for character-driven conversations with language models.
Think SillyTavern, but cloud-hosted, mobile-friendly, and with a modern UI.

It plugs into [WuApi](https://api.wusphere.ru) for identity, API keys, and billing,
so users don't need to configure anything — log in with Google/Telegram/etc., and start chatting.

## Architecture (bird's-eye)

```
Browser (Vue 3 + Vuetify SPA)
   │
   ├──► nest.wusphere.ru/api/*    (this service — Go)
   │       │
   │       ├── reads wu_session cookie from .wusphere.ru
   │       ├── resolves user via WuApi /api/me
   │       ├── stores characters, chats, worlds in own Postgres DB
   │       └── proxies chat completions to WuApi (SSE stream pass-through)
   │
   └──► WuApi /v1/chat/completions  (upstream LLM proxy)
```

High-level design lives in the (private) obsidian vault under
`../WuTavern/obsidian/Tavern/99-WuTavern-Plan/`. If you've cloned only
this repo, the architecture diagram above + [`CONTRIBUTING.md`](./CONTRIBUTING.md)
have everything a new contributor needs.

## Tech stack

- **Backend:** Go 1.25+ (`pgx/v5`, `go-redis/v9`, stdlib `net/http`)
- **Frontend:** Vue 3 + TypeScript + Vite 8 + Vuetify 4 (mirrors WuApi's stack)
- **DB:** PostgreSQL (own database `wunest` on the shared instance)
- **Cache:** Redis (shared with WuApi, `nest:*` namespace)
- **Deploy:** Docker blue/green on Selectel (`185.184.79.66`)

## Project layout

```
wunest/
├── cmd/wunest/           # main.go entry point
├── internal/             # Go packages
│   ├── config/           # env + config loading
│   ├── db/               # Postgres & Redis clients
│   ├── auth/             # wu_session cookie middleware
│   ├── wuapi/            # upstream WuApi HTTP client
│   ├── models/           # domain types
│   └── server/           # HTTP router & handlers
├── migrations/           # SQL migrations
├── scripts/              # deploy, nginx configs
├── frontend/             # Vue 3 SPA (embedded into Go binary at build)
├── Dockerfile
├── docker-compose.yml
├── Makefile              # deploy targets (ssh to Selectel)
├── .env.example
└── go.mod
```

## Dev quickstart

Full step-by-step in [`CONTRIBUTING.md`](./CONTRIBUTING.md). TL;DR:

```bash
# 1. Bring up Postgres + Redis (we use the same compose as prod)
docker compose up -d postgres redis

# 2. Configure the backend
cp .env.example .env
# edit DATABASE_URL, REDIS_ADDR, WUAPI_BASE_URL, COOKIE_DOMAIN
# (defaults work for local dev; SECRETS_KEY must be 32 bytes)

# 3. Run migrations
go run ./cmd/wunest --migrate-only   # or: make migrate

# 4. Start backend on :9090
go run ./cmd/wunest

# 5. Start frontend dev server (separate terminal)
cd frontend
npm install
npm run dev    # http://localhost:5173, proxies /api/* → :9090
```

**Gotchas:**
- BYOK calls to Anthropic / OpenAI use an outbound proxy pool because the
  origin host is geo-blocked. Set `OUTBOUND_PROXIES=` in `.env` for local
  dev — without it, BYOK requests to those providers will 403. WuApi-pool
  and OpenRouter / DeepSeek / Mistral / Google work without a proxy.
- MinIO is **optional** in dev. Without it, character-import avatars and
  background-image uploads fall back to "no image", but everything else
  works.
- `wu_session` cookie is shared across `.wusphere.ru` subdomains. For
  local dev, log in on the live site once and copy the cookie value into
  your dev browser, OR run WuApi locally (instructions in WuApi repo).

## Deploy

See `Makefile`:

```bash
make deploy       # blue/green deploy to production
make logs         # tail current active container
make status       # show active color + health
```

## License

MIT — see [LICENSE](./LICENSE).
