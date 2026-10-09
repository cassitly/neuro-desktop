#!/usr/bin/env bash
# Development bundle: build the server + dashboard, stage the agent and config,
# then run the whole thing from one directory — no Rust, no sudo.
#
#   ./scripts/bundle/dev.sh
#   NEURO_BUNDLE_NO_LAUNCH=1 ./scripts/bundle/dev.sh   # stage only
#
# Staged layout (desktop/dist/dev):
#   neuro-integration      the server: Neuro API client + dashboard API + hub
#   agent/controller/      the agent that executes commands on this machine
#   frontend/              the dashboard client, served at /ui/
#   catalog/, config/, permissions.json
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

echo "=== Development Bundle ==="

# Never run as root: it creates root-owned files that break later non-sudo builds
# and strips X11/Wayland auth so the agent cannot open the display.
if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
  echo "ERROR: do not run this script with sudo." >&2
  echo "  sudo leaves root-owned files under dist/ and frontend/dist," >&2
  echo "  and removes the display authorisation the agent needs." >&2
  exit 1
fi

OS_UNAME="$(uname -s | tr '[:upper:]' '[:lower:]')"
BIN_EXT=""
case "$OS_UNAME" in
  mingw*|msys*|cygwin*) BIN_EXT=".exe" ;;
esac

DIST="dist/dev"
SERVER="neuro-integration$BIN_EXT"

echo "Detected OS: $OS_UNAME"

# A leftover from an old sudo run cannot be cleaned up here; say so instead of
# failing with a confusing "permission denied" halfway through.
if [[ -e "$DIST" && ! -w "$DIST" ]]; then
  echo "ERROR: $DIST is not writable (root-owned from a prior sudo run)." >&2
  echo "  Fix ownership, then re-run as your user:" >&2
  echo "    sudo chown -R \"\$USER\" dist frontend/dist" >&2
  exit 1
fi

echo "Cleaning $DIST..."
rm -rf "$DIST"
mkdir -p "$DIST"

echo "Building the server (apps/neuro-integration)..."
(
  cd apps/neuro-integration
  mkdir -p dist
  # -tags neurodev: only development bundles accept NEURO_EXTENSIONS_ALLOW_UNSIGNED.
  go build -tags neurodev -o "dist/$SERVER" .
)
cp "apps/neuro-integration/dist/$SERVER" "$DIST/$SERVER"

echo "Building the dashboard client (frontend)..."
(
  cd frontend
  if [[ -f package-lock.json ]]; then npm ci; else npm install; fi
  npm run build
)
mkdir -p "$DIST/frontend"
cp -r frontend/dist/. "$DIST/frontend/"

echo "Staging the agent (backend/python/controller)..."
mkdir -p "$DIST/agent"
cp -r backend/python/controller "$DIST/agent/controller"
rm -rf "$DIST/agent/controller/__pycache__"
cp backend/python/requirements.txt "$DIST/agent/requirements.txt"
cp backend/python/requirements-windows.txt "$DIST/agent/requirements-windows.txt" 2>/dev/null || true

echo "Staging config, catalog and docs..."
mkdir -p "$DIST/integration-docs"
cp -r config "$DIST/config"
cp -r catalog "$DIST/catalog"
cp apps/neuro-integration/permissions.example.json "$DIST/permissions.json"
cp "apps/neuro-integration/integration-docs/Action Script Documentation.md" \
   "$DIST/integration-docs/" 2>/dev/null || true

echo
echo "=== Dev bundle complete ==="
echo "Location: $DIST"
echo
echo "1. Set up, then start the server (dashboard API + Neuro client):"
echo "     cd $DIST && ./$SERVER setup && NEURO_UI_DIR=\$PWD/frontend ./$SERVER --ws-url ws://localhost:8000"
echo "   setup prints the dashboard token once: copy it. It is the sign-in token."
echo
echo "2. Start the agent on the PC Neuro controls (same machine here):"
echo "     cd $DIST/agent && python3 -m controller.agent --bridge 127.0.0.1:9876"
echo
echo "   Split machines: run step 1 with --executor-listen 0.0.0.0:9876 and set"
echo "   NEURO_EXECUTOR_TOKEN on both sides, then point the agent at the server's IP."
echo "   The executor link is plain TCP for now: on an untrusted network, tunnel it over SSH."
echo "   The relay host and the server on this machine share ./relay-token, so nothing is copied."
echo "   A relay on another machine needs the same token value: copy the file's value there."
echo
echo "3. Dashboard: http://127.0.0.1:8300/ui/"
echo

if [[ "${NEURO_BUNDLE_NO_LAUNCH:-0}" == "1" ]]; then
  echo "Skipping launch (NEURO_BUNDLE_NO_LAUNCH=1)"
  exit 0
fi

echo "Launching the server (Ctrl-C to stop)..."
cd "$DIST"
export NEURO_UI_DIR="$PWD/frontend"
export NEURO_IPC_FILE="$PWD/neuro_ipc.json"
export NEURO_PERMISSIONS_FILE="$PWD/permissions.json"
export NEURO_CATALOG_FILE="$PWD/catalog/index.json"
export NEURO_GAME_PROFILES_DIR="$PWD/catalog/games"
export NEURO_EXTENSIONS_STATE_FILE="$PWD/catalog/extensions-state.json"
chmod +x "$SERVER"
# First-run step. Idempotent: it prints the dashboard token only the first time.
# A failed check stops the launch here, with the reason printed.
"./$SERVER" setup
exec "./$SERVER"
