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

# Also tag as :latest so out-of-band tooling (the storage-reaper
# systemd unit, ad-hoc `docker run wunest:latest /app/...` invocations)
# always gets the freshest image regardless of blue/green color.
docker tag "wunest:$NEW" wunest:latest

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
# CRITICAL: if nginx fails to reload we must NOT stop the old container,
# otherwise we're left with a broken upstream and the old target gone.
# We only proceed to the stop step once nginx is serving the new color.
echo "[deploy] swapping nginx upstream to :$NEW_PORT ..."
cat > /etc/nginx/conf.d/wunest-upstream.conf <<EOF
upstream wunest_backend { server 127.0.0.1:$NEW_PORT; }
EOF
if ! nginx -t 2>&1; then
    echo "[deploy] FAILED — nginx config invalid; reverting upstream and keeping $ACTIVE active"
    cat > /etc/nginx/conf.d/wunest-upstream.conf <<EOF
upstream wunest_backend { server 127.0.0.1:$OLD_PORT; }
EOF
    # Old container still up, so traffic keeps flowing. Kill the new one.
    docker rm -f "wunest-$NEW" >/dev/null 2>&1 || true
    exit 1
fi
systemctl reload nginx

# ── Stop old ─────────────────────────────────────────
if [[ "$ACTIVE" != "none" ]]; then
    echo "[deploy] stopping wunest-$ACTIVE ..."
    docker stop "wunest-$ACTIVE" || true
    docker rm "wunest-$ACTIVE" || true
fi

echo "$NEW" > .active
echo "[deploy] done. active=$NEW"
