# Safety

This is the order of defences, where each one lives, and what it does when it refuses.
Read it before you give Neuro a new capability, and before you open a port.

## 1. Where the safety code lives

| Layer | Where | What it does |
|-------|-------|--------------|
| Operator brake | `desktop/apps/neuro-integration/safety.go` | pause, and the kill-switch file; a small set of actions still runs while stopped |
| Policy | `permissions.go`, `permission_requests.go`, `admin_permissions.go` | scopes (on, requestable), allow and deny lists, approvals, and the requests Neuro files |
| Rate limits | `ratelimit.go` | a sliding window per scope, shared by Neuro and by watchers |
| Shell firewall | `shellfirewall.go` (server) and `controller/shell.py` (agent) | allowlist, dangerous patterns, timeouts |
| Executor hub | `executor_hub.go` | a token for every agent, and no listening beyond loopback without one |
| Dashboard | `admin_server.go` | loopback by default; writes need a token elsewhere |
| Relay | `relay_host.go` (host), `relay.go` (client, and the gates on watcher commands) | a token, browsers refused, frame limits |
| Extensions and catalog | `catalog_trust.go`, `extensions.go`, `mcp.go` | signatures, publishers, install mode, a clean environment for MCP servers |
| Vision | `vision.go`, `desktop/apps/nd-vision-server` | a token off loopback, reads only inside one directory |
| Audit | `NEURO_AUDIT_LOG` (JSON lines) | every refusal and every accepted action |

## 2. Layers, in the order an action passes through them

1. **Connection.** Neuro's API is outbound. The executor hub checks its token. The
   dashboard needs a token off loopback. The relay needs its token, and refuses
   browsers.
2. **Operator brake.** A paused bridge, or a present kill-switch file, refuses every
   action except the stop-safe set (section 3).
3. **Hard deny.** Actions in `NEURO_DENY_ACTIONS` are refused whatever the policy says.
   The dashboard cannot remove them.
4. **Policy.** The action's scope must be on, and the action must not be on the deny
   list. A scope that is off but requestable is refused with a way to ask (section 4).
5. **Rate limit.** Each scope has an optional `max_actions_per_minute`. Over the limit,
   the refusal says when to try again.
6. **Validation.** Parameters are checked before anything runs. A refusal names the
   action and the parameter, and gives a valid example, so a small model can correct
   itself on its next turn.
7. **Executor.** The agent applies its own checks. The shell firewall is one of them.
8. **Audit.** Each refusal and each accepted action is written to the audit log.

A watcher command that comes through the relay passes through the same gates, in
the same order (`denyRelayCommand`). It is counted against the same rate limit.

## 3. The operator brake

- `NEURO_PAUSED=1` starts the bridge paused. The dashboard's Pause and Resume buttons
  call `POST /api/control/pause` and `POST /api/control/resume`.
- The kill switch is a file. `NEURO_KILL_SWITCH_FILE` names it, and while it exists every
  action is refused. Deleting the file resumes the bridge.
- While the bridge is stopped, these still run: `key_release_all`, `get_status`,
  `send_desktop_context`, `game_release_all`, `game_status`, `game_end_session`,
  `desktop_guide`, and `reset_controls`. They release held input, report status, and
  tell Neuro how to recover. `request_permission` does not run while stopped.

## 4. Permissions

The policy is a JSON file (`NEURO_PERMISSIONS_FILE`). `default_allow` decides what happens to an
action that no scope or list covers, and it is `false` by default. Without a file the server uses a
built-in default, and an action is denied unless its scope is on.

| Scope | Default | Notes |
|-------|---------|-------|
| `input` | on | mouse and keyboard |
| `process`, `network`, `vision`, `game` | on | |
| `filesystem`, `system`, `shell`, `extensions` | **off** | `shell` and `system` need explicit consent |

Each scope has `allowed` and `requestable`:

- **Requestable** means Neuro may call `request_permission` with a reason and a time
  (5 minutes, 15, 1 hour, 4 hours, or until revoked). The request shows on the Permissions
  page. Vedal approves it or denies it. Approvals and requests are in memory, so a restart
  clears them.
- **Always allowed**, under any policy: `desktop_guide`, `reset_controls`, and
  `request_permission`. A deny-all policy cannot lock Neuro out of the way to recover or
  to ask. A test pins this.

The dashboard saves policy changes to `NEURO_PERMISSIONS_FILE` (`PUT /api/permissions`)
and applies them at once. Keep a copy of the file before you edit it on the dashboard.

## 5. Shell firewall

`shell_command` runs only when all of these hold:

1. the `shell` scope is enabled **and** the action is not denied;
2. the program name is in `NEURO_SHELL_ALLOWLIST` (empty allow-list = nothing
   runs);
3. the command line matches none of the built-in dangerous patterns (recursive
   force-delete of `/`, `dd` onto a block device, fork bombs, piping a download
   straight into a shell, `chmod 777 /`, ...);
4. `NEURO_SHELL_DENY_PATTERNS` (extra regexes) do not match;
5. the command finishes inside `NEURO_SHELL_TIMEOUT` seconds (default 20, max
   120). Output is clipped so it cannot swamp the context window.

The same rules are implemented twice, on purpose: in the bridge (so a bad
command never leaves the machine that owns the policy) and in the Python
controller (so the executor is not defenceless if it is driven directly).

## 6. Network exposure

- **Dashboard.** It binds to `127.0.0.1:8300` by default. Binding anywhere else needs
  `NEURO_ADMIN_TOKEN`. Non-GET requests without the token get `403`, and the bridge warns
  loudly at startup.
- **Executor hub.** With `NEURO_EXECUTOR_TOKEN` set, every agent must present it. Without
  one, the hub listens on loopback only. It refuses to start on any other address and
  says why. Before this rule, a hub with no token accepted any client as the executor,
  and that client received Neuro's commands.
- **Relay host.** Loopback by default. It refuses any request with an `Origin` header
  (so a web page cannot connect), refuses binary frames, refuses the upstream sample token
  and any token under 16 characters, and limits each connection's frame rate. Anything
  that holds the token can register, so keep the token secret.
- **Vision server.** Loopback by default. Binding elsewhere needs `NEURO_VISION_TOKEN`. It
  reads image files only inside `NEURO_VISION_ROOT`, and never opens a path without one.
- **MCP servers.** Child processes that the bridge starts only after Vedal enables them,
  and only from the signed catalog. A child gets a small base environment and only the
  variables its catalog entry lists.
- No component opens an inbound port to the internet by itself.

## 7. Extensions and the catalog

- Catalog items are signed. An item whose signature does not match is refused in every
  mode. An item signed by a publisher the server does not list is refused too.
- Installing never takes a URL from Neuro or from the dashboard. The id is looked up in the
  signed index.
- `NEURO_EXTENSION_INSTALL_MODE=metadata_only` (the default) records the install and
  fetches nothing. `git_clone` fetches the item's pinned commit from its `https`
  repository, and only for a verified item.
- `NEURO_EXTENSIONS_ALLOW_UNSIGNED` lets unsigned items be fetched. It exists for a lab
  machine. Do not set it on a machine Neuro controls.
- The shipped signature uses a key that the maintainer generated for this repository.
  Rotate to your own publisher key before you rely on the signatures. See
  `docs/PRODUCTION_TODO.md`.

## 8. What is audited

`NEURO_AUDIT_LOG` writes JSON lines. Each line has the action name, the decision
(`accepted`, `refused`, or `error`), a reason for a refusal, and the command for a shell
action. Relay commands are recorded as `relay_command`. The dashboard's `/api/audit`
endpoint reads it. Every refusal path records an entry, which is what makes a stuck model
debuggable after the fact. Tokens are never written.

## 9. Verify it yourself

```bash
cd desktop/apps/neuro-integration
go test -race -count=1 ./...                                  # everything
go test -count=1 -run 'Relay|Watcher' -v ./                   # the relay and watcher gates
go test -count=1 -run 'ExecutorHub' -v ./                     # the hub's token and loopback rules
go test -count=1 -run 'WeakModel|EscapeHatch|ExamplePolicy' -v ./   # refusals and the escape hatch
python3 ../../tools/ci/repo_checks.py                         # the repository checks (run from this folder)
```

The CI workflow runs all of these, and a release is blocked unless every job passes.
