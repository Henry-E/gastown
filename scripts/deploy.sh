#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "error: required command not found: $cmd" >&2
    exit 1
  fi
}

require_cmd git
require_cmd make
require_cmd gt
require_cmd jq

echo "==> Deploy root: $ROOT_DIR"
echo "==> Pulling latest main"
git pull --ff-only origin main

echo "==> Rebuilding gt binary"
make install

echo "==> Discovering operational rigs"
mapfile -t OPERATIONAL_RIGS < <(gt rig list --json | jq -r '.[] | select(.status == "operational") | .name')

if [[ ${#OPERATIONAL_RIGS[@]} -eq 0 ]]; then
  echo "No operational rigs found. Nothing to restart."
  exit 0
fi

echo "==> Restarting operational rigs (${#OPERATIONAL_RIGS[@]})"
FAILED=()
for rig in "${OPERATIONAL_RIGS[@]}"; do
  echo " -> gt rig restart $rig"
  if ! gt rig restart "$rig"; then
    FAILED+=("$rig")
  fi
done

if [[ ${#FAILED[@]} -gt 0 ]]; then
  echo "error: failed to restart rigs: ${FAILED[*]}" >&2
  exit 1
fi

echo "==> Deploy complete"
