"""Headless-safe stand-ins for the GUI libraries.

Command-line-only machines (containers, servers, CI, SSH sessions without X)
have no display, and the GUI libraries fail in different ways there:

* ``pyautogui`` raises while *importing* when ``DISPLAY`` is unset on Linux,
  which used to take the whole controller down before it could do anything.
* ``pynput``/``mss``/``pygetwindow`` either import fine and fail on use, or are
  not installed at all.

Neuro Desktop is still useful on such a machine — it can run shell commands,
report status and answer context — so instead of crashing, the input libraries
degrade into stubs that raise a clear, actionable error only if an action
actually tries to touch a mouse or keyboard that is not there.

The real libraries are used whenever they import successfully, so a normal
desktop session is unaffected.
"""

from __future__ import annotations

import os
import sys
from typing import Any


class NoDisplayError(RuntimeError):
    """Raised when an input action needs a display that is not available."""


def headless_requested() -> bool:
    """True when the operator asked for headless mode."""
    return os.environ.get("NEURO_HEADLESS", "").strip().lower() in {"1", "true", "yes", "on"}


def display_available() -> bool:
    """Best-effort answer to "is there a desktop session to drive?".

    On Windows and macOS a session is assumed (there is no cheap, reliable check),
    and ``NEURO_HEADLESS=0`` forces the answer to yes for tests that stub the
    libraries themselves.
    """
    forced = os.environ.get("NEURO_HEADLESS", "").strip().lower()
    if forced in {"0", "false", "no", "off"}:
        return True
    if forced in {"1", "true", "yes", "on"}:
        return False

    if sys.platform.startswith("win") or sys.platform == "darwin":
        return True

    return bool(os.environ.get("DISPLAY") or os.environ.get("WAYLAND_DISPLAY"))


def is_headless() -> bool:
    """The single answer the controllers use: no display, or forced headless."""
    return headless_requested() or not display_available()


def describe_headless_reason() -> str:
    if headless_requested():
        return "NEURO_HEADLESS is set"
    if sys.platform.startswith("linux") or sys.platform not in {"win32", "darwin"}:
        return "no DISPLAY/WAYLAND_DISPLAY in this session"
    return "no desktop session available"


def _no_display(*_args: Any, **_kwargs: Any) -> None:
    raise NoDisplayError(
        "This machine has no display session ("
        + describe_headless_reason()
        + "), so mouse/keyboard actions cannot run here. "
        "Use the shell_command action for command-line work, or run the executor "
        "on the desktop machine with `neuro-desktop --executor --server <bridge>:9876`."
    )


KEYBOARD_KEYS = [
    "a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o",
    "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z",
    "0", "1", "2", "3", "4", "5", "6", "7", "8", "9",
    "f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "f11", "f12",
    "enter", "esc", "tab", "space", "backspace", "delete", "insert",
    "up", "down", "left", "right", "home", "end", "pageup", "pagedown",
    "shift", "ctrl", "alt", "win", "cmd", "option", "command", "fn",
    "capslock", "numlock", "scrolllock", "printscreen", "pause", "menu",
    "+", "-", "*", "/", "=", ",", ".", ";", "'", "[", "]", "\\", "`",
]

#: Keys pyautogui understands on every platform we ship for.
VALID_KEYS = set(KEYBOARD_KEYS)


class StubPyAutoGUI:
    """A pyautogui-shaped object whose input methods raise NoDisplayError."""

    KEYBOARD_KEYS = KEYBOARD_KEYS
    FAILSAFE = False
    PAUSE = 0.0

    def __getattr__(self, name: str) -> Any:
        # size()/position() are used for coordinate maths; give plausible values
        # so context and geometry code keeps working without a display.
        if name == "size":
            return lambda: (1920, 1080)
        if name == "position":
            return lambda: (0, 0)
        if name in {"KEYBOARD_KEYS", "FAILSAFE", "PAUSE"}:
            return getattr(type(self), name)
        return _no_display

    def __repr__(self) -> str:  # pragma: no cover - debugging aid
        return "<pyautogui stub: no display session>"


def load_pyautogui() -> Any:
    """Import the real pyautogui, or return the stub when that is impossible."""
    if is_headless():
        return StubPyAutoGUI()
    try:
        import pyautogui  # noqa: WPS433 - runtime optional dependency

        return pyautogui
    except Exception:
        # Import can fail on Linux without X even when DISPLAY is set to a
        # dead socket; degrade instead of refusing to start.
        return StubPyAutoGUI()


def load_pynput_mouse() -> Any | None:
    """pynput's mouse module, or None when it cannot be used here."""
    if is_headless():
        return None
    try:
        from pynput import mouse  # noqa: WPS433 - runtime optional dependency

        return mouse
    except Exception:
        return None


def load_mss() -> Any | None:
    if is_headless():
        return None
    try:
        import mss  # noqa: WPS433 - runtime optional dependency

        return mss
    except Exception:
        return None


def load_pygetwindow() -> Any | None:
    if is_headless():
        return None
    try:
        import pygetwindow  # noqa: WPS433 - runtime optional dependency

        return pygetwindow
    except Exception:
        return None
