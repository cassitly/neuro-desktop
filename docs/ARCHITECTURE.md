# Architecture: one server, one agent, one client

## The decision

Neuro Desktop is built from three parts and two languages:

| Part | Language | Runs on | Job |
| --- | --- | --- | --- |
| **Server** (`desktop/apps/neuro-integration`) | Go | the PC that runs Neuro (or any PC) | Neuro API client, action registration, permission policy, audit, game catalog, executor hub, dashboard API + UI hosting, relay participation |
| **Agent** (`desktop/backend/python/controller/agent.py`) | Python | each PC Neuro controls | mouse, keyboard, scripts, shell, screen capture, window/process telemetry |
| **Client** (`desktop/frontend`) | TypeScript | the admin's browser | dashboard: watch, permissions, extensions, games, relay, audit |

The server is the part that *decides*; the agent is the only part that *touches
a machine*. That split is what makes a two-PC setup (`--executor-listen` on the
server, `--bridge <ip>` on the agent) and a one-PC setup (file IPC or loopback
hub) the same code path.

## Why this replaced the previous four-language mix

The first version of this project was Go (Neuro client) + Rust (executor and
process manager) + Python (drivers, called from Rust through PyO3) + C++ (a
process supervisor) + TypeScript (dashboard). Concretely, that meant:

* **A second toolchain in the shipping path.** A release needed Rust, Python and
  Go, and the Rust executor could not be built without the Python headers of a
  matching interpreter version.
* **Two implementations of one protocol.** The IPC command set existed in Rust
  types, Go structs, Python methods and C++ parsers; a renamed field in one place
  silently produced "the action did nothing" (this actually happened with
  `CmdMoveMouseRelative`).
* **PyO3 as a failure point.** Crashes inside the embedded interpreter took the
  whole executor with it, and the C++ supervisor existed mostly to restart it.
  The Python agent is a *process*; if it dies, the server says so and it
  reconnects — no supervisor process needed.
* **Unbuildable-without-network extras.** The C++ component needed CMake and
  GoogleTest fetched from the network, so CI could not verify it.

Kept from the old design: the Python driver code (it is the actual desktop
integration and it works), the action surface and its schemas, the game
interface, the dashboard, and the Relay compatibility layer.

## Migration status

| Component | Status |
| --- | --- |
| Python drivers (`controller/{actions,desktop,shell,platform_intents,controls}`) | **Shipped.** Used by the agent. |
| Python agent (executor protocol, TCP + listening + file IPC, queue semantics) | **Shipped.** Replaces the Rust executor. |
| Go server (Neuro client, policy, audit, catalog, hub, relay, dashboard API) | **Shipped.** The application. |
| Rust executor (`apps/neuro-desktop`) | **Not shipped.** Reference only; `cargo` is not in the release path. Its protocol is documented in `docs/EXECUTOR_PROTOCOL.md` and implemented by the agent. |
| C++ supervisor (`apps/process-handler`) | **Not shipped.** Real code with a real suite, but nothing calls it. Delete it or wire it as the Windows supervisor — see `docs/PRODUCTION_TODO.md`. |
| `desktop/native/*` | **Deleted.** Hello-world placeholders (`add(a, b)`, an empty CMake project) that were referenced by docs which described a layout that never existed. |
| Root `tests/integration/*.js` | **Deleted.** Jasmine-style files with no runner: running one straight with `node` fails with `describe is not defined`. The behaviour they described is covered by the Go end-to-end suite. |

## What the server must never do

The server must not implement machine control itself. If it did, the split
machines case would need a second implementation, and the "which language owns
input" question would come back. Every machine action goes through
`sendToExecutor` (`desktop/apps/neuro-integration/utils.go`), which refuses
anything that is not in `executorCommands`
(`desktop/apps/neuro-integration/executor_commands.go`). That single list is
checked against the agent's dispatch by
`desktop/backend/python/tests/test_agent.py`, so a new command cannot land on one
side only.

## Roadmap

1. **Done:** agent parity with the Rust executor, single source of truth for the
   command list, no Rust/C++ in the build or bundle.
2. **Next:** delete `apps/process-handler` and `apps/neuro-desktop` (or move them
   to a clearly-labelled `legacy/`), after the audit log and dashboard no longer
   mention them.
3. **Next:** the extensions page reads live runtime state (bridge, relay, vision,
   MCP) instead of install state only, and Neuro can ask for permission through
   the dashboard.
4. **Later:** a first-party relay (`docs/RELAY.md`) so the "run alongside other
   integrations" path does not depend on a third-party Python package.
