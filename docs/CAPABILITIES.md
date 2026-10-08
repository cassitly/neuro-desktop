# Neuro Desktop — Current Capabilities

Honest snapshot of what works today (post bridge/executor split). This is not a
roadmap; see [VISION.md](../VISION.md) and [PRODUCTION_TODO.md](PRODUCTION_TODO.md)
for direction and backlog.

Last updated: 2026-10-08

## Architecture (what you actually run)

| Piece | Binary / path | Role |
|-------|---------------|------|
| **Bridge (server)** | `neuro-integration` | Neuro WebSocket client, permissions, TCP executor hub `:9876`, admin HTTP `:8300` |
| **Executor (client)** | `neuro-desktop` + Python | Mouse/keyboard/scripts on the controlled machine |
| **Local Neuro mock** | `desktop/tools/ollama-neuro` | Randy-like tester using Ollama + `heredos/rwkv7:2.9b` |
| **Operator dashboard** | `desktop/frontend`, served by the bridge at `/ui/` | Live permissions, extensions, games and status |
| **Fake executor** | `desktop/tools/fake-executor` | Protocol simulator for the dashboard (never injects input) |
| **Process supervisor** | `process-handler` | Optional; incomplete messaging, not required for day-to-day use |

**Split machines:** bridge on the Neuro/operator PC; executor on the desktop Neuro should control (`--executor --server host:9876`).

**Co-located:** `./neuro-desktop` can still spawn the Go bridge beside itself (file IPC fallback if no TCP client).

## Platforms

| Capability | Windows | Linux | macOS |
|------------|---------|-------|-------|
| Low-level mouse / keyboard (`pyautogui`) | Yes | Yes* | Yes* |
| High-level desktop intents (start menu, snap, lock, …) | Yes (Win shortcuts) | Best-effort (DE-dependent) | Best-effort (⌘ mappings) |
| Window enumeration | Yes | Weak / optional | Weak / optional |
| Screenshots (`mss`) | Yes | Yes* | Yes* |
| Headless / CLI-only OS (no display) | Supported (`NEURO_HEADLESS` or no session) | Supported (no `DISPLAY`) | Supported (forced) |
| Command lines (`shell_command`) | Yes (allowlist + firewall) | Yes | Yes |

\* Needs a real graphical session. Do **not** run with `sudo` (breaks Wayland/X auth and file ownership). On Omarchy/Hyprland, run from your logged-in desktop session.

## Neuro API integration

Working:

- Connect / reconnect to Neuro (or Ollama mock / Randy) via WebSocket
- `startup`, `actions/register`, `action` → `action/result`, `context`
- Validate quickly, send `action/result`, then run desktop work (API ~20s result window)
- Permission policy: scopes + allow/deny lists (`permissions.json`)
- Periodic desktop context snapshots (Markdown)

Not production-hardened:

- Live Neuro vs Evil character UX beyond startup ack fields
- Voice chat side-channel
- Signed / distributed permission policies

## Actions Neuro can use

### Low-level (default registered on startup)

- `move_mouse_to`, `mouse_click`, `key_press`, `type_text`
- `run_script` (action-script language)
- `execute_queue`, `clear_action_queue`
- `enable_low_level_controls` / `disable_low_level_controls` (mode switch)

### High-level desktop intents (when HL mode / script intents)

OS-mapped shortcuts, including:

- Start / launcher, show desktop, minimize, close foreground app
- Task manager / Force Quit, file manager, run/search dialogs
- Snap left/right, settings, notifications, clipboard history (where supported)
- Lock workstation, app switcher, screen snip

Unsupported intents on an OS fail with a clear error (e.g. macOS clipboard history).

### Game interface (playing a game with no integration of its own)

- `game_list_profiles`, `game_detect`, `game_start_session`, `game_end_session`,
  `game_status`, `game_observe` — profiles in `desktop/catalog/games/*.json`
- `game_move`, `game_look`, `game_action`, `game_press`, `game_release_all` — bounded
  holds (≤30s), relative mouse-look with sensitivity/inversion/clamping, masked keys
  (`control.allow_raw_keys` for raw key presses)
- Per-session rate budget (`control.max_actions_per_minute`) plus the dashboard's
  per-scope `max_actions_per_minute`
- Every failed primitive releases held input, so a half-applied move cannot leave a
  key stuck down
- `game_launch` runs the profile's launch command and needs the `system` scope

### Shell (headless / terminal work)

- `shell_command {command, cwd?, timeout?}` runs one command line and returns the
  exit code plus truncated stdout/stderr — the only capability that needs nothing
  but an OS, which is what makes a command-line-only machine useful
- Program allowlist (`NEURO_SHELL_ALLOWLIST`, empty by default = run nothing) plus
  built-in deny patterns (`rm -rf /`, `mkfs`, `dd of=/dev/…`, `shutdown`, `sudo`,
  `curl … | sh`, `diskpart`, fork bombs, …); `NEURO_SHELL_DENYLIST` adds patterns
- `NEURO_SHELL_TIMEOUT` (default 20s, cap 120s), `NEURO_SHELL_CWD`,
  `NEURO_SHELL_MAX_OUTPUT` (default 4000 chars, cap 200 000)
- Checked twice: in the Go bridge (before the executor is bothered) and again in
  Python, because scripts and relay watchers are separate entry points
- Denied in every shipped example policy and *not* enabled by `default_allow`;
  the `shell` scope must be turned on by name

### Built for small models

- `desktop_guide {topic}` (always registered, `all|desktop|games|shell|safety`)
  returns the instruction sheet: order of operations, parameter names, examples
- A compact version is pushed as silent context after every (re)connect, so a weak
  model does not have to remember the surface from the conversation
- Action descriptions name the parameter and give an example; the script language
  is summarised in the same context push
- Refusals are written to be actionable: which scope is off, where to enable it,
  which program to allowlist, and which peer integration owns the action
- Missing/empty required parameters are answered with the exact expected shape
  (`{key, value} (required: key)`) instead of "invalid parameters"

### Catalog / extensions / vision

- Catalog list/search/get — metadata from local catalog config
- Extensions: install (`metadata_only` or `git_clone`), enable/disable, uninstall,
  persisted in the extension state file, driven from the dashboard Extensions tab
- Desktop context + optional HTTP vision summarize (`NEURO_VISION_SERVER_URL`)
- `game_observe` (and the dashboard's “Show me what Neuro sees”) captures a
  screenshot and asks the vision server for a summary

### Working alongside other integrations

- Relay participation (`NEURO_RELAY_*`): register as `<name>` on the relay's
  intermediary socket, publish action schemas, report peers, and receive watcher
  commands that still pass through the permission policy, pause flag and kill switch
- Shared-Neuro-connection mode: point `NEURO_SDK_WS_URL` at the relay's Nakurity
  Backend (usually `ws://127.0.0.1:8001`) and the bridge is an ordinary Neuro SDK
  client to the relay, which is the supported way to run beside a game integration
- Peer awareness: watcher events (`integration_registered_actions`,
  `integration_message`) are turned into concrete hints, so a refused
  `game_move` names the owning integration's real action names
- Registrations are verified against the upstream sources; a bad
  `NEURO_RELAY_TOKEN` is reported as "invalid auth token" instead of an
  unexplained disconnect, and the bridge no longer lists itself as a peer
- The relay link survives an idle relay: liveness is checked with websocket
  pings (a gorilla read deadline used to reconnect the link every 90 seconds),
  and writes racing the relay's rejection frame are reported as such
- Verified against a live upstream relay (`python3
  desktop/tools/relay-compat/run_relay.py <neuro-relay>/src`): the intermediary
  logs `Registered actions from Neuro Desktop: [... 51 actions]` and the link
  stays connected
- Upstream caveat, found while verifying: the relay cannot start with any
  published `neuro-api` release because `NakurityBackend` does not implement all
  abstract methods (0.x/1.x have no `neuro_api.server` at all; 2.x+ leave five to
  seven methods abstract). `desktop/tools/relay-compat/run_relay.py` fills them
  in at runtime so the relay runs unchanged
- `NEURO_RESERVED_ACTIONS` keeps another integration's action names out of the
  registry so the two cannot shadow each other
- `control.mode: auto` delegates a game to its dedicated integration when that
  integration is connected, and drives it from Neuro Desktop when it is not

## Operator / Vedal controls

Working:

- `permissions.json` enforced in the Go bridge before actions run
- Scopes: `input`, `game`, `shell`, `filesystem`, `process`, `network`, `system`, `vision`
- Scope values accept both `true`/`false` and `{"allowed": …, "limits": {...}}`;
  per-scope `max_actions_per_minute` is enforced (sliding window, denial explains
  how to raise it)
- Safe defaults: system + filesystem restricted; the example policy denies
  `game_launch` and the `shell` scope
- Operator brake: `NEURO_PAUSED`, `POST /api/control/pause|resume`, and a
  kill-switch file (`NEURO_KILL_SWITCH_FILE`) that blocks actions while it exists;
  input release, status and session end always stay allowed
- Hard deny list (`NEURO_DENY_ACTIONS`) that survives dashboard edits, because it
  is merged into the policy at load time
- Audit log (`NEURO_AUDIT_LOG`, JSON lines) plus `GET /api/audit?limit=N` for the
  dashboard tail: every accepted/refused action with its reason
- `shell` and `system` scopes ignore `default_allow` and bare allow-list entries:
  they have to be enabled by name
- Admin HTTP on `:8300` — `GET/PUT /api/permissions`, `/api/permissions/schema`,
  `/api/actions`, `/api/extensions[/{id}/{action}]`, `/api/games`,
  `/api/games/session`, `/api/games/release`, `/api/games/observe`, `/api/relay`,
  `/api/control` (+`/pause`, `/resume`), `/api/audit`, `/api/status`, `/api/config`,
  and the dashboard itself at `/ui/`
- Dashboard writes are guarded by `NEURO_ADMIN_TOKEN`; a local browser gets the
  token injected, a remote one must paste it (Status tab)
- Live sync: `Save` in the Permissions tab applies the policy to the running bridge

Not done:

- Rich path/process/host allowlists for file and network actions (the shell has a
  real allowlist/denylist; file/network scopes are still whole-capability toggles)
- Signed / distributed permission policies

## Local testing without live Neuro

`desktop/tools/ollama-neuro`:

- Mimics Neuro API on `ws://127.0.0.1:8000`
- Modes: `manual` (Randy-like), `random`, `ollama` (RWKV7)
- Control UI on `:1337`
- Tuned for slow CPUs (long timeouts, `--warm`, `keep_alive`)

## Explicitly not ready

- Full plugin marketplace / signed catalogs (extensions are a catalog + state file,
  not a sandboxed plugin runtime)
- Production native tray shell
- Process-handler as the primary supervisor (stubs / TODOs remain)
- Guaranteed Linux DE shortcut parity
- Running input under `sudo` (still forbidden; headless mode exists precisely so
  this is not needed)

## Quick verify

```bash
cd desktop

# Fix leftover root-owned build dirs if a past sudo broke things:
sudo chown -R "$USER:$USER" frontend/dist dist apps/neuro-desktop/target backend/python/.venv

# Bundle as your user (never sudo):
./scripts/bundle/dev.sh

# Or split:
# terminal A — bridge
./apps/neuro-desktop/target/release/neuro-integration --ws-url ws://127.0.0.1:8000
# terminal B — executor (graphical session)
./apps/neuro-desktop/target/release/neuro-desktop --executor --server 127.0.0.1:9876
```

Dashboard without a display, a GPU or pyautogui:

```bash
# terminal A — fake executor (reports the active window from NEURO_FAKE_WINDOW)
python3 desktop/tools/fake-executor/fake_executor.py --addr 127.0.0.1:9876

# terminal B — bridge + dashboard
cd desktop/apps/neuro-integration
NEURO_UI_DIR=../frontend/dist NEURO_ADMIN_TOKEN=demo go run .
# then http://127.0.0.1:8300/ui/
```

Headless / CLI-only verification (no display, no GUI libraries):

```bash
# Python controller: imports and runs with the standard library alone
NEURO_HEADLESS=1 NEURO_SHELL_ALLOWLIST=ls PYTHONPATH=desktop/backend/python \
  python3 -m unittest discover desktop/backend/python/tests -t desktop/backend/python -v

# Repository-level checks (JSON, catalog, example policies, docs)
python3 desktop/tools/ci/repo_checks.py

# Relay protocol contract (fake intermediary speaking the upstream shapes)
cd desktop/apps/neuro-integration && go test -run TestRelay -v ./...

# Relay end-to-end (real upstream relay + this bridge)
python3 desktop/tools/relay-compat/run_relay.py /path/to/neuro-relay/src
# then, with NEURO_RELAY_ENABLED=true NEURO_RELAY_URL=ws://127.0.0.1:8765
# and NEURO_RELAY_TOKEN matching intermediary.auth_token:
curl -s http://127.0.0.1:8300/api/relay     # connected/registered true
```

For a fake Neuro backend: see [desktop/tools/ollama-neuro/README.md](../desktop/tools/ollama-neuro/README.md).
