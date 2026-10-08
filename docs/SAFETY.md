# Safety systems and firewalls

Every control listed here exists in the code, has a test, and can be turned on or
off without editing code. The rule the project follows: **the desktop is never
driven by something the operator did not allow, and every refusal is visible.**

## 1. Where the safety code lives

| Layer | File | What it enforces |
| ----- | ---- | ---------------- |
| Bridge (Go) | `desktop/apps/neuro-integration/safety.go` | stop switch, kill-switch file, audit log |
| Bridge (Go) | `desktop/apps/neuro-integration/permissions.go` | permission scopes, allow/deny lists, default-allow |
| Bridge (Go) | `desktop/apps/neuro-integration/shellfirewall.go` | shell command allow-list and dangerous-pattern rules |
| Bridge (Go) | `desktop/apps/neuro-integration/ratelimit.go` | per-scope actions/minute limits |
| Bridge (Go) | `desktop/apps/neuro-integration/admin_server.go` | token-gated dashboard, loopback-only defaults |
| Bridge (Go) | `desktop/apps/neuro-integration/executor_hub.go` | executor shared secret, per-request timeout |
| Controller (Python) | `desktop/backend/python/controller/shell.py` | the same shell rules again, on the machine that runs them |
| Controller (Python) | `desktop/backend/python/controller/gui_stub.py` | refuses input actions with no display |
| Controller (Python) | `desktop/backend/python/controller/launcher.py` | launch allow-list for `system` scope |
| Bundle scripts | `desktop/scripts/bundle/*.sh` | refuse to run as root / sudo |

## 2. Layers, in the order an action passes through them

```
Neuro / relay watcher / dashboard
        │
        ├─ 1. stop switch        paused?  kill-switch file?  → refuse
        ├─ 2. deny list          NEURO_DENY_ACTIONS         → refuse
        ├─ 3. permission scope   scope allowed? allow-list? → refuse
        ├─ 4. rate limit         scope limits, e.g. 30/min  → refuse
        ├─ 5. shell firewall     allow-list + regex rules   → refuse
        └─ 6. audit log          every decision, allowed or refused
```

Refusals name the reason and, where the operator can change it, the setting to
change (`NEURO_SHELL_ALLOWLIST`, the dashboard's Permissions tab, ...).

## 3. The operator brake

| Control | How to use it |
| ------- | ------------- |
| Pause | `POST /api/control/pause` (dashboard button) or `NEURO_PAUSED=1` |
| Resume | `POST /api/control/resume`, or the dashboard button |
| Kill switch | create the file in `NEURO_KILL_SWITCH_FILE`, or `POST /api/control` with `{"kill_switch": true}` |
| Release input | `POST /api/games/release` — releases every held key and mouse button |

While stopped, only release/status/session-end actions run. The stop switch is
checked before permissions, so it cannot be bypassed by a permissive policy, and
it applies to commands that arrive through the relay as well as through Neuro.

## 4. Permissions

Scopes are `input`, `filesystem`, `process`, `network`, `system`, `vision`,
`game`, `shell`.

* `default_allow` applies only to actions whose scope is not listed. Shell and
  system are deliberately excluded from that shortcut.
* `allowed_actions` / `denied_actions` override per action; hard denials
  (`denied_actions`) always win.
* `shell` and `system` need an explicit entry in the policy. An allow-list entry
  for `shell_command` alone is not enough: the scope must be enabled too.
* Each scope can carry limits, e.g. `{"allowed": true, "max_actions_per_minute": 30}`.
* The dashboard writes the policy atomically and applies it to the running
  bridge immediately (`PUT /api/permissions`); a policy that does not parse is
  rejected and the previous one stays live.

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

* The dashboard binds to `127.0.0.1:8300` by default. Binding anywhere else
  requires `NEURO_ADMIN_TOKEN`; non-GET requests without the token get `403`,
  and the bridge warns loudly at startup.
* The executor port takes a shared secret (`NEURO_EXECUTOR_TOKEN`). Without it,
  only loopback clients can connect.
* The relay link sends `auth_token` from `NEURO_RELAY_TOKEN` and reports a
  mismatch as an actionable error instead of silently retrying forever.
* No component opens an inbound port to the internet by itself.

## 7. What is audited

`NEURO_AUDIT_LOG` writes JSON lines: action name, decision (`accepted`,
`refused`, `error`), reason, and the command for shell actions. The dashboard's
`/api/audit` endpoint tails it. Every refusal path in the bridge records an
entry, which is what makes a stuck model debuggable after the fact.

## 8. Verify it yourself

```bash
# Go: safety, policy, rate limit, shell firewall, executor auth, relay rules
cd desktop/apps/neuro-integration && go test -count=1 ./...

# Python: headless refusal, shell firewall, primitives — no display needed
cd desktop/backend/python && python3 -m unittest discover -s tests -v

# Repository-wide: JSON, catalog, docs, C++ parser/lifecycle, ollama brain
python3 desktop/tools/ci/repo_checks.py

# Live: refusal is reported, not silent
curl -s -H "X-ND-Token: $TOKEN" -H 'Content-Type: application/json' \
  -d '{"command":"ls"}' http://127.0.0.1:8300/api/control   # pause etc.
```

Tests that guard the invariants specifically:
`TestScopeRequiresExplicitConsent` (shell/system cannot be handed out by
default-allow or an allow-list entry), `TestShellFirewall*`,
`TestRateLimit*`, `TestExecutorHubRequiresToken`,
`TestExecutorHubRejectsWrongToken`, `test_headless_and_shell.py`.
