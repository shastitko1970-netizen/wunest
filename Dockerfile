# ── Stage 1: frontend build ──────────────────────────
FROM node:22-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci --no-audit --no-fund || npm install
COPY frontend/ ./
RUN npm run build

# ── Stage 2: backend build ───────────────────────────
FROM golang:1.25-alpine AS backend
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum* ./
RUN go mod download || true
COPY . .

# Embed the frontend bundle into the binary (via //go:embed).
# Overwrites the placeholder index.html that lives in internal/spa/dist.
COPY --from=frontend /app/frontend/dist/. ./internal/spa/dist/

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /out/wunest ./cmd/wunest

# The storage reaper ships in the same image — a daily systemd timer on
# the host invokes it via `docker run --rm ... /app/nest-storage-reaper`.
# Keeps one image to deploy, and the reaper always matches the running
# schema because it's built from the same source tree.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /out/nest-storage-reaper ./cmd/nest-storage-reaper

# ── Stage 3: runtime ─────────────────────────────────
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata tini && \
    adduser -D -u 1000 nest
WORKDIR /app
COPY --from=backend /out/wunest /app/wunest
COPY --from=backend /out/nest-storage-reaper /app/nest-storage-reaper
COPY migrations /app/migrations
USER nest

ENV ENV=production \
    HTTP_PORT=9090 \
    LOG_LEVEL=info

EXPOSE 9090

ENTRYPOINT ["/sbin/tini", "--"]
CMD ["/app/wunest"]
