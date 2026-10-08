# Neuro Relay: how Neuro Desktop coexists with other integrations

[Neuro Relay](https://github.com/Nakashireyumi/neuro-relay) ("Nakurity") sits
between Neuro and the integrations and lets several of them share one Neuro
connection. Neuro Desktop supports two ways of working with it.

## Mode 1 — a normal Neuro SDK client (recommended)

Point the server at the relay's *Nakurity backend* socket instead of Neuro:

```bash
NEURO_SDK_WS_URL=ws://127.0.0.1:8001 ./neuro-integration
```

The server then behaves like any other `neuro_api` client: it sends `startup`
with `"game": "Neuro Desktop"`, registers its actions, and answers `action`
messages. The relay multiplexes those registrations with the game integrations'
own ones and forwards the merged set to Neuro. That is the whole "run alongside
an existing integration" story — the relay (not this project) owns routing.

Requirements on our side, verified by `bridge_e2e_test.go`:

* `startup` carries `game`, because the relay names the client from it
  (`data.get("game") or client_name or "unknown-client"`) and uses that name to
  route results back.
* action names must not collide with the peer integrations' names. Use
  `NEURO_RESERVED_ACTIONS=move_mouse_to,minecraft.*` (comma separated, `*`
  allowed at the end) to leave names to the owner; the reserved names are
  skipped when registering actions with Neuro *and* with the relay.

## Mode 2 — an intermediary integration (watchers, delegation)

Set `NEURO_RELAY_URL` (and `NEURO_RELAY_TOKEN`) to join the relay's intermediary
as an integration:

```bash
NEURO_RELAY_URL=ws://127.0.0.1:8765 \
NEURO_RELAY_TOKEN="$INTERMEDIARY_AUTH_TOKEN" \
NEURO_RELAY_NAME="Neuro Desktop" \
./neuro-integration
```

The first frame is exactly what the relay validates:

```json
{"type": "integration", "name": "Neuro Desktop", "auth_token": "…"}
```

(`role`/`platform` are extra keys the relay ignores.) After that the server:

* registers its action schemas with an `{"event": "register_actions", "actions":
  {"<name>": {"name", "description", "kind", "schema"}}}` message — the
  name → schema mapping the intermediary expects, namespaced by the relay as
  `<integration>.<action>`;
* announces a `status` event (platform, whether an executor is connected, how many
  game profiles are loaded);
* accepts watcher commands: the relay forwards `{"from_watcher": …, "cmd":
  {"action": …, "params": …}}` and the server runs them through the same
  permission pipeline as a Neuro action, so a watcher cannot bypass the policy;
* records peers from `integration_connected`, `integration_registered_actions`
  and `integration_disconnected`, so `/api/relay` shows who else is on the relay
  and which actions they own. When a game is in `external`/`auto` mode, the
  refusal message names the actions of the integration that owns it.

Our own registration is echoed back by the relay as `integration_connected`;
the state filter ignores events whose name is ours, otherwise the bridge reports
itself as a coexisting integration (`peer_count` would never reach 0).

### What the intermediary path does *not* do

Actions registered through the intermediary are **not** forwarded to Neuro by
the relay. In its source, `intermediary.py` handles `register_actions` by storing
it in `self.action_registry` and notifying watchers; only registrations from
Neuro-SDK clients on its backend socket reach Neuro
(`linker.register_actions → actions/register`). It also expects a *list* of
action objects there, while the intermediary stores a name → schema *mapping* —
the two shapes cannot both be satisfied by one message.

So: **use Mode 1 to give Neuro actions**; use Mode 2 for watcher control,
visibility, and peer awareness. This is a limitation of the upstream relay, and
it is why `desktop/tools/relay-compat/run_relay.py` exists.

## Running the relay locally

No published `neuro-api` release can start the relay unmodified (0.x/1.x have no
`neuro_api.server`, 2.x/3.x leave five methods abstract, 4.x adds abstract
`get_character_id`/`get_websocket_session_id`). The compat shim supplies what is
missing:

```bash
git clone https://github.com/Nakashireyumi/neuro-relay /tmp/neuro-relay
python3 desktop/tools/relay-compat/run_relay.py /tmp/neuro-relay/src
# Intermediary on ws://127.0.0.1:8765, Nakurity backend on ws://127.0.0.1:8001
```

`NEURO_RELAY_TOKEN` must equal `intermediary.auth_token` in the relay's
`src/resources/authentication.yaml`. A rejected registration is reported in
`/api/relay` as `last_error` with the relay's own message.

## Health in the dashboard

`GET /api/relay` returns:

```json
{"enabled": true, "url": "ws://127.0.0.1:8765", "connected": true,
 "registered": true, "peer_count": 0, "peer_actions": {},
 "process_restarts": 0, "last_event": "registered", "last_seen": "…",
 "reserved_actions": ["move_mouse_to"], "last_error": ""}
```

`last_error` is cleared as soon as a `registered` event arrives, so a stale
rejection message never looks current. `process_restarts` counts how often a
relay process started by the server (`NEURO_RELAY_COMMAND`) had to be restarted.

## A first-party relay

The shim proves the protocol, not the product: it is a third-party Python
package with a config file, no packaging, and an upstream shape mismatch
(above). The plan in `docs/PRODUCTION_TODO.md` is to keep this compatibility
layer working and, when the "run alongside other integrations" path needs to be
bulletproof for users, implement the intermediary protocol in the Go server and
ship it as `neuro-relay` next to the server binary: same registration envelope,
same watcher command shape, same registration semantics for peers, but one
language, one release, and no Python runtime for the relay itself.
