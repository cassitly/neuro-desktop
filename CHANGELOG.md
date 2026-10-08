# Changelog

All notable changes to this project should be documented in this file.

This project aims to follow semantic versioning once stable releases begin.

## [Unreleased]

### Added

- **Python agent** (`desktop/backend/python/controller/agent.py`): the process
  that executes commands on the machine Neuro controls, replacing the Rust
  executor. Speaks the executor protocol over TCP (connecting out or listening)
  and over the bridge's file-IPC fallback, honours `execute_now`/`clear_after`,
  answers pings, and refuses input on a headless machine with a message that
  points at `shell_command`.
- `docs/ARCHITECTURE.md` — the consolidation decision, the migration table, and
  what is (and is not) on the shipping path.
- `docs/EXECUTOR_PROTOCOL.md` — the server↔agent wire format, the command table,
  queue semantics and file-IPC rules.
- `docs/RELAY.md` — how Neuro Desktop coexists with other integrations through
  Neuro Relay, including the two modes and the upstream limitation that actions
  registered on the intermediary never reach Neuro.
- `docs/LLM_GUIDE.md` and `docs/SAFETY.md` — prompting tactics for small models,
  and every guard with where it lives and how to verify it.
- `desktop/scripts/bundle/templates/` — checked-in `start.sh`, `start.bat` and
  `README.txt` for the release bundle, so the launchers can be syntax-checked.
- `desktop/scripts/build-go.ps1` cross-compiles the server for Windows, Linux and
  macOS; CI cross-compiles the same targets on every change.
- `agent-e2e` CI job: starts the server and the agent together and asserts the
  dashboard sees the agent, so a protocol regression between the two languages
  fails the pipeline.
- Executor keepalive on the hub: a 30-second ping with a 90-second drop, which
  removed a reconnect-per-2-minutes loop (`total_connections` used to climb
  forever).
- `desktop/tools/ci/repo_checks.py`: bundle-template validation, a stale-path
  checker (deleted `native/*` paths may not be referenced any more), and
  documentation link checks for the three new guides.

### Changed

- The shipped product is **Go server + Python agent + TypeScript dashboard
  client**; the Rust executor and the C++ supervisor are no longer part of the
  build, the bundle, or the CI gate (they remain as reference code).
- Test coverage for the executor protocol, the `/api/actions` metadata
  (schema + plain-language params), the relay self-peer phantom, and the
  process-handler parser/lifecycle.

### Removed

- `desktop/native/{go,c_cpp,rust-core}` — hello-world placeholders that were
  referenced by documentation describing a layout that never existed.
- The root `tests/integration/*.js` files — Jasmine-style tests with no runner
  (they cannot be executed at all).
- The Rust `relay_manager` invocation and the `build-all.ps1` "relay build" step
  (both launched a `neuro-relay` CLI that does not exist), plus every pointer to
  the placeholder Go module under `desktop/native/` that was never written.

### Fixed

- The relay link reported **its own registration** as a coexisting integration
  (the relay echoes `integration_connected` back), so `peer_count` could never
  reach zero and game delegation could point at ourselves.
- A rejected relay registration now reports the relay's own error text
  ("invalid auth token") with a hint at `NEURO_RELAY_TOKEN`, instead of a bare
  "connection failed" that read like a network problem.
- The bridge no longer writes a stale executor result: replaced clients are
  counted (`replaced_connections`) and closing one no longer looks like a crash.
- `sendToExecutor` refuses commands the bridge handles itself, so a forgotten
  command fails loudly instead of silently doing nothing.
- Baseline Windows CI workflow for Rust, Go, Node, and C++ validation.
- Python parser tests for high-level commands and click syntax.
- Go tests for integration documentation loading.
- `run_script` JSON schema (`action-schema.run_script.json`).
- Production backlog and issue verification tracker (`docs/PRODUCTION_TODO.md`).
- Project vision document (`VISION.md`).
- Relay participation built into the Go bridge (register as a relay integration,
  report peers, reserved action names) — replaces the old Rust "relay process
  manager", which invoked a `neuro-relay` CLI that does not exist.
- Docker-based modular test setup (`docker-compose.tests.yml`).
- Optional relay bundling support in PowerShell bundle/build scripts.
- Game interface: game profiles (`desktop/catalog/games`), sessions with
  `nd`/`external`/`hybrid`/`auto` control modes, and `game_*` actions (move, look,
  press, observe, release all).
- Live operator dashboard: permissions editor, extensions manager, games panel and
  status page, served by the bridge at `/ui/`.
- Per-scope actions-per-minute limits, enforced with a sliding window.
- `desktop/tools/fake-executor` protocol simulator for dashboard development.
- `/api/actions` now returns each action's JSON schema plus a plain-language
  `params` rendering and its required list, so the dashboard (and a weak model)
  can see parameter shapes before a call fails. Guarded by
  `TestActionsExposeLLMFriendlyMetadata`.
- `docs/LLM_GUIDE.md`: prompting tactics and parameter shapes for small/weak
  models, and `docs/SAFETY.md`: every safety system, where it lives and how to
  verify it. `desktop/tools/ci/repo_checks.py` fails if they disappear or stop
  being linked from the README.
- Dependency-free process-handler test suite (`tests/test_standalone.cpp`) that
  CI compiles and runs, plus an ollama-neuro check that no longer needs `aiohttp`.
- Go tests for the executor hub, game profiles, permissions/rate limits, the admin
  API and a full bridge end-to-end run against a fake Neuro backend.
- Python protocol tests for the game input primitives.
- Headless support: `NEURO_HEADLESS` (or simply no `DISPLAY`/`WAYLAND_DISPLAY`)
  now degrades `pyautogui`/`pynput`/`mss`/`pygetwindow` into self-explaining stubs
  instead of failing at import, so the controller runs on a CLI-only OS.
- `shell_command` action plus the `shell` permission scope: one command line per
  call, returning the exit code and truncated output, for headless machines and
  terminal work (`SHELL "…"` script verb too).
- Shell firewall: program allowlist (`NEURO_SHELL_ALLOWLIST`), built-in destructive
  patterns, `NEURO_SHELL_DENYLIST`, timeouts and output caps, enforced in the Go
  bridge and again in the Python executor.
- Safety systems: `NEURO_PAUSED` / `POST /api/control/pause|resume`, a
  kill-switch file (`NEURO_KILL_SWITCH_FILE`), a hard deny list
  (`NEURO_DENY_ACTIONS`) that dashboard edits cannot lift, and a JSON-lines audit
  log (`NEURO_AUDIT_LOG`) with `GET /api/audit?limit=N`.
- `desktop_guide` action and a startup context push, so weak/small models get an
  instruction sheet (order of operations, parameter names, script syntax).
- `relay_protocol_test.go`: a fake intermediary that speaks what upstream
  `intermediary.py` speaks, pinning registration, action announcement, watcher
  commands, peer tracking and bad-token handling.
- `desktop/tools/relay-compat/run_relay.py`: starts the upstream relay despite its
  abstract-method bug (it cannot be instantiated with any published `neuro-api`).
- CI: repository checks (JSON/catalog/example-policy/doc consistency) and a
  stdlib-only Python job that proves the CLI-only install; the main workflow now
  also runs `go build`, `go vet` and `go test -race` on Linux.

### Changed

- Process-handler test build now creates one executable per test source.
- Rust IPC execution path now propagates queue execution/clear failures.
- Action script supports high-level desktop commands.
- Integration docs loading is now non-fatal and supports fallback paths.
- Action script documentation updated to match runtime behavior.
- Go integration migrated to `neuro-integration-sdk` with local compatibility patch.
- High-level Windows action coverage expanded (Explorer, Run, Search, snap-left, snap-right).
- Rust executor validation in CI is advisory (`continue-on-error`) until the crate
  is confirmed building on all three runner platforms.
- Relay coexistence guidance: pointing the bridge's Neuro client at the relay's
  Nakurity Backend is documented as the supported "run alongside an integration"
  path; the intermediary registration is documented as the watcher/visibility path.

### Fixed

- Process handler: `Message::from_json` was a stub that returned an empty message
  for every input, so no IPC command could ever be understood. It now parses the
  wire format strictly (rejecting malformed input instead of routing an empty
  command), `is_safe_json` validates instead of accepting everything, and
  `check_rate_limit` enforces a per-source sliding window.
- Process handler: crash recovery self-deadlocked (`monitor_process` held the
  manager lock and called `restart_process` → `stop_process`, which re-locks the
  same non-recursive mutex). Monitoring now reads state under the lock and
  recovers outside it; `enable_health_monitoring` actually toggles the heartbeat
  check, heartbeat timeouts terminate the offender instead of leaking it, held
  keys are released, `env_vars` are applied on Windows and POSIX, a graceful
  shutdown message is sent before killing a child, and monitor threads are
  joined in `shutdown()` instead of being detached.
- Relay: the bridge counted the relay's echo of its own registration as a
  coexisting integration, so `/api/relay` reported a phantom peer and the
  "another integration owns this game" hints could point at itself.
- Relay: a bad `NEURO_RELAY_TOKEN` is now reported deterministically even when
  the relay's error frame arrives before the follow-up writes fail.
- Relay: a successful registration clears `last_error`, and closes that we cause
  ourselves (shutdown/reconnect) are no longer reported as failures.
- Executor hub: replacements are counted (`replaced_connections`, shown by
  `/api/status`) and the replacement log is rate-limited — two executors running
  at once used to produce thousands of identical lines and look like a network
  problem. The fake executor now backs off when the bridge drops it instead of
  reconnecting in a hot loop.
- `ollama-neuro` imports `aiohttp` lazily, so its parser tests run on a CLI-only
  install with no dependencies.

- Concurrent websocket writes in the vendored Neuro SDK (action results racing the
  read loop); caught by `go test -race`.
- Executor responses are published atomically (write + rename) so the bridge can no
  longer read a half-written response file.
- Dashboard counters (`actions_seen`/`actions_failed`/`actions_denied`) were never
  incremented, so the status API always reported zeros.
- `max_actions_per_minute` in a permission policy used to be ignored.
- A policy file with `"version": 1` (number) or `"input": true` (shorthand) no
  longer prevents the bridge from starting.
- PyO3/Python compatibility blocker for local Python 3.14 environments.
- Mismatch between action docs and `CLICK` parser behavior.
- The relay link reported success the moment it wrote its registration, so a wrong
  `NEURO_RELAY_TOKEN` looked like an unexplained disconnect; the relay's
  `{"error": "invalid auth token"}` is now surfaced with the setting to check.
- Action announcements to the relay now use the name -> schema mapping that
  `intermediary.py` stores (a flat description string was silently useless).
- The bridge listed itself as a relay peer (the relay echoes every connection
  event), which made the external-control message point at the wrong integration.
- A policy with `default_allow: true` (or a bare allow-list entry) could hand out
  the new shell capability; `shell` and `system` now require explicit scope consent.
- `shell_command` parameters that are missing, empty, or sent as the wrong type
  are answered with the expected shape instead of a generic refusal.
- The relay link reconnected every ~10 seconds: the registration was probed with
  a short read deadline, and gorilla keeps the first read error forever, so a
  quiet relay poisoned the socket. Liveness is now checked with websocket
  pings, and a rejection frame (or a write racing it) is reported as
  "relay rejected the registration" with the setting to check.
- `shell_command` output was dropped after the action was acknowledged; the
  transcript is now delivered to Neuro as context.
