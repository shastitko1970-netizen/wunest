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
COPY --from=frontend /app/frontend/dist ./frontend/dist

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /out/wunest ./cmd/wunest

# ── Stage 3: runtime ─────────────────────────────────
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata tini && \
    adduser -D -u 1000 nest
WORKDIR /app
COPY --from=backend /out/wunest /app/wunest
COPY migrations /app/migrations
USER nest

ENV ENV=production \
    HTTP_PORT=9090 \
    LOG_LEVEL=info

EXPOSE 9090

ENTRYPOINT ["/sbin/tini", "--"]
CMD ["/app/wunest"]
