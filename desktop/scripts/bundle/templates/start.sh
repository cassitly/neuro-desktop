#!/usr/bin/env bash
# Neuro Desktop launcher (bundled). Installed as start.sh.
#
# Starts the server, which serves the dashboard at http://127.0.0.1:8300/ui/ and
# talks to Neuro, and starts the local agent unless this is a split-machine
# setup. Split machine:
#   server:  NEURO_NO_AGENT=1 ./start.sh --executor-listen 0.0.0.0:9876
#   agent:   cd agent && python3 -m controller.agent --bridge <server-ip>:9876
set -euo pipefail

cd "$(dirname "$0")"

export NEURO_UI_DIR="$PWD/frontend"
export NEURO_IPC_FILE="$PWD/neuro_ipc.json"
export NEURO_PERMISSIONS_FILE="$PWD/permissions.json"
export NEURO_CATALOG_FILE="$PWD/catalog/index.json"
export NEURO_GAME_PROFILES_DIR="$PWD/catalog/games"
export NEURO_EXTENSIONS_STATE_FILE="$PWD/catalog/extensions-state.json"
export PYTHONPATH="$PWD/agent${PYTHONPATH:+:$PYTHONPATH}"

SERVER_BIN="$PWD/@SERVER@"
if [[ ! -x "$SERVER_BIN" ]]; then
  echo "Cannot find the server binary at $SERVER_BIN" >&2
  exit 1
fi

echo "Dashboard: http://127.0.0.1:8300/ui/"

if [[ "${NEURO_NO_AGENT:-0}" != "1" ]] && command -v python3 >/dev/null 2>&1; then
  python3 -m controller.agent --bridge "${NEURO_AGENT_BRIDGE:-127.0.0.1:9876}" &
  AGENT_PID=$!
  # The agent must not outlive the server, or the next start finds the port busy.
  trap 'kill "$AGENT_PID" 2>/dev/null || true' EXIT
else
  echo "Agent not started here. On the PC Neuro should control:"
  echo "  cd agent && python3 -m controller.agent --bridge <server-ip>:9876"
fi

exec "$SERVER_BIN" "$@"
