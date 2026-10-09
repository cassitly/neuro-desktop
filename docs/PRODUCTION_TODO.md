# Production TODO

Last updated: 2026-10-09. Each status below was checked against the code in this
repository. `[x]` done, `[~]` partly done (what is missing is written next to it),
`[ ]` open, `[-]` declined with the reason.

## Upstream trackers

Check these directly; this file does not keep a copy of their issue lists.

- This project: `cassitly/neuro-desktop`
- Neuro Relay (the upstream this relay interoperates with): `Nakashireyumi/neuro-relay`
  - Open upstream item that is not fixed here:
    [#6 Nakurity ID Server](https://github.com/Nakashireyumi/neuro-relay/issues/6).
    It belongs to the relay backend and identity architecture, not to the Go host.
- Neuro API: `VedalAI/neuro-sdk`
- The Go SDK port this server uses: `cassitly/neuro-integration-sdk`

## Remaining production work

- [x] **Permission model and runtime enforcement** (#12/#20). Scopes carry `allowed`
  and `requestable`. The default is closed. Per-scope rate limits apply to Neuro and
  to watchers. The stop switch and the kill-switch file block Neuro's actions and watcher
  commands; input release and status stay allowed. Neuro can
  ask for a requestable scope, and Vedal decides on the Permissions page. Tests:
  `permissions_test.go`, `permission_requests_test.go`, `ratelimit_test.go`,
  `relay_host_test.go`. Approvals are in memory, so a restart clears them.

- [x] **UI token synchronization and relay-first onboarding** (#16). The dashboard
  pushes no tokens to other processes, and it no longer injects its token into the page.
  `neuro-integration
  setup` creates the relay token, which the relay host and the server share through one
  0600 file. Then it creates the dashboard token, prints it once, and keeps only its hash.
  `setup --check` confirms that the two sides agree. The dashboard asks for its token on a
  sign-in page, and every `/api` route needs it. Tests: `setup_test.go`,
  `relay_token_test.go`, `dashboard_token_test.go`, `admin_server_test.go`. CI runs a
  setup smoke test. Not checked here: the sign-in page in a real browser, and the
  Windows and macOS bundles.

- [ ] **Three separate binaries** (proposed, not started; scope to be confirmed). The
  dashboard, the neuro client on the controlled PC, and the server would be three
  programs. The controlled PC would hold no admin UI and no policy, so Neuro cannot
  change her own permissions from there.

- [ ] **Authenticated reverse connections, and pinned TLS for the executor link.** The
  design allows the server to dial the client, and it requires that connection to be
  authenticated. The plan is a certificate fingerprint pinned on the dialing side, with
  the executor secret sent only over that pinned channel, and the dashboard API served
  the same way. Not built. The executor link is plain TCP today (`docs/SAFETY.md`,
  section 6).

- [ ] **Log failed dashboard sign-ins.** A `401` for a wrong dashboard token is counted in
  memory only. It is not written to the audit log, and it is not logged.

- [ ] **License file.** The README says the project is MIT-licensed and links to a
  `LICENSE` file. The repository has no `LICENSE` file. The maintainer must add it,
  because it needs the copyright holder's name.

- [~] **Integration manager, marketplace, and signed plugin distribution** (#11/#14/#15).
  Done: a signed catalog (`desktop/catalog/index.json`), publisher keys
  (`publishers.json`), verification on every install, `neuro-integration catalog verify`
  in CI, and one MCP item (`memory`, pinned). Missing: a public marketplace, more than one
  publisher, a documented key-rotation procedure, and an install flow for publishers other
  than the maintainer. The shipped signatures use a key that the maintainer must rotate to
  their own before relying on them.

- [~] **Vision pipeline confidence loop and formalized server contract.** Done: the HTTP
  contract is in `desktop/apps/nd-vision-server/README.md`, and the bridge's client is
  tested against it. Missing: a confidence loop (re-capture, or a score the bridge can
  act on).

- [~] **Security hardening: signed bundles, config integrity, least-privilege defaults.**
  Done: least-privilege defaults (shell, system, filesystem, and extensions are off),
  and signed extensions. Missing: signed release bundles and an integrity check on the
  configuration files.

- [-] **Native tray / minimize-to-notification-area shell. Declined.** The dashboard is
  served by the server at `/ui/` and works on a headless machine, which is the target.
  A tray needs a GUI toolkit for each OS, and none of them can be built or run from this
  environment. Adding it would put untested code on the shipping path. It can be
  reconsidered once there are Windows and macOS machines to verify it on.

- [~] **Release quality gates.** Done: the `release-gate` job in CI, which fails unless
  every required job succeeded; repository checks; the race detector; cross-compiles for
  Windows, macOS, and Linux; the weak-model eval; the relay smoke test; and the MCP,
  vision, and catalog jobs. Missing: a coverage target, installer validation (the bundle
  is built, not installed), and a changelog check that is more than a manual review.

## Completed

- The Go server: Neuro client (vendored SDK port), executor hub, dashboard API,
  permissions, audit, stop switch, and game interface.
- The Python agent (`controller/agent.py`): input, high-level desktop intents, scripts,
  the shell with an allowlist and a firewall, and telemetry.
- The action-script language and its high-level commands, with parser tests, and the
  `run_script` payload schema.
- Integration docs loading, which is non-fatal and has fallback paths.
- Neuro Relay: the built-in host (`neuro-integration relay`) and the server's client, with
  relay-backed routing and relay state on the dashboard.
- Catalog actions (`list_catalog_items`, `find_catalog_items`, `get_catalog_item`) and the
  extension actions (install, enable, disable, uninstall, list), with a state file.
- Desktop context: the agent's snapshot, and the server's periodic context sender
  (`NEURO_CONTEXT_POLL_SECONDS`).
- The vision client (`NEURO_VISION_URL`) and `nd-vision-server`.
- The dashboard (React and Vite, TypeScript): status, permissions with requests,
  extensions with live runtime state, and games.
- Headless and command-line-only operation.
- Removed: the Rust executor, the C++ process handler, the relay shim, and the `native/`
  placeholders. The history is in `CHANGELOG.md`.
