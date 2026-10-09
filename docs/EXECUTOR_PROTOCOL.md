# Executor protocol (server ↔ agent)

The server decides *what* to do; the agent is the only process that touches a
machine. They talk over one of three transports with the same message format:
newline-delimited JSON (one object per line, UTF-8, no length prefix).

* **agent → server** (`neuro-client --bridge host:9876` on the PC Neuro controls, or
  `python3 -m controller.agent --bridge host:9876` from source; both run the same agent).
  The agent always dials out to the hub, so it needs no inbound port. The server's side is
  `NEURO_EXECUTOR_LISTEN` (`-executor-listen`), which defaults to `127.0.0.1:9876`.
* **file IPC** (co-located only): the server writes the command to
  `NEURO_IPC_FILE` and polls `NEURO_IPC_FILE.response`.

`PROTOCOL_VERSION` is `2`; the server sends its version in `hello_ack` and both
sides log a mismatch instead of misbehaving.

## Frames

```jsonc
// agent -> server, first frame
{"type": "hello", "role": "executor", "version": "2", "token": "<optional>"}

// server -> agent
{"type": "hello_ack", "role": "bridge", "version": "2"}
{"type": "hello_nack", "error": "invalid or missing executor token"}

// server -> agent
{"type": "command", "id": "1730000000000000000",
 "command": {"type": "shell_command", "params": {"command": "ls -la"}}}

// agent -> server
{"type": "result", "id": "1730000000000000000", "success": true,
 "data": {"output": "exit code: 0\nstdout:\n…"}, "error": ""}

// liveness, both directions; the server sends one every 30s and drops an agent
// that has not answered for 90s
{"type": "ping", "id": "…"} / {"type": "pong", "id": "…"}
```

The handshake is bounded (10 s): a client that connects and never says hello is
closed, so a port probe cannot hold a slot. The token is compared in constant
time and is required whenever the hub is not bound to loopback.

**Frame size.** A frame is one line, and the hub reads it with a limit:

* the hello is capped at **64 KiB**, because it is read before the token is checked;
* every later frame is capped at **8 MiB**;
* a frame over its limit closes the connection. The hub never buffers past the limit,
  even when no newline ever arrives.

The largest normal frame is a status reply with a screenshot: the agent caps the PNG
at 3 MiB, which is about 4 MiB as base64.

## Commands

The authoritative list is `executorCommands` in
`desktop/apps/neuro-integration/executor_commands.go`, mirrored by
`EXECUTOR_COMMANDS` in `desktop/backend/python/controller/agent.py` and pinned by
a test on both sides.

| Command | Params | Notes |
| --- | --- | --- |
| `move_mouse_to` | `x`, `y`, `duration?` | clamped to the screen by the agent |
| `move_mouse_relative` | `dx`, `dy`, `duration?` | mouse-look (no absolute cursor) |
| `mouse_click` | `button?` | `left`, `right`, `middle` |
| `mouse_hold_for` | `button?`, `seconds?` | hold a button down, then release |
| `key_press` | `key` | one key or a shortcut string |
| `key_combo` | `keys[]` | pressed together |
| `key_hold_for` | `key`, `seconds` | for movement keys |
| `key_release_all` | — | safety release |
| `type_text` | `text` | types into the focused window |
| `run_script` | `script` | the ACTION script language (`TYPE`, `MOVE`, `SHELL`, …) |
| `shell_command` | `command`, `cwd?`, `timeout?` | shell firewall + allowlist; output is returned in `data.output` |
| `get_status` | `max_open_windows?`, `max_processes?`, `max_actions?`, `capture_screenshot?` | telemetry; `headless` tells the truth about the session; the screenshot comes back as bytes (below) |
| `execute_queue` / `clear_action_queue` | — | queue control (below) |
| `shutdown_gracefully` / `shutdown_immediately` | — | releases input, then exits |

Anything else in the action registry (desktop shell intents, catalog, extension
commands, `game_*`) is handled **inside the server** and never forwarded;
`sendToExecutor` rejects them with an explicit error rather than sending them to
an agent that could not run them.

## Screenshots travel as bytes

`get_status` with `capture_screenshot: true` returns the screen in the reply:

```jsonc
{"status": "running", "headless": false, ..., "screenshot_png_b64": "iVBORw0KGgo…"}
```

* `screenshot_png_b64` is a base64 PNG, scaled so its long side is at most 1 600 px and
  its size is at most 3 MiB. It is `null` when no screenshot was asked for, when the
  session is headless, or when capture failed.
* The server never opens a path that the agent names. An older `screenshot_path` field
  is ignored, and the server does not read files on the agent's behalf. The reason is
  that the server may run on another PC, and a path there would be an arbitrary file
  read. The server checks the bytes (PNG signature, base64, size limit of 8 MiB) before
  it uses them.
* The vision service receives the same bytes as `image_base64`. It also accepts
  `image_path`, but only inside its own `NEURO_VISION_ROOT`, and the server does not send it.

## Queueing

Each command may carry `execute_now` and `clear_after` (both default `true`), the
same contract the Rust executor had:

* input commands are queued; `execute_now` runs the queue, `clear_after` empties
  it afterwards. `{"execute_now": false}` therefore batches several primitives
  into one motion (used by the game layer to press a key *while* looking around).
* `execute_queue` and `clear_action_queue` do those two steps explicitly.

Failures are values, not exceptions: `success: false` with a message written for
a reader that has to decide what to do next ("This machine has no display
session… use the `shell_command` action").

## File IPC

Used when the server has no TCP client (the classic one-PC case). The agent polls
`NEURO_IPC_FILE`, **removes it before executing** (so a slow command cannot run
twice and the server's timeout can cancel it), and writes the response to
`NEURO_IPC_FILE.response` through a temporary file + rename, because the server
polls that path and must never read a half-written object. The server deletes a
stale response before writing a new command.

## Safety on the agent side

The agent trusts the server for *intent*, not for *bounds*: the shell allowlist
and deny patterns, the headless refusal, key validation and the 4 000-character
output cap are enforced in `desktop/backend/python/controller/`. A compromised
server cannot make the agent do more than the policy on that machine allows.

## Testing

* Server: `desktop/apps/neuro-integration/executor_hub_test.go`,
  `executor_commands_test.go`, `executor_liveness_test.go` (round trip, token
  rejection, slow command, timeout, replacement, ping/drop), `executor_frame_test.go`
  (the frame limits, an oversized hello refused before auth, a screenshot-sized reply
  kept whole), and `screenshot_test.go` (the PNG check and the size limit).
* Agent: `desktop/backend/python/tests/test_agent.py` (handshake, ping, malformed
  frames, rejection, queue semantics, file IPC, and the cross-language command
  parity check) and `test_screenshot_bytes.py` (the screenshot is scaled and encoded
  as bytes, and the status reply carries it).
* Simulator: `desktop/tools/fake-executor/fake_executor.py` — no input is ever
  generated, so the whole server can be exercised on a headless machine.
