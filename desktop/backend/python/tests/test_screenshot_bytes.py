"""A screenshot leaves the client as bytes, never as a path.

The server may run on another PC, so it cannot open a file that the client names.
These tests pin the status reply to base64 PNG, and check the size limits.
"""

import base64
import pathlib
import sys
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from controller import agent as agent_module  # noqa: E402
from controller import desktop  # noqa: E402

FAKE_PNG = b"\x89PNG\r\n\x1a\nfake-image-bytes"


class ScreenshotIsBytesTest(unittest.TestCase):
    def test_status_returns_base64_png_and_no_path(self):
        agent = agent_module.Agent()
        agent.headless = False
        agent.monitor.capture_screen_png = lambda *args, **kwargs: FAKE_PNG

        data = agent.status({"capture_screenshot": True})

        self.assertNotIn("screenshot_path", data)
        self.assertEqual(base64.b64decode(data["screenshot_png_b64"]), FAKE_PNG)

    def test_status_ignores_a_path_asked_for_by_the_server(self):
        agent = agent_module.Agent()
        agent.headless = False
        agent.monitor.capture_screen_png = lambda *args, **kwargs: FAKE_PNG

        data = agent.status({"capture_screenshot": True, "screenshot_path": "/etc/passwd"})

        self.assertNotIn("screenshot_path", data)
        self.assertEqual(base64.b64decode(data["screenshot_png_b64"]), FAKE_PNG)

    def test_no_capture_when_not_asked_or_headless(self):
        agent = agent_module.Agent()
        agent.headless = True
        called = []
        agent.monitor.capture_screen_png = lambda *args, **kwargs: called.append(1) or FAKE_PNG

        self.assertIsNone(agent.status({"capture_screenshot": True})["screenshot_png_b64"])
        agent.headless = False
        self.assertIsNone(agent.status({})["screenshot_png_b64"])
        self.assertEqual(called, [])


@unittest.skipUnless(desktop.Image is not None, "Pillow is not installed")
class ScreenshotSizeTest(unittest.TestCase):
    def test_a_large_screen_is_scaled_to_the_side_limit(self):
        image = desktop.Image.new("RGB", (3840, 2160), (10, 20, 30))
        small = desktop._fit_within(image, desktop.SCREENSHOT_MAX_SIDE)
        self.assertLessEqual(max(small.size), desktop.SCREENSHOT_MAX_SIDE)

    def test_a_small_screen_is_not_scaled(self):
        image = desktop.Image.new("RGB", (800, 600), (10, 20, 30))
        self.assertEqual(desktop._fit_within(image, desktop.SCREENSHOT_MAX_SIDE).size, (800, 600))

    def test_encoded_png_is_a_png_under_the_byte_limit(self):
        image = desktop.Image.new("RGB", (1600, 900), (1, 2, 3))
        data = desktop._encode_png(image)
        self.assertTrue(data.startswith(b"\x89PNG"))
        self.assertLess(len(data), desktop.SCREENSHOT_MAX_BYTES)


if __name__ == "__main__":
    unittest.main()
