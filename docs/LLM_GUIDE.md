# Driving Neuro Desktop with a small or "dumb" model

Neuro Desktop works with strong models out of the box, but most of the trouble
with desktop control comes from weak models: wrong parameter names, guessed
action names, "I moved the mouse" without doing anything, or chain-calling
`key_press` instead of a named game action.

Everything here is written so a 2-8B model can still succeed. Nothing depends on
the model being able to read JSON Schema, use tools reliably, or plan ahead.

## 1. What the surface gives a weak model

| Signal | Where it comes from | Why it helps |
| ------ | ------------------- | ------------ |
| One-sentence descriptions | every action's `description` | the model can pick by name and description alone |
| Parameter list in plain text | `/api/actions` → `params`, e.g. `direction (string, required: forward\|back\|left\|right), seconds (number)` | no JSON Schema knowledge needed |
| Required parameters | `required` array and the `(required: ...)` marker in `params` | the single most common failure is a missing parameter |
| Corrective error text | `action-registry.go` → `missingParamsReason`, `expectedParamsHint` | a failed call answers with the fix: `game_move needs the parameter "direction". Call it again with {direction, seconds, steps} (required: direction).` |
| Ground truth about the desktop | `get_status`, `game_observe`, `game_list` | the model never has to guess what is on screen |
| A ready-made briefing | `desktop_guide`, `game_start_session` output | the model gets the control map instead of inventing it |

The registration sent to Neuro contains the same information, so the model sees
it *before* it acts, not after a failure.

## 2. Shortest useful system prompt

```
You control a desktop through Neuro Desktop.

Rules:
1. Call get_status first, always. It tells you the platform and whether the
   executor is connected.
2. Only use actions from the list you were given, with the exact parameter
   names shown after them. Never invent an action or a parameter.
3. If an action fails, read the error: it tells you the parameter it wants.
   Fix the call once; if it fails again, call get_status and re-read the list.
4. Playing a game: call game_list, then game_start_session, then use game_move
   {direction, seconds} and game_look {dx, dy}. Do not press raw keys unless
   game_start_session says raw keys are allowed.
5. Nothing you say moves the mouse. Only the actions do.
6. Keep going in small steps: one action, look at the result, then the next.
```

The last two rules matter the most. Weak models often announce an action without
calling it, or call five actions in one turn and lose track of the error.

## 3. Parameter shapes worth prompting explicitly

| Action | Call it like | Common weak-model mistake |
| ------ | ------------ | ------------------------- |
| `move_mouse_to` | `{x: 640, y: 360}` | passing `"x": "640px"` — coordinates are numbers |
| `mouse_click` | `{button: "left"}` | omitting `button` (allowed, defaults to left) |
| `type_text` | `{text: "hello"}` | `{message: ...}` |
| `key_press` | `{key: "enter"}` | passing `{keys: [...]}` — use `key_combo` for chords |
| `key_combo` | `{keys: ["ctrl", "s"]}` | more than 6 keys; chords are 1-6 |
| `game_move` | `{direction: "forward", seconds: 1}` | `{dir: "w"}`; directions are `forward/back/left/right` |
| `game_look` | `{dx: 120, dy: 0}` | using absolute pixel coordinates; these are relative |
| `game_action` | `{name: "jump"}` | inventing a name — call `game_list` for the real ones |
| `shell_command` | `{command: "ls -la"}` | expecting a free-form shell; the allow-list decides what runs |
| `game_start_session` | `{mode: "auto"}` | forcing `nd` for a game owned by another integration |

`/api/actions` is the authority for all of these at runtime, including any
change made through the dashboard.

## 4. Tactics that reliably rescue a weak model

1. **Ground first.** `get_status` → `game_observe` (or `game_list`) before any
   input action. A model that has just been told "the active window is
   Minecraft" stops guessing about the desktop.
2. **One action per turn while debugging.** The action result window is short
   (about 20 s); batching hides which call failed.
3. **Let the error do the teaching.** Do not re-prompt the model with a
   paraphrase of the failure: the failure text already contains the corrected
   parameter shape. Just resend it verbatim.
4. **Escalate, do not improvise.** If two `game_move` calls fail, the model
   should call `game_list` (control map) and `desktop_guide` (what this desktop
   can do) instead of trying raw keys.
5. **Use named actions over raw keys.** `game_action {name: "jump"}` survives
   keybind changes in the profile; `key_press {key: "space"}` does not.
6. **Never let the model "just press the buttons".** `shell_command` runs only
   what the allow-list permits, and raw keys work only when the profile allows
   them. Tell the model that in the prompt: it prevents most loops where a model
   keeps retrying a forbidden action.
7. **Bound the run.** `NEURO_DENY_ACTIONS`, the pause switch and the kill switch
   exist so a stuck model cannot cause damage while you fix the prompt.

## 5. Checking the surface yourself

```bash
# What the model will be offered, including parameter hints:
curl -s http://127.0.0.1:8300/api/actions | python3 -m json.tool | head -40

# The briefing a model gets for a game:
curl -s -H "X-ND-Token: $TOKEN" http://127.0.0.1:8300/api/games | python3 -m json.tool
```

`TestActionsExposeLLMFriendlyMetadata` (Go) enforces the invariants: every
action has a real description, every parameterised action exposes a schema, and
the plain-text `params` rendering mentions every parameter, its type and whether
it is required. If someone adds an action without that metadata, CI fails.
