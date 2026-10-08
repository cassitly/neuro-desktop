# Changelog

All notable changes to this project should be documented in this file.

This project aims to follow semantic versioning once stable releases begin.

## [Unreleased]

### Added

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
