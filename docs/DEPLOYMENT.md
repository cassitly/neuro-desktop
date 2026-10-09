# Deployment

This describes how to build, bundle, and run Neuro Desktop. It replaces the older
guide, which described the Rust executor and the C++ supervisor; both were removed
(see `CHANGELOG.md`).

There are two processes: the **server** (`neuro-integration`, Go), which is the PC
Neuro uses and which talks to Neuro's API, and the **agent** (Python), which runs
the commands on the desktop. They can run on one machine or on two. The dashboard
is served by the server.

## Requirements

| What | Version | Needed for |
| --- | --- | --- |
| Go | 1.22 or newer (`go.mod` says 1.22) | the server |
| Python | 3.12 is what CI runs; other 3.x versions are untested | the agent |
| Node.js | 22 is what CI runs | building the dashboard only |
| Git | any | cloning |

Optional: `tesseract` (OCR for the vision server), Ollama (the local test brain), and
`npx` on the PATH for MCP servers from the catalog.

**Never run any of it with `sudo`.** The agent needs your logged-in desktop session
(Wayland and X authorisation belong to that session), and `sudo` makes the files it
writes owned by root.

## Build

From the repository root:

```bash
cd desktop
# the server
(cd apps/neuro-integration && go build -o neuro-integration .)
# the dashboard
(cd frontend && npm ci && npm run build)
# the agent: nothing to build; it is Python
```

On Windows, use `go build -o neuro-integration.exe .` in `apps\neuro-integration`.
`scripts/build-go.ps1` does the Go step for all three platforms.

## Bundle

The bundle scripts put the server, the dashboard, the agent, the catalog, and the
policy into one folder:

| Platform | Development bundle | Release bundle |
| --- | --- | --- |
| Linux / macOS | `scripts/bundle/dev.sh` | `scripts/bundle/prod.sh` |
| Windows | `scripts/bundle/dev.ps1` | `scripts/bundle/prod.ps1` |

The output is `desktop/dist/neuro-desktop/`:

```
neuro-desktop/
├── neuro-integration(.exe)     the server
├── start.sh / start.bat        starts the server and the local agent
├── agent/                      the Python agent (controller/, requirements)
├── frontend/                   the dashboard
├── catalog/                    the signed extension index and publisher keys
├── config/                     example configuration
├── permissions.json            the policy (copied from the example; edit it)
├── integration-docs/           documentation for Neuro
└── README.txt
```

Set `NEURO_BUNDLE_SKIP_VENV=1` to skip creating a Python virtual environment.

**Edit `permissions.json` before you start.** It is copied from the example, which is
safe by default: the shell, system, filesystem, and extensions scopes are off. Neuro
can ask for filesystem and extensions with `request_permission`; you decide on the
Permissions page.

## Run

On the PC Neuro uses, from a logged-in session:

```bash
cd dist/neuro-desktop
./start.sh          # Windows: start.bat
```

This starts the server and the local agent. `NEURO_NO_AGENT=1` starts only the
server, for a split-machine setup where the agent runs elsewhere.

Open the dashboard at `http://127.0.0.1:8300/ui/`.

### Split machines

Run the server on the machine that runs Neuro. Run the agent on the desktop it should
control. The agent always dials the server, so the desktop needs no inbound port:

```bash
# on the desktop PC
python3 -m controller.agent --bridge <server-ip>:9876 --token "$NEURO_EXECUTOR_TOKEN"
```

On the server, the hub must listen on an address the desktop can reach, for example
`NEURO_EXECUTOR_LISTEN=0.0.0.0:9876`, and `NEURO_EXECUTOR_TOKEN` must be set. The hub
refuses to listen beyond loopback without a token, and it says why. Allow port 9876 from
the desktop's address only.

### Headless (no display)

On a machine with no graphical session, set `NEURO_HEADLESS=1`. The agent then skips the
GUI libraries, so it starts without a display. Shell commands still run (within the
allowlist), and a screenshot reports that there is no display session. The dashboard and
the policy work the same. See `docs/CAPABILITIES.md`.

To open the dashboard from another machine, forward the port over SSH rather than
exposing it:

```bash
ssh -L 8300:127.0.0.1:8300 user@server
```

## Ports

| Port | Process | Default address | Expose it? |
| --- | --- | --- | --- |
| 8300 | server: dashboard and admin API | `127.0.0.1:8300` (`NEURO_ADMIN_LISTEN`) | no; use SSH forwarding |
| 9876 | server: executor hub, agents connect here | `127.0.0.1:9876` (`NEURO_EXECUTOR_LISTEN`) | only on a trusted LAN, with a token |
| 8765 | relay host: integrations and watchers | `127.0.0.1:8765` | only on loopback or a firewalled LAN |
| 8766 | relay host: `/health` | `127.0.0.1:8766` | no |
| 8610 | vision server | `127.0.0.1:8610` | no |
| 8000 | Neuro API (outbound) | `NEURO_SDK_WS_URL` | outbound only |

Allow outbound traffic to the Neuro API and, if you use them, to the npm registry (for
MCP servers) and to GitHub (for `git_clone` extension installs). Nothing else needs to
reach the machine from outside.

## The relay and the vision server (optional)

- **Relay:** run `neuro-integration relay` next to the server, then set the server's
  `NEURO_RELAY_ENABLED`, `NEURO_RELAY_URL`, and `NEURO_RELAY_TOKEN`. Details are in
  `docs/RELAY.md`.
- **Vision:** run `python3 -m nd_vision` from `desktop/apps/nd-vision-server` and set
  `NEURO_VISION_URL` and `NEURO_VISION_TOKEN` on the server. Details are in that
  folder's README.

The Extensions page shows whether each one is reachable.

## Configuration

Every environment variable the server reads is listed in `desktop/README.md`
(Configuration). The ones an operator usually sets:

| Variable | Set it to |
| --- | --- |
| `NEURO_SDK_WS_URL` | the Neuro API websocket |
| `NEURO_ADMIN_TOKEN` | a token for dashboard writes when the dashboard is not on loopback |
| `NEURO_EXECUTOR_TOKEN` | the secret every agent presents |
| `NEURO_PERMISSIONS_FILE` | the policy file (defaults to `permissions.json` in the bundle) |
| `NEURO_SHELL_ALLOWLIST` | the programs the shell action may run |
| `NEURO_AUDIT_LOG` | where the audit log is written (JSON lines) |
| `NEURO_KILL_SWITCH_FILE` | a file whose existence stops every action |

## Upgrading

1. Stop the running processes (Ctrl-C in the terminal that runs `start.sh`).
2. Build and bundle the new version.
3. Keep your `permissions.json`, your audit log, and the extension state file. Copy them
   into the new bundle, then compare the new example policy with yours.
4. Start it again and check the Status tab and the Extensions page.

## Health checks

```bash
curl -s http://127.0.0.1:8300/api/runtime       # bridge, relay, vision, and MCP state
curl -s http://127.0.0.1:8766/health            # the relay host, if you run it
curl -s http://127.0.0.1:8610/health            # the vision server, if you run it
```

The dashboard's Extensions page shows the same runtime state, live.

## Backups

Back up `permissions.json`, the audit log, and the extension state file. Nothing else
holds state. Permission approvals and pending requests are in memory, so a restart
clears them.

## Service installs

The bundle does not install a service, and this repository does not ship one. Run the
bundle from a logged-in session. A service that runs without the desktop session cannot
control the desktop, so it is not a supported setup for the agent. The server alone can
run as a service on a headless machine, but that has not been tested here.

## What is verified here, and what is not

Verified in CI: the server builds for Windows, macOS, and Linux; the server and agent
test suites pass; the dashboard builds; the release bundle builds on Linux, and the
dashboard is served from the bundle folder; and the agent and server run end to end on
Linux with no display.

Not verified here: the bundle on a real Windows or macOS desktop, input and screen
capture on a real desktop, and an installed (not built) release. Treat those as open
until someone has run them.
