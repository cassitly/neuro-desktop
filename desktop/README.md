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
│   ├── neuro-integration/    SERVER (Go): Neuro client, dashboard API, hub, policy
│   ├── process-handler/      optional C++ supervisor — NOT shipped, see docs/ARCHITECTURE.md
│   └── neuro-desktop/        legacy Rust executor — NOT shipped, kept as reference
├── backend/python/
│   ├── controller/           AGENT + drivers (agent.py, actions.py, controls/, shell.py)
│   └── tests/                unittest suite for the agent and the safety rules
├── frontend/                 DASHBOARD CLIENT (TypeScript + Vite, no framework)
├── catalog/                  game profiles, catalog index, live extension state
├── config/                   example policy + integration config
├── scripts/                  build-all.*, build-go.ps1, bundle/{dev,prod}.*, templates/
├── tools/
│   ├── ci/repo_checks.py     dependency-free repository checks (what CI runs first)
│   ├── fake-executor/        protocol simulator for the server (no input is ever generated)
│   ├── ollama-neuro/         local small-model brain for testing the action surface
│   └── relay-compat/         runs Neuro Relay with the neuro-api version we can support
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
# the C++ suite, the ollama brain tests
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
| `relay rejected the registration` | `NEURO_RELAY_TOKEN` must equal `intermediary.auth_token` in the relay's `authentication.yaml`. |
| Shell action says the command is not allow-listed | Add the program to `NEURO_SHELL_ALLOWLIST`; the output names the pattern that blocked it. |
| Dashboard shows 403 on a write | Set `X-ND-Token`/`Bearer`/`?token=` from `NEURO_ADMIN_TOKEN` when the dashboard is not on loopback. |

## What is *not* shipped

* `apps/neuro-desktop` — the Rust executor, which the Python agent replaces. Keep
  it as a reference for the action surface, or delete it once nothing references it.
* `apps/process-handler` — a C++ supervisor with a real test suite. Nothing in the
  shipped path uses it (the agent reconnects on its own); it is a candidate for
  deletion or for wiring in as the Windows supervisor. Tracked in
  `docs/PRODUCTION_TODO.md`.
