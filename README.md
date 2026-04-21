# WuNest

Modern web client for LLM roleplay, part of the [WuSphere](https://wusphere.ru) ecosystem.

**Status:** 🏗️ early scaffold — `nest.wusphere.ru` not yet live.

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

See [obsidian vault](../WuTavern/obsidian/Tavern/99-WuTavern-Plan/WuApi-Integration.md)
for the full integration design.

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

## Dev quickstart (WIP)

```bash
# backend
cp .env.example .env
go run ./cmd/wunest

# frontend (separate terminal)
cd frontend
npm install
npm run dev
```

Frontend dev server at http://localhost:5173 proxies `/api/*` to backend on `:9090`.

## Deploy

See `Makefile`:

```bash
make deploy       # blue/green deploy to production
make logs         # tail current active container
make status       # show active color + health
```

## License

MIT — see [LICENSE](./LICENSE).
