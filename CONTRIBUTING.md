# Contributing to WuNest

WuNest is a Vue 3 + Go SaaS web client for LLM roleplay, hosted on `nest.wuproj.com` and integrated with [WuApi](https://api.wuproj.com). This document is for contributors who want to run it locally or ship a PR.

End-user documentation lives at `/docs` in the running app. This file covers the build / dev / deploy story.

## Requirements

- **Go 1.25+** (see `go.mod` — `pgx/v5`, `go-redis/v9`, stdlib `net/http`)
- **Node 20+** + npm — frontend build
- **Docker / Docker Compose** — for local Postgres + Redis (optional but recommended)
- **make** (Linux/macOS) — convenience targets in `Makefile`
- **rsync** + **ssh** — only if you're deploying to production

Windows: WSL2 recommended for the Docker / make pieces. Native Windows works if you have Docker Desktop + Git Bash.

## Local setup (first time)

### 1. Bring up Postgres + Redis

```bash
# Use the same docker-compose as production
docker compose up -d postgres redis
```

This starts:
- Postgres 16 on `127.0.0.1:5434` (DB `wunest`, user `wunest`)
- Redis 7 on `127.0.0.1:6380` (`nest:*` keyspace)

If you don't want Docker, point `DATABASE_URL` / `REDIS_ADDR` at your own instances.

### 2. Configure the backend

```bash
cp .env.example .env
```

Edit `.env`:

| Key | Required? | Default | Notes |
|---|---|---|---|
| `DATABASE_URL` | yes | `postgresql://wunest:wunest@127.0.0.1:5434/wunest` | matches docker-compose |
| `REDIS_ADDR` | yes | `127.0.0.1:6380` | matches docker-compose |
| `WUAPI_BASE_URL` | yes | `https://api.wuproj.com` | for production-like dev; runs in your local cluster if you have one |
| `SESSION_COOKIE_DOMAIN` | no | inferred from `PUBLIC_BASE_URL` | e.g. `.wuproj.com` in prod — must match WuApi cookie domain |
| `SECRETS_KEY` | yes | (32 random bytes, base64) | AES-GCM master key for BYOK encryption. Generate via `openssl rand -base64 32`. **Never commit.** |
| `MINIO_ENDPOINT` | no | empty | optional; without it, character/avatar/bg uploads fall back to "no image" |
| `OUTBOUND_PROXIES` | no | empty | see "BYOK gotcha" below |
| `PORT` | no | `9090` | backend listen port |

### 3. Run migrations

Migrations run on every backend startup automatically (see `internal/db/postgres.go`'s `Migrate`). To run them manually:

```bash
go run ./cmd/wunest --migrate-only
# OR via make:
make migrate
```

If you change a migration AFTER deploying once, **don't** edit the existing file — add a new `0NN_my_change.sql`. Migrations are append-only.

### 4. Start backend

```bash
go run ./cmd/wunest
# Listens on :9090, serves /api/* + the SPA bundle (built into the Go binary)
```

For development you usually want the frontend dev server in a separate terminal — the embedded SPA is rebuilt by `make build` only.

### 5. Start frontend dev server

```bash
cd frontend
npm install      # first time only
npm run dev      # http://localhost:5173
```

Vite proxies `/api/*` → backend on `:9090`, so frontend hot-reload works against your local backend.

### 6. Sign in for development

WuNest uses `wu_session` cookie set by WuApi (`api.wuproj.com`). Two options:

**Option A (easy)** — log in on the live site once, then copy the cookie value:
1. Visit `https://nest.wuproj.com` and sign in
2. DevTools → Application → Cookies → `.wuproj.com` → copy `wu_session`
3. In your dev browser, paste the value into `localhost`'s cookies (manually via DevTools, since the Domain attribute differs)
4. Visit `http://localhost:5173` — should be authenticated

**Option B (full local stack)** — run WuApi locally too. See WuApi's repo for setup. You'll need both stacks running, with `SESSION_COOKIE_DOMAIN=localhost` on both.

## Gotchas

### BYOK outbound proxy

Anthropic and OpenAI native APIs **geo-block** several Russian/CIS regions on the origin host. Without an outbound proxy, BYOK requests to those providers will 403.

Set `OUTBOUND_PROXIES=` in `.env` to a comma-separated list of `socks5://user:pass@host:port` URLs. WuApi-pool requests, OpenRouter, DeepSeek, Mistral and Google work without a proxy.

The proxy pool round-robins; pool of 0 → falls back to direct (good enough for non-blocked providers).

### MinIO is optional

Without `MINIO_ENDPOINT`, the storage client returns `ErrDisabled` for all uploads. Character imports lose their avatar thumbnails, background images can't be uploaded — but everything else works.

If you want MinIO locally, the docker-compose file has it commented out — uncomment, set `MINIO_*` env vars, and you're set.

### Cookie domain mismatch in dev

`wu_session` is shared across `*.wuproj.com` subdomains via `Domain=.wuproj.com`. For pure `localhost` dev that domain doesn't apply — you can either:
- Run with `SESSION_COOKIE_DOMAIN=localhost` and a local WuApi (Option B above)
- Or paste the prod cookie manually (Option A above)

There's no "auto" mode that handles both transparently — pick one for your dev session.

## Project layout

See [`README.md`](./README.md) for the bird's-eye view. Summary:

- `cmd/wunest/` — `main.go` entry point (binary embeds the SPA at build)
- `internal/` — Go packages (auth, chats, characters, presets, byok, converter, etc.)
- `frontend/` — Vue 3 SPA, Vite-bundled, embedded into Go binary at `make build`
- `migrations/` — append-only SQL migrations (numeric prefix `0NN_*`)
- `scripts/` — production deploy + nginx configs
- `frontend/src/styles/design-system/docs/` — internal design docs (not user-facing)
- `frontend/src/docs/pages/` — user-facing /docs markdown (RU + EN parallel)

## Tests

```bash
# Backend
make test              # equivalent: go test -race -count=1 ./...

# Specific package
go test ./internal/converter/ -v

# Frontend
cd frontend && npm run typecheck   # via vue-tsc; build runs typecheck too
```

## Deploy

Production runs blue/green Docker on Selectel (`185.184.79.66`).

```bash
make deploy            # rsync + ssh + bash scripts/deploy.sh on server
make logs              # tail current active container
make status            # active color + health
make rollback          # emergency: switch to inactive color
```

Don't deploy without a clean `make test` and `cd frontend && npm run build` locally first — the production deploy script doesn't rebuild on rsync, it ships your local working tree.

## Conventions

- **Migrations** explain *why* not just *what* in the header comment block. See `migrations/008_chat_search.sql` and `migrations/013_converter_input_data.sql` for the model.
- **Inline rationale** for non-obvious decisions — quote the user / linked-issue / linked-vault-note. Don't bury "why" in commit messages alone.
- **Code comments** use godoc/JSDoc for exported APIs. Per-package top-of-file comment frames the package's role.
- **Dev-doc files** (`internal/converter/handler.go:74-88` is exemplary — request/response/errors as inline doc).
- **Frontend stores** — top-of-file comment is the contract (`frontend/src/stores/theme.ts:5-32`, `appearance.ts:28-47`). Keep it accurate.

## Pull-request checklist

- [ ] `make test` passes
- [ ] `cd frontend && npm run build` clean (no vue-tsc errors, no Vite warnings about unused imports)
- [ ] If you added a public DB field — also added it to the Go struct json tag AND to the frontend TypeScript type. The two **MUST** match (camelCase on wire — see `internal/models/models.go` patterns).
- [ ] If you added a frontend route — also updated `frontend/src/router.ts` AND `Docs.vue` if it's user-visible.
- [ ] If you added a doc page — added entry in `frontend/src/docs/index.ts` with both RU and EN content, and tested `/docs/<slug>` renders.
- [ ] If you added an internal design rule — updated `frontend/src/styles/design-system/docs/` AND made sure it doesn't drift from `tokens/colors_and_type.css`.
- [ ] No `console.log` / `console.warn` left in shipped code (use `slog` in Go, structured `error.value = ...` in stores). `console.warn` for non-fatal store errors is acceptable but flag in PR description.

## Where to ask

- **Issue**: GitHub issues
- **Quick chat**: [@wuapi_support](https://t.me/wuapi_support) on Telegram
- **Big design**: open a PR titled `RFC: ...` with markdown in `docs/rfcs/` (folder doesn't exist yet — first RFC creates it)
