#!/usr/bin/env bash
# Install / reinstall the WuNest storage-reaper systemd timer.
#
# Run once on the production server after a fresh `make deploy` that
# includes the nest-storage-reaper binary (any deploy since M33
# follow-up). Re-running is safe — `systemctl daemon-reload` + `enable`
# both idempotent.
#
# Usage: on the server, from /opt/wunest:
#   sudo bash scripts/systemd/install-reaper.sh
#
# Verification after install:
#   systemctl status nest-storage-reaper.timer
#   systemctl list-timers nest-storage-reaper.timer
#   journalctl -u nest-storage-reaper.service -f  # while trigger firing
#
# Dry-run a single invocation without waiting for the timer:
#   sudo systemctl start nest-storage-reaper.service

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UNIT_DIR="/etc/systemd/system"

echo "[reaper] installing unit files into $UNIT_DIR ..."
install -m 0644 "$SCRIPT_DIR/nest-storage-reaper.service" "$UNIT_DIR/"
install -m 0644 "$SCRIPT_DIR/nest-storage-reaper.timer"   "$UNIT_DIR/"

echo "[reaper] reloading systemd ..."
systemctl daemon-reload

echo "[reaper] enabling + starting timer ..."
systemctl enable --now nest-storage-reaper.timer

echo "[reaper] done. Next fire:"
systemctl list-timers nest-storage-reaper.timer --no-pager | head -4
