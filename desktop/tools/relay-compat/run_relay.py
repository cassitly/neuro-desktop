#!/usr/bin/env python3
"""Start Neuro Relay (Nakashireyumi/neuro-relay) with a small compatibility shim.

Why this exists
---------------

Run the relay as-is and it does not start at all:

    $ python -m dev.nakurity
    TypeError: Can't instantiate abstract class NakurityBackend with abstract
    methods get_character_id, get_websocket_session_id

`src/dev/nakurity/server.py` subclasses three abstract `neuro_api` server
classes but only implements part of the interface. Checked against every
published `neuro-api` release:

    0.x / 1.x   no `neuro_api.server` module at all -> ImportError
    2.x / 3.x   classes exist, but `get_next_id`, `handle_actions_register`,
                `handle_actions_unregister`, `handle_actions_force` and
                `handle_action_result` stay abstract -> TypeError
    4.x         same, plus `get_character_id` / `get_websocket_session_id`

So the relay cannot be started with any current dependency set — nothing to do
with Neuro Desktop. This wrapper imports the relay's own modules, fills in the
missing methods with the behaviour the rest of the relay expects, and then runs
`dev.nakurity.__main__` unchanged. It is a bridge for upstream code, not a
fork: nothing is patched on disk.

Usage
-----

    python3 desktop/tools/relay-compat/run_relay.py /path/to/neuro-relay/src

    # or with the environment variable
    NEURO_RELAY_SRC=/path/to/neuro-relay/src \\
        python3 desktop/tools/relay-compat/run_relay.py

The relay's own configuration (`src/resources/authentication.yaml`) still
decides the ports and the `auth_token`; the defaults are the intermediary on
`ws://127.0.0.1:8765` and the Nakurity Backend on `ws://127.0.0.1:8001`.
"""

from __future__ import annotations

import os
import sys
import uuid
from typing import Any, Dict, List, Tuple


def _relay_src(argv: List[str]) -> str:
    if len(argv) > 1:
        return os.path.abspath(argv[1])
    from_env = os.environ.get("NEURO_RELAY_SRC", "").strip()
    if from_env:
        return os.path.abspath(from_env)
    print(
        "usage: run_relay.py /path/to/neuro-relay/src\n"
        "       (or set NEURO_RELAY_SRC)",
        file=sys.stderr,
    )
    raise SystemExit(2)


def _install_shim() -> None:
    """Give NakurityBackend the methods the abstract base classes require."""
    from dev.nakurity import server as relay_server  # noqa: WPS433 - imported after sys.path setup

    backend_cls = relay_server.NakurityBackend

    def get_next_id(self) -> str:
        """Unique id for each command the backend sends."""
        return str(uuid.uuid4())

    def get_character_id(self) -> str:
        """The relay is transparent; Neuro's own id is not ours to invent."""
        return getattr(self, "_character_id", "unknown")

    def get_websocket_session_id(self) -> str:
        if not hasattr(self, "_websocket_session_id"):
            self._websocket_session_id = str(uuid.uuid4())
        return self._websocket_session_id

    def handle_actions_register(self, data: Any = None) -> None:
        """Called by neuro_api when a client registers actions.

        The relay records registrations itself (see `run_server`), so this only
        keeps the library contract satisfied.
        """
        return None

    def handle_actions_unregister(self, data: Any = None) -> None:
        return None

    def handle_actions_force(self, data: Any = None) -> None:
        return None

    def handle_action_result(self, data: Any = None) -> None:
        return None

    def register_action(self, action: Any = None) -> None:
        return None

    def unregister_action(self, action_name: Any = None) -> None:
        return None

    def clear_registered_actions(self) -> None:
        return None

    def submit_call_async_soon(self, cb, *args):  # noqa: D401 - matches upstream name
        """Already implemented by the relay; kept for older base classes."""
        import asyncio

        loop = asyncio.get_event_loop()
        loop.call_soon(cb, *args)

    def add_context(self, game_title: str, message: str, reply_if_not_busy: bool):  # noqa: D401
        """Fallback for very old bases: forward context to watchers if possible."""
        intermediary = getattr(self, "intermediary", None)
        if intermediary is None:
            return
        import asyncio

        asyncio.create_task(
            intermediary._notify_watchers(  # noqa: SLF001 - upstream internals
                {
                    "event": "add_context",
                    "game_title": game_title,
                    "message": message,
                    "reply_if_not_busy": reply_if_not_busy,
                }
            )
        )

    patched = {
        "get_next_id": get_next_id,
        "get_character_id": get_character_id,
        "get_websocket_session_id": get_websocket_session_id,
        "handle_actions_register": handle_actions_register,
        "handle_actions_unregister": handle_actions_unregister,
        "handle_actions_force": handle_actions_force,
        "handle_action_result": handle_action_result,
        "register_action": register_action,
        "unregister_action": unregister_action,
        "clear_registered_actions": clear_registered_actions,
    }

    added = []
    for name, function in patched.items():
        if getattr(backend_cls, name, None) is None or getattr(
            getattr(backend_cls, name), "__isabstractmethod__", False
        ):
            setattr(backend_cls, name, function)
            added.append(name)

    # `add_context` / `submit_call_async_soon` already exist on the relay class;
    # only fill them in when an older base still marks them abstract.
    for name, function in (("add_context", add_context), ("submit_call_async_soon", submit_call_async_soon)):
        if getattr(getattr(backend_cls, name, None), "__isabstractmethod__", False):
            setattr(backend_cls, name, function)
            added.append(name)

    # An ABC caches __abstractmethods__ at class-creation time, so filling the
    # attributes in is not enough: recompute the frozen set ourselves.
    remaining = sorted(
        name
        for name in getattr(backend_cls, "__abstractmethods__", ())
        if getattr(getattr(backend_cls, name, None), "__isabstractmethod__", False)
    )
    if remaining:
        raise SystemExit(
            "relay compat shim is incomplete; still abstract: " + ", ".join(remaining)
        )
    backend_cls.__abstractmethods__ = frozenset()

    print(f"[relay-compat] filled in {len(added)} abstract method(s): {', '.join(sorted(added))}")
    print("[relay-compat] starting the relay unchanged (configuration comes from authentication.yaml)")


def main() -> int:
    src = _relay_src(sys.argv)
    if not os.path.isdir(os.path.join(src, "dev", "nakurity")):
        print(f"{src} does not look like a neuro-relay src directory", file=sys.stderr)
        return 2

    sys.path.insert(0, src)
    _install_shim()

    # Import the relay after the shim so its module-level work (config loading,
    # port constants) happens exactly once.
    import runpy  # noqa: WPS433

    runpy.run_module("dev.nakurity", run_name="__main__", alter_sys=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
