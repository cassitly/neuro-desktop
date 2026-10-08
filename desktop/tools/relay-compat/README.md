# relay-compat

`run_relay.py` starts [Nakashireyumi/neuro-relay](https://github.com/Nakashireyumi/neuro-relay)
despite an upstream bug that stops it from starting at all.

## The bug

`src/dev/nakurity/server.py` subclasses three abstract `neuro_api` server classes
but implements only part of the interface, so instantiation fails:

```
TypeError: Can't instantiate abstract class NakurityBackend with abstract methods
get_character_id, get_websocket_session_id
```

Checked against every published `neuro-api` release:

| neuro-api | Result |
|-----------|--------|
| 0.x, 1.x | no `neuro_api.server` module → `ImportError` |
| 2.x, 3.x | classes exist; `get_next_id`, `handle_actions_register`, `handle_actions_unregister`, `handle_actions_force`, `handle_action_result` stay abstract |
| 4.x | same, plus `get_character_id` and `get_websocket_session_id` |

## What the shim does

It imports the relay's own modules, fills in exactly those methods with the
behaviour the rest of the relay expects (unique ids, no-op handlers, a stable
session id), recomputes `__abstractmethods__` (Python caches it at class creation),
and then runs `dev.nakurity.__main__` unchanged. Nothing on disk is patched.

## Usage

```bash
pip install "websockets==13.1" neuro-api pyyaml   # the relay uses the legacy websockets API
python3 desktop/tools/relay-compat/run_relay.py /path/to/neuro-relay/src
# or: NEURO_RELAY_SRC=/path/to/neuro-relay/src python3 desktop/tools/relay-compat/run_relay.py
```

Then point Neuro Desktop at it:

```bash
export NEURO_RELAY_ENABLED=true
export NEURO_RELAY_URL=ws://127.0.0.1:8765          # intermediary
export NEURO_RELAY_TOKEN=<intermediary.auth_token>  # default in the repo: super-secret-token
export NEURO_RESERVED_ACTIONS=""                    # names owned by other integrations
curl -s http://127.0.0.1:8300/api/relay             # connected/registered true
```

The relay's ports and token come from its own `src/resources/authentication.yaml`.
