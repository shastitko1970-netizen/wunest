#!/usr/bin/env bash
# Flip the nginx upstream back to the other color without rebuilding.
# Useful when a just-deployed version breaks and the previous container is
# still running idle.

set -euo pipefail

APP_DIR="${APP_DIR:-/opt/wunest}"
cd "$APP_DIR"

ACTIVE="$(cat .active 2>/dev/null || echo 'none')"
if [[ "$ACTIVE" == "blue" ]]; then
    OTHER="green"
    OTHER_PORT=9091
else
    OTHER="blue"
    OTHER_PORT=9090
fi

echo "[rollback] current=$ACTIVE → switching to $OTHER (:$OTHER_PORT)"

if ! docker ps --format '{{.Names}}' | grep -q "^wunest-$OTHER\$"; then
    echo "[rollback] wunest-$OTHER is not running — cannot roll back."
    exit 1
fi

if ! curl -sf "http://localhost:$OTHER_PORT/health" >/dev/null; then
    echo "[rollback] wunest-$OTHER is not healthy — aborting."
    exit 1
fi

cat > /etc/nginx/conf.d/wunest-upstream.conf <<EOF
upstream wunest_backend { server 127.0.0.1:$OTHER_PORT; }
EOF
nginx -t && systemctl reload nginx

echo "$OTHER" > .active
echo "[rollback] done. active=$OTHER"
