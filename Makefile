# WuNest — Makefile
#
# Mirrors WuApi's deploy pattern: rsync to the production server, then run
# a remote deploy.sh that does blue/green container swap. Local dev uses
# docker-compose.

SERVER  := root@185.184.79.66
APP_DIR := /opt/wunest

.PHONY: help dev build lint test tidy deploy sync setup logs env stop ps status rollback

help:
	@echo "WuNest — targets:"
	@echo "  make dev        — run go backend locally on :9090"
	@echo "  make build      — build the Go binary (also compiles the frontend)"
	@echo "  make lint       — go vet + gofmt check"
	@echo "  make test       — go test ./..."
	@echo "  make tidy       — go mod tidy"
	@echo ""
	@echo "  make deploy     — blue/green deploy to $(SERVER)"
	@echo "  make sync       — rsync code only (no deploy)"
	@echo "  make logs       — tail the currently active container"
	@echo "  make status     — show blue/green status on server"
	@echo "  make env        — edit .env on server"
	@echo "  make setup      — first-time server setup (nginx + .env)"

# ── Local ───────────────────────────────────────────
dev:
	go run ./cmd/wunest

build:
	cd frontend && npm run build
	go build -ldflags="-s -w" -o wunest ./cmd/wunest

lint:
	go vet ./...
	@gofmt -l -s . | (! grep .) || (echo "gofmt issues — run gofmt -w -s ." && exit 1)

test:
	go test -race -count=1 ./...

tidy:
	go mod tidy

# ── Blue/Green deploy ───────────────────────────────
deploy: sync
	@echo ""
	@echo "=== WuNest: Blue/Green Deploy ==="
	ssh $(SERVER) "bash $(APP_DIR)/scripts/deploy.sh"

sync:
	@echo "[sync] Uploading files to $(SERVER):$(APP_DIR)/ ..."
	rsync -avz --delete \
		--exclude '.git' \
		--exclude '.env' \
		--exclude '.active' \
		--exclude 'frontend/node_modules' \
		--exclude 'frontend/dist' \
		--exclude '*.md' \
		--exclude 'obsidian' \
		./ $(SERVER):$(APP_DIR)/
	ssh $(SERVER) "chmod +x $(APP_DIR)/scripts/*.sh"

setup: sync
	@echo "[setup] Initial server setup ..."
	ssh $(SERVER) "cp $(APP_DIR)/scripts/nginx-nest.conf /etc/nginx/sites-available/nest.wusphere.ru"
	ssh $(SERVER) "ln -sf /etc/nginx/sites-available/nest.wusphere.ru /etc/nginx/sites-enabled/nest.wusphere.ru"
	ssh $(SERVER) "echo 'upstream wunest_backend { server 127.0.0.1:9090; }' > /etc/nginx/conf.d/wunest-upstream.conf"
	ssh $(SERVER) "nginx -t && systemctl reload nginx"
	ssh $(SERVER) "test -f $(APP_DIR)/.env || cp $(APP_DIR)/.env.example $(APP_DIR)/.env"
	@echo "[setup] Done. Edit server .env: make env"

logs:
	@ssh $(SERVER) "cat $(APP_DIR)/.active 2>/dev/null || echo 'unknown'" | xargs -I{} \
		ssh $(SERVER) "docker logs -f --tail=100 wunest-{}"

logs-blue:
	ssh $(SERVER) "docker logs -f --tail=100 wunest-blue"

logs-green:
	ssh $(SERVER) "docker logs -f --tail=100 wunest-green"

status:
	@echo "=== WuNest status ==="
	@ssh $(SERVER) "echo -n 'Active: ' && cat $(APP_DIR)/.active 2>/dev/null || echo 'unknown'"
	@ssh $(SERVER) "docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}' | grep wunest || echo '(no wunest containers)'"
	@ssh $(SERVER) "curl -sf http://localhost:9090/health 2>/dev/null && echo ' :9090 OK' || echo ' :9090 DOWN'"
	@ssh $(SERVER) "curl -sf http://localhost:9091/health 2>/dev/null && echo ' :9091 OK' || echo ' :9091 DOWN'"

env:
	ssh $(SERVER) "test -f $(APP_DIR)/.env || cp $(APP_DIR)/.env.example $(APP_DIR)/.env"
	ssh -t $(SERVER) "nano $(APP_DIR)/.env"

stop:
	ssh $(SERVER) "cd $(APP_DIR) && docker compose down"

rollback:
	@echo "[rollback] Switching active color ..."
	ssh $(SERVER) "bash $(APP_DIR)/scripts/rollback.sh"
