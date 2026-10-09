# Neuro Desktop — desktop control for Neuro-sama

Neuro Desktop lets Neuro (or Evil) use a real computer: move the mouse, press
keys, run scripts and shell commands, play games that have no integration of
their own, and read the desktop back as context. Vedal/you get a dashboard to
watch it, grant permissions, and pause or kill anything at any time.

## The three pieces

```
┌─────────────────────────────┐        ┌──────────────────────────────┐
│  Neuro API (ws://…)         │        │  Dashboard client (browser)  │
└──────────┬──────────────────┘        └───────────┬──────────────────┘
           │ neuro-sdk (Go)                        │ HTTP, same-origin /ui/
┌──────────▼───────────────────────────────────────▼──────────────────┐
│  SERVER — desktop/apps/neuro-integration (Go)                        │
│  • talks to Neuro (actions, context, results)                        │
│  • serves the dashboard API and the UI bundle                        │
│  • owns the permission policy, audit log, kill switch, game catalog  │
│  • forwards machine commands to the agent                            │
└──────────┬───────────────────────────────────────────────────────────┘
           │ executor protocol (JSON lines) or file IPC
┌──────────▼───────────────────────────────────────────────────────────┐
│  AGENT — desktop/backend/python/controller/agent.py (Python)         │
│  • runs on the PC Neuro controls (usually the same machine)          │
│  • mouse/keyboard via pyautogui, screen capture, windows, shell      │
│  • refuses input on a headless machine and says why                  │
└──────────────────────────────────────────────────────────────────────┘
```

The server executes *decisions*; the agent executes *actions*. The admin
dashboard is a client of the server. See `docs/ARCHITECTURE.md` for why the
project was consolidated to Go + Python (and what was removed), and
`docs/EXECUTOR_PROTOCOL.md` for the wire protocol between the two.

## Quick start

```bash
# 1. Server (on the PC that runs Neuro)
cd desktop/apps/neuro-integration
go build -o neuro-integration .
NEURO_SDK_WS_URL=ws://127.0.0.1:8000 ./neuro-integration

# 2. Agent (on the PC Neuro should control — same machine in dev)
cd desktop/backend/python
python3 -m controller.agent --bridge 127.0.0.1:9876

# 3. Dashboard
xdg-open http://127.0.0.1:8300/ui/      # macOS: open, Windows: start
```

Everything can also be built and staged in one step:

```bash
./scripts/build-all.sh            # server + dashboard + agent syntax check
./scripts/bundle/dev.sh           # stages dist/dev and launches the server
NEURO_BUNDLE_SKIP_VENV=1 ./scripts/bundle/prod.sh   # release bundle in dist/neuro-desktop
```

The bundle's `start.sh` / `start.bat` launch the server and the local agent
together; `NEURO_NO_AGENT=1` skips the agent for a split-machine setup.

## Layout

```
desktop/
├── apps/
│   ├── neuro-integration/    SERVER (Go): Neuro client, dashboard API, executor hub, policy,
│   │                         relay host (`relay` subcommand), MCP bridge, signed catalog
│   └── nd-vision-server/     optional vision service (Python, stdlib HTTP; Pillow optional)
├── backend/python/
│   ├── controller/           AGENT + drivers (agent.py, actions.py, controls/, shell.py)
│   └── tests/                unittest suite for the agent and the safety rules
├── frontend/                 DASHBOARD CLIENT (TypeScript + Vite, no framework)
├── catalog/                  game profiles, catalog index, live extension state
├── config/                   example policy (no config file: settings are env vars)
├── scripts/                  build-all.*, build-go.ps1, bundle/{dev,prod}.*, templates/
├── tools/
│   ├── ci/repo_checks.py     dependency-free repository checks (what CI runs first)
│   ├── fake-executor/        protocol simulator for the server (no input is ever generated)
│   └── ollama-neuro/         local small-model brain for testing the action surface
└── docs/ (repo root docs/)   ARCHITECTURE, EXECUTOR_PROTOCOL, LLM_GUIDE, SAFETY, CAPABILITIES
```

## Configuration

| Variable | What it does |
| --- | --- |
| `NEURO_SDK_WS_URL` / `-ws-url` | Neuro API websocket (`ws://localhost:8000` by default) |
| `NEURO_ADMIN_LISTEN` | dashboard address (`127.0.0.1:8300`) |
| `NEURO_ADMIN_TOKEN` | token required for destructive dashboard calls |
| `NEURO_EXECUTOR_LISTEN` / `-executor-listen` | hub address agents connect to (`127.0.0.1:9876`) |
| `NEURO_EXECUTOR_TOKEN` | shared secret every agent must present |
| `NEURO_IPC_FILE` | file-IPC fallback path for a co-located agent |
| `NEURO_PERMISSIONS_FILE` | policy file (see `config/permissions.example.json`) |
| `NEURO_HEADLESS` | `1`/`true` on a machine with no display session |
| `NEURO_SHELL_ALLOWLIST` | programs the shell action may run (`ls,echo,python3`) |
| `NEURO_DENY_ACTIONS` | actions that never run (`type_text`) |
| `NEURO_AUDIT_LOG` | JSON-lines audit file |
| `NEURO_RELAY_ENABLED` / `NEURO_RELAY_URL` / `NEURO_RELAY_TOKEN` | connect the server to a relay as an integration (`neuro-integration relay`, or the upstream relay) |
| `NEURO_RELAY_LISTEN` / `NEURO_RELAY_AUTH_TOKEN` / `NEURO_RELAY_TOKEN_FILE` | the relay host's socket, token, and the file a generated token is written to (mode 0600) |
| `NEURO_RELAY_HEALTH_LISTEN` / `NEURO_RELAY_NEURO_URL` / `NEURO_RELAY_NEURO_OS_TOKEN` | the relay host's health endpoint, the optional Neuro link for `direct_to_neuro`, and the enhanced watcher token |
| `NEURO_VISION_URL` / `NEURO_VISION_TOKEN` | the vision service `game_observe` calls (`NEURO_VISION_SERVER_URL` is still read as an alias) |
| `NEURO_CATALOG_FILE` / `NEURO_PUBLISHERS_FILE` | the signed extension index and the publisher keys the server trusts |
| `NEURO_EXTENSION_DIR` / `NEURO_EXTENSION_INSTALL_MODE` | where installed extensions live, and `metadata_only` (default) or `git_clone` |
| `NEURO_EXTENSIONS_ALLOW_UNSIGNED` | **dangerous**: install extensions whose signature does not verify. Leave it unset |
| `NEURO_GAME_PROFILES_DIR`, `NEURO_CATALOG_FILE` | game profiles and catalog index |
| `NEURO_RELAY_*` | optional Neuro Relay participation (see `docs/RELAY.md`) |
| `NEURO_VISION_URL` | vision server used by `game_observe` |

## Actions

`GET /api/actions` lists every action with its `kind`, its JSON `schema`, and a
plain-language `params` hint (for example `direction` with the allowed values,
which params are required). The same schemas are registered with Neuro.
`docs/LLM_GUIDE.md` explains how to prompt a small model so it uses them
correctly; `docs/CAPABILITIES.md` is the full feature list.

Roughly: mouse/keyboard primitives, script and shell execution, desktop shell
intents (`open_windows_menu`, `show_desktop`, …), catalog and extension
management, telemetry (`get_status`, `get_desktop_context`), and the high-level
game interface (`game_list_profiles`, `game_start_session`, `game_move`, …).

## Safety

Every action goes through the same pipeline: kill switch and pause state, deny
list, permission policy (per-scope budgets and explicit consent for `shell` and
`system`), rate limit, the shell firewall — all logged to the audit file, and all
flippable live from the dashboard. `docs/SAFETY.md` lists each guard, where it
lives, and how to verify it.

## Headless / command-line-only machines

Set `NEURO_HEADLESS=1` (or just have no `DISPLAY`/`WAYLAND_DISPLAY`). Input
actions then fail fast with an explanation, `get_status` reports `headless:
true`, and the shell capability becomes the useful surface — still bounded by
`NEURO_SHELL_ALLOWLIST`, `NEURO_SHELL_DENY_PATTERNS` and the 20-second default
timeout.

## Testing

```bash
# server (Go)
cd desktop/apps/neuro-integration && go test ./...          # add -race when cgo is available

# agent + safety rules (Python, no third-party deps needed)
cd desktop/backend/python && python3 -m unittest discover -s tests -t .

# everything CI runs first: JSON/JSONC, policy parity, catalog, docs, file sizes,
# stale references, the ollama brain tests
python3 desktop/tools/ci/repo_checks.py

# dashboard client
cd desktop/frontend && npm run build
```

`desktop/tools/fake-executor/fake_executor.py` speaks the agent protocol and
never generates input, so the server can be exercised (and the dashboard
explored) on a machine with no display and without touching anything.

## Troubleshooting

| Symptom | Cause / fix |
| --- | --- |
| `/api/status` shows `executor.connected: false` | No agent connected. Start `python3 -m controller.agent --bridge 127.0.0.1:9876`, or use `NEURO_IPC_FILE` file IPC. |
| `executor.replaced_connections` climbing | Two agents are running against one hub; the newer one wins. Kill the extra process. |
| Actions fail with "no display session" | The agent is on a machine without a GUI session. Use `shell_command`, or run the agent on the desktop PC. |
| `relay rejected the registration` | The token must match on both sides: `NEURO_RELAY_AUTH_TOKEN` for `neuro-integration relay`, or `intermediary.auth_token` for the upstream relay. The relay's log says `invalid auth token`. |
| Shell action says the command is not allow-listed | Add the program to `NEURO_SHELL_ALLOWLIST`; the output names the pattern that blocked it. |
| Dashboard shows 403 on a write | Set `X-ND-Token`/`Bearer`/`?token=` from `NEURO_ADMIN_TOKEN` when the dashboard is not on loopback. |

## Removed

Several components were deleted when the server and the agent were consolidated into Go and Python: the Rust executor, the C++ process handler, the Python relay shim, the `native/` placeholders, and the root integration scripts that could not run. Nothing in the shipped path uses them, and `tools/ci/repo_checks.py` fails if a reference to one of them returns. The list and the reasons are in `CHANGELOG.md`.

There is no native tray. The dashboard is served by the server and works headless, and a tray could not be verified here. The decision is recorded in `docs/PRODUCTION_TODO.md`.
