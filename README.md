# Neuro Desktop

> **An AI-powered desktop control system that gives Neuro-sama the ability to control a computer through natural language commands.**

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![CI](https://github.com/Nakashireyumi/neuro-desktop/actions/workflows/ci.yml/badge.svg)](https://github.com/Nakashireyumi/neuro-desktop/actions/workflows/ci.yml)
[![Version](https://img.shields.io/badge/version-0.0.3b--dev-blue.svg)]()

## Table of Contents

- [Overview](#overview)
- [Features](#features)
- [Architecture](#architecture)
- [Quick Start](#quick-start)
- [Installation](#installation)
- [Usage](#usage)
- [Development](#development)
- [Documentation](#documentation)
- [Contributing](#contributing)
- [License](#license)

## Overview

Neuro Desktop is a multi-language integration system that enables [Neuro-sama](https://twitch.tv/vedal987) to interact with desktop environments through a sophisticated action scripting language. The system bridges AI decision-making with real-world computer control through a carefully designed architecture that prioritizes safety, reliability, and human-like interaction patterns.

### What Makes Neuro Desktop Special?

- **Multi-Language Architecture**: Combines Rust (system integration), Go (API communication), Python (cross-platform control), and C++ (process management) for optimal performance
- **Human-Like Mouse Movement**: Advanced algorithmic pathfinding that mimics natural human mouse movements with Bézier curves and Perlin noise
- **Powerful Script Language**: Simple yet expressive action scripting for complex automation tasks
- **Automatic Recovery**: Built-in crash detection and automatic process restart capabilities
- **Cross-Platform**: Windows, Linux, and macOS — high-level intents resolve to OS-native shortcuts
- **Operator Controls**: Vedal can gate capabilities via scoped permission policies

## Features

See **[docs/CAPABILITIES.md](docs/CAPABILITIES.md)** for an honest “what works today” list
(bridge/executor, actions, permissions, platforms, and what’s still stubbed).

### Core Capabilities

- Mouse / keyboard control with human-like pathfinding
- Action script language for multi-step workflows
- Bridge ↔ executor over TCP (same PC or remote controlled machine)
- Scoped permission policies for Vedal / operators
- Cross-platform intents (Windows-solid; Linux/macOS best-effort)
- Local Ollama+RWKV7 Neuro API mock for integration testing

## Architecture

Neuro Desktop is a **bridge + executor** stack. Per the
[Neuro SDK](https://github.com/VedalAI/neuro-sdk), this app is a WebSocket
**client** of Neuro's API server. Internally:

- **Bridge (server)** — `neuro-integration` talks to Neuro, enforces permissions,
  listens for executor clients on TCP `:9876`, and serves operator admin HTTP on `:8300`.
- **Executor (client)** — `neuro-desktop` + Python run on the machine being controlled
  (`--executor --server host:9876`). Can be a different PC than the bridge.

```
┌──────────────────┐     Neuro WS      ┌─────────────────────────────┐
│  Neuro API       │◄─────────────────►│  Bridge (Go)                │
│  (Vedal)         │                   │  + admin :8300              │
└──────────────────┘                   │  + executor hub :9876       │
                                       └──────────────┬──────────────┘
                                                      │ TCP JSON-lines
                                       ┌──────────────▼──────────────┐
                                       │  Executor (Rust + Python)   │
                                       │  mouse / keyboard / scripts │
                                       └─────────────────────────────┘
```

### Split-machine quick start

```bash
# PC with Neuro / Vedal (bridge)
./neuro-integration --ws-url ws://localhost:8000 --executor-listen 0.0.0.0:9876

# PC Neuro should control (executor) — Omarchy/Linux graphical session, NO sudo
./neuro-desktop --executor --server <bridge-lan-ip>:9876
```

Co-located (default): `./neuro-desktop` still spawns the bridge beside itself.

### Operator dashboard

The bridge serves the dashboard itself — no separate server, no `file://` page:

```
http://127.0.0.1:8300/ui/
```

- **Extensions** — install / enable / disable / uninstall from the catalog
  (`desktop/catalog/index.json`), with the install mode (metadata, git clone, MCP).
- **Games** — profiles found on this machine, the game detected right now, the live
  session (mode, actions issued), "show me what Neuro sees", and a
  **release all input** panic button.
- **Permissions** — scopes (input, game, filesystem, process, network, system,
  vision), per-scope actions-per-minute limits, and explicit allow/deny lists.
  `Save` applies the policy to the running bridge immediately.
- **Status** — version, executor connection, relay peers, reserved actions, paths.

Writes are guarded by `NEURO_ADMIN_TOKEN`. The dashboard pages served to a local
browser carry the token as `window.__ND_BOOTSTRAP`; from another machine, paste it
into the Status tab. Reads stay open on loopback so `curl http://127.0.0.1:8300/api/status`
works while debugging.

Try it without a display, a Windows box, or pyautogui — the repo ships a protocol
simulator that never touches your real mouse or keyboard:

```bash
# Terminal 1: pretend to be the executor (reports "Minecraft" as the active window)
python3 desktop/tools/fake-executor/fake_executor.py --addr 127.0.0.1:9876

# Terminal 2: the bridge + dashboard
cd desktop/apps/neuro-integration
NEURO_UI_DIR=../frontend/dist NEURO_ADMIN_TOKEN=demo go run .
```

### Playing games

Neuro Desktop can play a game two ways, and picks the right one per game:

1. **Alongside a dedicated integration** (Minecraft, osu!, ...). The game keeps its
   own integration; Neuro Desktop does not register the actions that integration
   owns (`NEURO_RESERVED_ACTIONS`, relay peers) and refuses to send input while the
   session is delegated (`control.mode: external`).
2. **On its own**, for anything without an integration: Neuro Desktop supplies the
   high-level interface (`game_list_profiles`, `game_detect`, `game_start_session`,
   `game_move`, `game_look`, `game_action`, `game_press`, `game_observe`, ...), using
   the profile's keybinds and masked keys.

Profiles live in `desktop/catalog/games/*.json` (see
[the profile README](desktop/catalog/games/README.md)); ship one profile per game:

```json
{
  "id": "minecraft",
  "name": "Minecraft",
  "match": { "window_titles": ["Minecraft"], "processes": ["javaw"] },
  "control": { "mode": "auto", "external_integration": "minecraft",
               "mouse_look": { "enabled": true }, "move_hold_seconds": 0.6 },
  "keys": { "forward": "w", "jump": "space" }
}
```

`mode: auto` hands the game to the dedicated integration when it is connected
(through the relay) and drives it from Neuro Desktop otherwise. Override it per
session from the dashboard or with the `mode` parameter of `game_start_session`
(`auto`, `nd`, `external`, `hybrid`). `generic-keyboard-mouse.json` is the fallback
profile for games nobody wrote a profile for.

### Headless machines (no display, command line only)

Neuro Desktop runs on a server, container, SSH session or CI runner with no
graphical session. Nothing has to be installed for it: mouse and keyboard
libraries are loaded only when a display actually exists, and `NEURO_HEADLESS`
forces the mode either way.

```bash
# A CLI-only box: no X/Wayland, no pyautogui/pynput/mss needed
export NEURO_HEADLESS=1                    # optional: auto-detected when DISPLAY is unset
export NEURO_SHELL_ALLOWLIST="ls,cat,python3,git"   # programs Neuro may run (empty = none)
export NEURO_SHELL_TIMEOUT=20              # seconds per command (max 120)
export NEURO_SHELL_CWD=/srv/work
export NEURO_ADMIN_TOKEN="pick-a-secret"   # required before exposing the dashboard

cd desktop/apps/neuro-integration && go run .
```

What changes on a headless machine:

- **`shell_command` is the capability that matters.** Neuro runs one command line
  per call and gets the exit code plus truncated stdout/stderr back, which is what
  a weak model needs to decide the next step: `shell_command {"command": "ls -la"}`.
- Mouse, keyboard, screenshot and game actions answer with a clear message
  ("no display session ... use the shell_command action") instead of crashing the
  controller at import time, which is what used to happen.
- `get_status` still reports platform, processes (from `/proc`, `psutil` optional)
  and screen size; there is simply no cursor or window to report.
- The dashboard, permissions, relay and game *registry* all work exactly as on a
  desktop, so Neuro can be pointed at a headless build during development.

The shell is fenced three times: the `shell` scope (off in every shipped example
policy, and not enabled by `default_allow`), the allowlist/denylist firewall, and
the same checks again inside the Python executor. Destructive patterns (`rm -rf /`,
`mkfs`, `shutdown`, `curl ... | sh`, `sudo`, `diskpart`, ...) are refused even when
the program is allowlisted; `NEURO_SHELL_DENYLIST` adds the operator's own
patterns.

### Seeing what it did (audit log) and stopping it

Two operator safety systems sit in front of every action:

```bash
export NEURO_PAUSED=1                              # start paused
export NEURO_KILL_SWITCH_FILE=/run/nd/STOP         # actions refuse while this file exists
export NEURO_AUDIT_LOG=/var/log/neuro-desktop.jsonl # one JSON line per decision
export NEURO_DENY_ACTIONS="type_text,key_press"     # a hard deny list the dashboard cannot undo
```

- `POST /api/control/pause` and `POST /api/control/resume` toggle the brake from
  the dashboard; `GET /api/control` shows the current state, and input release /
  status / session-end actions are always allowed so nothing stays stuck down.
- The kill-switch file is checked once per second; creating it stops action
  execution immediately and removing it resumes. Actions answer with the reason
  and the file name, so a model knows to wait rather than retry.
- `GET /api/audit?limit=100` returns the tail of the audit log
  (`{"time","event","action","decision","reason"}`); `NEURO_DENY_ACTIONS` is
  merged into the policy at load time and survives dashboard edits.

### Component Breakdown

| Component | Language | Role |
|-----------|----------|------|
| **neuro-integration** | Go | Bridge: Neuro API, permissions, game interface, dashboard API |
| **neuro-desktop** | Rust | Executor orchestrator, IPC, process lifecycle |
| **controller** | Python | Input control, script parsing, platform intents |
| **frontend** | TypeScript | Operator dashboard, served by the bridge at `/ui/` |
| **process-handler** | C++ | Optional multi-process supervisor |

## Quick Start

### Prerequisites

```bash
# Rust 1.70+
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh

# Go 1.22+
# Download from https://go.dev/dl/

# Python 3.10+
python --version

# Node.js 18+ (for frontend)
node --version
```

### Installation

**Option 1: Pre-built Binaries** (Recommended)

1. Download the latest release from [Releases](https://github.com/Nakashireyumi/neuro-desktop/releases)
2. Extract the archive
3. Run `neuro-desktop.exe` (Windows) or `./neuro-desktop` (Linux/macOS)

**Option 2: Build from Source**

```bash
# Clone repository
git clone https://github.com/Nakashireyumi/neuro-desktop.git
cd neuro-desktop/desktop

# Run automated build
.\scripts\build-all.ps1  # Windows
./scripts/build-all.sh   # Linux/macOS
```

### First Run

```bash
# Windows
cd apps/neuro-desktop/target/release
.\neuro-desktop.exe

# Linux/macOS
cd apps/neuro-desktop/target/release
./neuro-desktop
```

The system will automatically:
1. ✅ Initialize Python controllers
2. ✅ Start IPC handler
3. ✅ Launch Go integration
4. ✅ Connect to Neuro API (default: `ws://localhost:8000`)

## Usage

### Basic Example

Once running, Neuro can execute commands like:

```javascript
// Move mouse to center of screen
{
  "action": "move_mouse_to",
  "params": { "x": 960, "y": 540 }
}

// Type text
{
  "action": "type_text",
  "params": { "text": "Hello from Neuro!" }
}

// Execute complex script
{
  "action": "run_script",
  "params": {
    "script": `
      TYPE "notepad"
      ENTER
      WAIT 1
      TYPE "Hello World!"
    `
  }
}
```

### Action Script Language

The script language supports powerful multi-command sequences:

```text
# Open application
SHORTCUT win
TYPE "notepad"
ENTER
WAIT 1

# Type content
TYPE "Dear User,"
ENTER
TYPE "This is Neuro!"
ENTER

# Save file
SHORTCUT ctrl s
WAIT 0.5
TYPE "neuro_message.txt"
ENTER
```

See [Action Script Documentation](docs/action_script/LANGUAGE_REFERENCE.md) for complete reference.

### Configuration

Edit `config/integration-config.yml`:

```yaml
connection:
  neuro-backend: "ws://localhost:8000"
  
package:
  name: "neuro-desktop"
  version: "0.0.3b-dev"
```

Or use environment variables:

```bash
# Windows
$env:NEURO_SDK_WS_URL = "ws://localhost:8000"
$env:NEURO_IPC_FILE = "./neuro_ipc.json"
$env:NEURO_PERMISSIONS_FILE = "./desktop/apps/neuro-integration/permissions.example.json"
$env:NEURO_RELAY_ENABLED = "true"
$env:NEURO_RELAY_EMULATED_ADDR = "127.0.0.1:8001"
$env:NEURO_RELAY_NAME = "Neuro Desktop Hub"
$env:NEURO_CATALOG_FILE = "./desktop/catalog/index.json"
$env:NEURO_CONTEXT_POLL_SECONDS = "15"
$env:NEURO_CONTEXT_CAPTURE_SCREENSHOT = "false"
$env:NEURO_VISION_SERVER_URL = "http://127.0.0.1:8080/infer"
$env:NEURO_EXTENSION_INSTALL_MODE = "metadata_only"
$env:NEURO_EXTENSION_DIR = "./plugins"
$env:NEURO_UI_LAUNCH = "true"
$env:NEURO_UI_DIR = "./desktop/frontend/dist"      # dashboard build to serve
$env:NEURO_ADMIN_TOKEN = "change-me"               # required for dashboard writes
$env:NEURO_EXECUTOR_TOKEN = "change-me"            # required when the hub is exposed
$env:NEURO_GAME_PROFILES_DIR = "./desktop/catalog/games"
$env:NEURO_RESERVED_ACTIONS = "move_mouse_to,osu_click"   # owned by other integrations
$env:NEURO_HEADLESS = ""                     # 1 = no display (auto-detected on Linux without DISPLAY)
$env:NEURO_SHELL_ALLOWLIST = "ls,python3"    # programs shell_command may run (empty = none)
$env:NEURO_SHELL_TIMEOUT = "20"              # seconds per command (max 120)
$env:NEURO_AUDIT_LOG = "./neuro-desktop.jsonl"
$env:NEURO_KILL_SWITCH_FILE = "./STOP"       # actions refuse while this file exists
$env:NEURO_DENY_ACTIONS = "type_text"        # hard deny list the dashboard cannot undo

# Linux/macOS
export NEURO_SDK_WS_URL="ws://localhost:8000"
export NEURO_IPC_FILE="./neuro_ipc.json"
export NEURO_PERMISSIONS_FILE="./desktop/apps/neuro-integration/permissions.example.json"
export NEURO_RELAY_ENABLED="true"
export NEURO_RELAY_EMULATED_ADDR="127.0.0.1:8001"
export NEURO_RELAY_NAME="Neuro Desktop Hub"
export NEURO_CATALOG_FILE="./desktop/catalog/index.json"
export NEURO_CONTEXT_POLL_SECONDS="15"
export NEURO_CONTEXT_CAPTURE_SCREENSHOT="false"
export NEURO_VISION_SERVER_URL="http://127.0.0.1:8080/infer"
export NEURO_EXTENSION_INSTALL_MODE="metadata_only"
export NEURO_EXTENSION_DIR="./plugins"
export NEURO_UI_LAUNCH="true"
export NEURO_UI_DIR="./desktop/frontend/dist"
export NEURO_ADMIN_TOKEN="change-me"
export NEURO_EXECUTOR_TOKEN="change-me"
export NEURO_GAME_PROFILES_DIR="./desktop/catalog/games"
export NEURO_RESERVED_ACTIONS="move_mouse_to,osu_click"
export NEURO_HEADLESS=""                     # 1 = no display (auto-detected on Linux without DISPLAY)
export NEURO_SHELL_ALLOWLIST="ls,python3"    # programs shell_command may run (empty = none)
export NEURO_SHELL_TIMEOUT="20"              # seconds per command (max 120)
export NEURO_AUDIT_LOG="./neuro-desktop.jsonl"
export NEURO_KILL_SWITCH_FILE="./STOP"       # actions refuse while this file exists
export NEURO_DENY_ACTIONS="type_text"        # hard deny list the dashboard cannot undo
```

Use [`permissions.example.json`](desktop/apps/neuro-integration/permissions.example.json) as a starting policy (it ships with the
`game` scope enabled, `game_launch` denied and the `shell` scope off, so Neuro can play
but cannot start programs or run command lines).
A scope value may be written as `"game": true` or `"game": {"allowed": true, "limits": {"max_actions_per_minute": 120}}`.
Use `NEURO_EXTENSION_INSTALL_MODE=git_clone` if you want extension installation to clone repositories from GitHub.

## Development

### Local Neuro API tester (Ollama + RWKV7)

In-repo stand-in for [Randy](https://github.com/VedalAI/neuro-sdk/tree/main/Randy) that
can also pick actions with [heredos/rwkv7:2.9b](https://ollama.com/heredos/rwkv7):

```bash
cd desktop/tools/ollama-neuro
./setup.sh
./run.sh --mode ollama --warm    # ws://127.0.0.1:8000 + http://127.0.0.1:1337/
# Instant IPC debugging without waiting on the LLM:
./run.sh --mode manual
```

On CPU laptops (e.g. Dell Latitude E7490), cold model load can take minutes —
use `--warm` / `--keep-alive -1`, or stay on `manual`/`random` while wiring IPC.
Details: [`desktop/tools/ollama-neuro/README.md`](desktop/tools/ollama-neuro/README.md).

### Fake executor (no display needed)

`desktop/tools/fake-executor/fake_executor.py` speaks the executor protocol against
the bridge and never generates input, so the dashboard, the permission checks and
the game interface can be exercised on any machine (including CI and macOS):

```bash
python3 desktop/tools/fake-executor/fake_executor.py --addr 127.0.0.1:9876
```

Run it before the bridge to see the executor as *connected* and a game as detected.

### Docker Modular Tests

Run modular tests in Docker:

```bash
docker compose -f docker-compose.tests.yml run --rm go-integration-tests
docker compose -f docker-compose.tests.yml run --rm python-parser-tests
```

### Coexistence with other integrations (Neuro Relay)

[Neuro Relay](https://github.com/Nakashireyumi/neuro-relay) multiplexes several
integrations behind one Neuro connection. Neuro Desktop participates as a relay
*integration* — it does not spawn the relay, which is a Python service with its own
YAML config:

```bash
export NEURO_RELAY_ENABLED=true
export NEURO_RELAY_URL="ws://127.0.0.1:8765"     # relay intermediary socket
export NEURO_RELAY_TOKEN="super-secret-token"    # intermediary.auth_token
export NEURO_RELAY_NAME="Neuro Desktop"
export NEURO_RESERVED_ACTIONS="minecraft_place_block,osu_click"
```

There are two ways to coexist, and they are complementary:

1. **Share one Neuro connection (recommended for games).** Point the bridge's own
   Neuro API client at the relay's Nakurity Backend instead of at Neuro
   (`NEURO_SDK_WS_URL=ws://127.0.0.1:8001`, the `nakurity-backend` port in the
   relay's `authentication.yaml`). The bridge then looks like any other Neuro SDK
   client to the relay: it sends `startup` (`game: "Neuro Desktop"`) and
   `actions/register`, and the relay multiplexes everything to the real backend.
   This is the path that makes "run alongside an existing game integration" work
   with no extra configuration.
2. **Look in on / be driven by the relay (operator visibility).** With
   `NEURO_RELAY_ENABLED=true` the bridge registers on the relay's intermediary
   socket (`ws://127.0.0.1:8765`) as `{"type":"integration","name":...,
   "auth_token":...}`, publishes its action schemas, and accepts `cmd` envelopes
   from Neuro-OS watchers — each one still subject to the same permission policy,
   pause flag and kill switch as Neuro's own calls. Note that the relay's
   intermediary keeps registrations for watchers; it does not forward them to
   Neuro, which is exactly why mode 1 exists.

### Upstream relay compatibility (it does not start as-is)

Worth knowing before you debug your setup: **Nakashireyumi/neuro-relay cannot
start with any published `neuro-api` release.** `src/dev/nakurity/server.py`
subclasses three abstract server classes but implements only part of the
interface, so Python refuses to instantiate it:

```text
TypeError: Can't instantiate abstract class NakurityBackend with abstract methods
get_character_id, get_websocket_session_id
```

Checked against every release on PyPI: 0.x/1.x have no `neuro_api.server` module
at all, and 2.x/3.x/4.x leave `get_next_id`, `handle_actions_register`,
`handle_actions_unregister`, `handle_actions_force`, `handle_action_result` (plus
the two above in 4.x) abstract. This is upstream's bug, not a configuration
problem — the relay's own protocol code is fine.

`desktop/tools/relay-compat/run_relay.py` starts the relay unchanged by filling
in exactly those methods before `dev.nakurity.__main__` runs:

```bash
pip install "websockets==13.1" neuro-api pyyaml
python3 desktop/tools/relay-compat/run_relay.py /path/to/neuro-relay/src
# [relay-compat] filled in 2 abstract method(s): get_character_id, get_websocket_session_id
# [Intermediary] listening on ws://127.0.0.1:8765
# [Nakurity Backend] Starting websocket server on ws://127.0.0.1:8001
```

Nothing on disk is patched and no fork is maintained: it is a shim for a broken
upstream entry point, and it disappears the day upstream fixes their class.

Reserved names are neither registered with Neuro nor accepted from the dashboard
(the dashboard refuses to add them to the allow list): they belong to another
integration, and shadowing them is how two integrations end up fighting over the
same key. When Neuro asks to play a game that a relay peer owns, the refusal names
that peer's registered actions instead of leaving the model stuck:

```
game_move -> "this game is controlled by the "minecraft" integration ...
               Its registered actions are: minecraft.move_forward, minecraft.jump, ..."
```

Relay status (peers, their actions, reserved names, last error) is on the
dashboard's Status tab and at `/api/relay`. The protocol shapes are pinned by
`relay_protocol_test.go`, which speaks what `intermediary.py` actually speaks.

### Supervised Runtime (Process Handler)

The process handler can now supervise ND and integration workers directly:

```bash
# From dist bundle folder
./process-handler.exe
```

`process-handler` starts `neuro-desktop.exe --supervised` and launches `neuro-integration.exe` itself.
If `neuro-relay.exe` exists in the same folder, it is also supervised and the integration is routed through relay automatically.

### Project Structure

```
desktop/
├── apps/
│   ├── neuro-desktop/          # Main Rust application
│   │   └── src/
│   │       ├── main.rs          # Entry point
│   │       ├── controller.rs    # Python FFI bridge
│   │       ├── ipc_handler.rs   # IPC command processor
│   │       └── go_manager.rs    # Go process manager
│   │
│   └── neuro-integration/      # Go WebSocket client
│       ├── main.go
│       ├── action-handling.go
│       ├── action-registry.go
│       └── types.go
│
├── backend/python/controller/  # Python control drivers
│   ├── lib.py                  # Entry point
│   ├── actions.py              # Script parser
│   ├── controls/
│   │   ├── mouse.py            # Mouse controller
│   │   └── keyboard.py         # Keyboard controller
│   └── libraries/
│       └── mouse_pathfinder.py # Human-like motion
│
├── frontend/                   # Web UI (TypeScript/Vite)
├── config/                     # Configuration files
├── scripts/                    # Build and bundle scripts
└── docs/                       # Documentation
```

### Development Workflow

```bash
# 1. Setup development environment
.\scripts\setup-dev.ps1

# 2. Build all components
make all

# 3. Run in development mode
.\scripts\bundle\dev.ps1

# 4. Run tests
cargo test                      # Rust tests
go test ./...                   # Go tests
pytest backend/python/          # Python tests

# 5. Build production bundle
.\scripts\bundle\prod.ps1
```

### Adding New Actions

1. **Define action schema** in `action-registry.go`:

```go
var MyActionSchema = ActionDefinition{
    Name: "my_action",
    Description: "Does something cool",
    Schema: map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "param1": map[string]interface{}{
                "type": "string",
                "description": "A parameter",
            },
        },
        "required": []string{"param1"},
    },
}
```

2. **Handle action** in `action-handling.go`:

```go
case string(CmdMyAction):
    param1, _ := params["param1"].(string)
    cmd = IPCCommand{
        Type: CmdMyAction,
        Params: map[string]interface{}{
            "param1": param1,
        },
    }
```

3. **Implement in Rust** (`ipc_handler.rs`):

```rust
IPCCommand::MyAction { params } => {
    controller.my_action(&params.param1)
}
```

4. **Add Python implementation** if needed (`controller/`).

### Testing with Randy

Randy is a mock Neuro API server for testing:

```bash
# Terminal 1: Start Randy
cd Randy
npm install
npm start

# Terminal 2: Run Neuro Desktop
cd apps/neuro-desktop/target/release
.\neuro-desktop.exe
```

Randy will send random actions to test your integration.

## Documentation

- ✅ [Current Capabilities](docs/CAPABILITIES.md) — what works today (honest)
- 🐣 [Driving it with a small/weak model](docs/LLM_GUIDE.md) — prompting tactics and parameter shapes
- 🛡️ [Safety systems and firewalls](docs/SAFETY.md) — every guard, where it lives, how to verify it
- 📖 [Action Script Language Reference](docs/action_script/LANGUAGE_REFERENCE.md)
- 🏗️ [Architecture Deep Dive](docs/ARCHITECTURE.md)
- 🔧 [API Specification](desktop/apps/neuro-integration/integration-docs/Action Script Documentation.md)
- 🚀 [Deployment Guide](docs/DEPLOYMENT.md)
- 🧪 [Ollama Neuro Tester](desktop/tools/ollama-neuro/README.md)
- 🤝 [Contributing Guidelines](CONTRIBUTING.md)
- 🧭 [Project Vision](VISION.md)
- 🗺️ [Production TODO](docs/PRODUCTION_TODO.md)
- 📝 [Changelog](CHANGELOG.md)
- 🤝 [Code of Conduct](CODE_OF_CONDUCT.md)
- 🔐 [Security Policy](SECURITY.md)

## Troubleshooting

### Common Issues

**`EACCES` / Permission denied on `frontend/dist`**

Leftover from a `sudo` bundle. Fix ownership, never rebuild as root:

```bash
cd desktop
sudo chown -R "$USER:$USER" frontend/dist dist apps/neuro-desktop/target backend/python/.venv
./scripts/bundle/dev.sh
```

**"Go integration binary not found"**
```bash
# Rebuild Go integration
cd apps/neuro-integration
go build -o neuro-integration.exe .
cp neuro-integration.exe ../neuro-desktop/target/release/
```

**"Failed to initialize Python controller"**
```bash
# Reinstall Python dependencies
cd backend/python
python -m venv .venv
.venv\Scripts\activate
pip install -r requirements.txt
```

**"WebSocket connection failed"**
```bash
# Check if Randy or Neuro API is running
curl ws://localhost:8000
# Or start Randy
cd Randy && npm start
```

See [Troubleshooting Guide](docs/TROUBLESHOOTING.md) for more solutions.

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

### Areas We Need Help

- 🐛 Bug reports and fixes
- 📝 Documentation improvements
- ✨ New action types
- 🧪 Test coverage
- 🌐 Cross-platform testing
- 🎨 UI/UX improvements

## Roadmap

- [ ] **v0.1.0**: Core functionality (current)
- [ ] **v0.2.0**: Enhanced safety features
- [ ] **v0.3.0**: Vision system integration
- [ ] **v0.4.0**: Advanced macro system
- [ ] **v1.0.0**: Production-ready release

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- **Neuro-sama** - The AI that makes this all worthwhile
- **Vedal** - Creator of Neuro-sama
- The community for testing and feedback

## Support

- 💬 [Discord](https://discord.gg/neuro)
- 🐛 [Issue Tracker](https://github.com/Nakashireyumi/neuro-desktop/issues)
- 📧 Email: support@neuro-desktop.dev

---

**Made with ❤️ by the Neuro Desktop Team**

*"Giving Neuro the keys to the desktop, one action at a time."*

