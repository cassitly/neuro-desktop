# Architecture

Neuro Desktop lets Neuro-sama use a desktop. It is split into a **server** that
receives Neuro's commands and applies Vedal's policy, an **agent** that performs
them on the machine, and a **dashboard** that Vedal uses to control the server.

```
                 Neuro API (websocket, Neuro-sama's backend)
                                  │
                                  ▼
 ┌─────────────────────── SERVER (Go, neuro-integration) ───────────────────────┐
 │ Neuro client (vendored SDK port)   policy · scopes · rate limits · audit     │
 │ stop switch: pause / kill file     executor hub ◀── agents (TCP, token)     │
 │ dashboard API + UI at /ui/         file IPC fallback (co-located agent)     │
 │ relay client ──▶ Neuro Relay (other integrations share the connection)      │
 │ relay host    (`neuro-integration relay`, optional, separate listener)       │
 │ MCP bridge ──▶ MCP servers Vedal enabled (child processes)                   │
 │ catalog (signed) · game profiles · extension state · vision client           │
 └──────────────────────────────────────────────────────────────────────────────┘
          │ commands                                  ▲ results, telemetry
          ▼                                           │
 ┌──────────── AGENT (Python, desktop/backend/python/controller) ────────────┐
 │ the only part that touches the machine: input, screen, shell, scripts      │
 │ runs on the desktop PC, or on another PC (split-machine); CLI-only is fine  │
 └────────────────────────────────────────────────────────────────────────────┘

 DASHBOARD (TypeScript + Vite, served by the server at /ui/) — Vedal's client:
 Extensions · Games · Permissions · Status (Settings)
```

## Components

| Component | Where | Language | Role |
| --- | --- | --- | --- |
| Server | `desktop/apps/neuro-integration` | Go | Everything Neuro can reach, and every gate between Neuro and the machine |
| Agent | `desktop/backend/python/controller` | Python (stdlib-first) | Executes commands on the machine; reconnects on its own |
| Dashboard | `desktop/frontend` | TypeScript | Vedal's controls, served by the server |
| Relay host | the server binary, `relay` subcommand | Go | Neuro Relay socket for integrations and watchers |
| Vision | `desktop/apps/nd-vision-server` | Python (stdlib HTTP) | Optional screen description for `game_observe` |
| MCP servers | catalog-installed | any | Optional tools, started only after Vedal enables them |
| Test tools | `desktop/tools/` | Python | `fake-executor` (protocol simulator), `ollama-neuro` (small-model tester) |

The Go SDK port is vendored under `desktop/apps/neuro-integration/third_party/`
and pinned in `go.mod`.

## How a command flows

1. Neuro sends an action over the Neuro API websocket.
2. The server checks, in order: the **stop switch** (paused, or the kill-switch
   file exists), the **permission policy** (is the action's scope allowed, or
   requestable?), and the **per-scope rate limit**. A refusal is returned to
   Neuro with a message that says what to do next. The refusal is audited.
3. The command goes to an **agent** connected to the executor hub. If no agent is
   connected and one runs on the same machine, the server falls back to file IPC
   (`NEURO_IPC_FILE`).
4. The agent runs it and reports back. The server answers Neuro's `action/result`
   early, then sends the output as context when it arrives, for long commands.

Watcher commands from the relay take the same path, with the same gates
(`denyRelayCommand` in `relay.go`).

## Where state lives

| State | Where | Notes |
| --- | --- | --- |
| Policy | `NEURO_PERMISSIONS_FILE` (`permissions.json`) | the example is `desktop/config/permissions.example.json` |
| Permission requests and approvals | in memory, shown on the Permissions page | Neuro asks; Vedal approves or denies. A restart clears both |
| Audit | `NEURO_AUDIT_LOG` (JSON lines) | refusals, accepted actions, relay commands, policy changes |
| Extension state | `NEURO_EXTENSIONS_STATE_FILE` (runtime, not tracked by Git) | what is installed and enabled |
| Catalog | `desktop/catalog/index.json`, `publishers.json` | signed; the bridge verifies every item |

## Decisions

* **Two languages on the machine: Go and Python**, plus the TypeScript dashboard.
  Rust and C++ were removed from the shipped path, and `tools/ci/repo_checks.py`
  fails if they come back. The history is in `CHANGELOG.md`.
* **One setup step, one shared secret file.** `neuro-integration setup` creates the
  relay token, which the relay host and the server both read. The dashboard never pushes
  tokens to another process. The dashboard token is printed once and kept only as a hash.
  This answers upstream issue Nakashireyumi/neuro-desktop#16, which asked how the UI and a
  relay that the bundle did not ship keep their tokens in step.
* **Development builds are separate.** The unsigned-extension switch
  (`NEURO_EXTENSIONS_ALLOW_UNSIGNED`) compiles in only with `-tags neurodev`, which the
  development bundle uses. A release build has no way to turn it on.
* **The server does not start other programs.** It does not start a relay, and it
  does not supervise the agent. Operators run each process (or the bundle's
  `start.sh` / `start.bat`) and point the server at the others.
* **No native tray.** The dashboard is served by the server and works without a
  display, which the headless target needs. A tray could not be built or verified
  in this environment, so it was not added. That is an explicit decision, not an
  oversight. See `docs/PRODUCTION_TODO.md`.
* **Headless first.** On a machine with no display (`NEURO_HEADLESS=1`), the agent
  runs its command-line parts and the server still serves the dashboard and the
  policy. CI runs the agent with no display as a regression test.
* **The default is closed.** Without a policy file the server uses a built-in
  default, and an action is denied unless its scope is on. Input, process,
  network, vision, and game are on; filesystem, system, shell, and extensions are
  off. `desktop_guide`, `reset_controls`, and `request_permission` are always
  allowed. Shell and system access need explicit consent.

## Security boundaries

* Agent connections require the executor token (`NEURO_EXECUTOR_TOKEN`). The link is
  plain TCP until the pinned-TLS work in `docs/PRODUCTION_TODO.md` lands.
* Every dashboard API call needs the dashboard token that `setup` makes. There is no
  exception for loopback. The server keeps only the token's SHA-256 hash.
* The relay refuses browsers, binary frames, and weak tokens. Its token comes from the
  relay token file that `setup` writes, which the server reads too. See `docs/RELAY.md`.
* Extensions must have a valid signature from a publisher in `publishers.json`.
  See `docs/SAFETY.md`.

## Testing

| Layer | Command | Where it runs |
| --- | --- | --- |
| Server | `go test -race ./...` | CI on Linux, Windows and macOS (race detector on Linux) |
| Server, development build | `go test -race -tags neurodev ./...` | CI on Linux |
| Relay, MCP, vision, catalog, eval | see `.github/workflows/ci.yml` | their own CI jobs |
| Agent | `python3 -m unittest discover backend/python/tests` | CI, with no display |
| Repository | `python3 desktop/tools/ci/repo_checks.py` | CI, first job |
| Dashboard | `npm run build` in `desktop/frontend` | CI |
| End to end | the `agent-e2e` job starts the server and the agent | CI |

What is **not** verified here: the agent on a real Windows or macOS desktop
(input, screen capture), and the dashboard visually. CI builds the server for
those systems but cannot exercise the desktop.
