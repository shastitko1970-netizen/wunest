#!/usr/bin/env bash
# Blue/Green zero-downtime deploy for WuNest.
#
# Runs on the production server (invoked via `make deploy` from a dev box).
# Assumes the repo has been rsync'd to $APP_DIR.
#
# Ports:
#   blue  — 9090
#   green — 9091
#
# Flow:
#   1. Detect currently active color (read .active file).
#   2. Build a new image from the rsync'd source.
#   3. Start the inactive color with the new image.
#   4. Wait for /health OK.
#   5. Swap the nginx upstream to point at the new color.
#   6. Stop the old container.

set -euo pipefail

APP_DIR="${APP_DIR:-/opt/wunest}"
cd "$APP_DIR"

ACTIVE="$(cat .active 2>/dev/null || echo 'none')"
if [[ "$ACTIVE" == "blue" ]]; then
    NEW="green"
    NEW_PORT=9091
    OLD_PORT=9090
else
    NEW="blue"
    NEW_PORT=9090
    OLD_PORT=9091
fi

echo "[deploy] active=$ACTIVE new=$NEW port=$NEW_PORT"

# ── Build image ───────────────────────────────────────
echo "[deploy] docker build ..."
docker build -t "wunest:$NEW" .

# ── Start new container ───────────────────────────────
echo "[deploy] starting wunest-$NEW ..."
docker rm -f "wunest-$NEW" 2>/dev/null || true
docker run -d \
    --name "wunest-$NEW" \
    --restart unless-stopped \
    --network host \
    --env-file "$APP_DIR/.env" \
    -e "HTTP_PORT=$NEW_PORT" \
    "wunest:$NEW"

# ── Wait for health ──────────────────────────────────
echo "[deploy] waiting for /health on :$NEW_PORT ..."
for i in {1..30}; do
    if curl -sf "http://localhost:$NEW_PORT/health" >/dev/null; then
        echo "[deploy] wunest-$NEW healthy"
        break
    fi
    if (( i == 30 )); then
        echo "[deploy] FAILED — wunest-$NEW never became healthy"
        docker logs --tail=50 "wunest-$NEW"
        exit 1
    fi
    sleep 2
done

# ── Swap nginx upstream ──────────────────────────────
echo "[deploy] swapping nginx upstream to :$NEW_PORT ..."
cat > /etc/nginx/conf.d/wunest-upstream.conf <<EOF
upstream wunest_backend { server 127.0.0.1:$NEW_PORT; }
EOF
nginx -t && systemctl reload nginx

# ── Stop old ─────────────────────────────────────────
if [[ "$ACTIVE" != "none" ]]; then
    echo "[deploy] stopping wunest-$ACTIVE ..."
    docker stop "wunest-$ACTIVE" || true
    docker rm "wunest-$ACTIVE" || true
fi

echo "$NEW" > .active
echo "[deploy] done. active=$NEW"
