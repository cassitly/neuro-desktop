#!/usr/bin/env bash
# Build everything the shipped product needs on Linux / macOS / Git Bash / WSL.
#
# The shipped product is: a Go server (the bridge) + a Python agent (the process
# that touches the machine) + a TypeScript dashboard that the server serves. The
# Rust executor and the C++ supervisor are *not* part of it; they are kept as
# reference/optional code and only built with the flags below.
#
#   ./scripts/build-all.sh                  # server + dashboard + agent check
#   ./scripts/build-all.sh --with-legacy    # also build the pieces above
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

WITH_LEGACY=0
for arg in "$@"; do
  case "$arg" in
    --with-legacy|--legacy) WITH_LEGACY=1 ;;
    -h|--help) sed -n '2,12p' "$0"; exit 0 ;;
    *) echo "unknown argument: $arg" >&2; exit 2 ;;
  esac
done

OS_UNAME="$(uname -s | tr '[:upper:]' '[:lower:]')"
BIN_EXT=""
case "$OS_UNAME" in
  mingw*|msys*|cygwin*) BIN_EXT=".exe" ;;
esac

echo "=== neuro-desktop build ($OS_UNAME) ==="

echo "[1/3] Go server (apps/neuro-integration)"
mkdir -p apps/neuro-integration/dist
(
  cd apps/neuro-integration
  go build -o "dist/neuro-integration${BIN_EXT}" .
)
echo "      -> apps/neuro-integration/dist/neuro-integration${BIN_EXT}"

echo "[2/3] Dashboard client (frontend)"
(
  cd frontend
  if [[ -f package-lock.json ]]; then
    npm ci
  else
    npm install
  fi
  npm run build
)
echo "      -> frontend/dist"

echo "[3/3] Python agent + drivers"
python3 -m compileall -q backend/python/controller >/dev/null
echo "      -> backend/python/controller (no build step; syntax checked)"

STEPS=3
if [[ "$WITH_LEGACY" == "1" ]]; then
  echo "[legacy] Rust executor (apps/neuro-desktop)"
  (cd apps/neuro-desktop && cargo build --release) || echo "      ! cargo unavailable — skipped"

  echo "[legacy] C++ supervisor (apps/process-handler)"
  if command -v cmake >/dev/null 2>&1; then
    cmake -S apps/process-handler -B apps/process-handler/build -DBUILD_TESTS=OFF >/dev/null
    cmake --build apps/process-handler/build --config Release --parallel >/dev/null
  else
    echo "      ! cmake not found — skipped (the supervisor is not shipped anyway)"
  fi
fi

echo
echo "Build complete."
echo "  Bundle + run (dev):   ./scripts/bundle/dev.sh"
echo "  Release bundle:       ./scripts/bundle/prod.sh"
echo "  Server binary:        apps/neuro-integration/dist/neuro-integration${BIN_EXT}"
echo "  Dashboard client:     frontend/dist"
echo "  Agent:                python3 -m controller.agent --bridge 127.0.0.1:9876   (from desktop/backend/python)"
