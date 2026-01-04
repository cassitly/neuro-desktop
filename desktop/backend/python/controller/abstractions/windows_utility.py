import win32gui
import pyautogui
import win32con
import time
import random

from ..libraries.window_info import WindowInfo
from ..libraries.mouse_pathfinder import AlgorithmicPath


# FSW = Fullscreen Window
class FSWUtils:
    """## Fullscreen Window Utils

    Provides a bunch of methods for an AI model to use to control a fullscreen window.

    Static Methods use the pyautogui library to control the mouse and keyboard, to do
    those given actions.

    While the instance methods use the win32gui library to control the window, to do
    those given actions.

    The difference in these two provide an instant, and a slower, way to control the window.
    The slower way can be used for a better streaming experience (as an algrothim is used for random mouse movements).
    """
    pathfinder = AlgorithmicPath()

    @staticmethod
    def minimize():
        """Minimize the current fullscreen window"""
        FSWUtils.pathfinder.move_to(1800 + random.randint(-3, 4), 50, 0.05)
        time.sleep(random.uniform(0.15, 0.4))
        pyautogui.click(button='left')

    @staticmethod
    def close(confirm=False):
        """Close the current fullscreen window
        
        You have to confirm, you want to close the window.
        Otherwise the mouse will just hover over the close button.
        """
        FSWUtils.pathfinder.move_to(1900, 50 + random.randint(-3, 2), 0.05)
        time.sleep(random.uniform(0.15, 0.4))
        if confirm: pyautogui.click(button='left')

    @staticmethod
    def unfullscreen():
        """Unfullscreen the current window"""
        FSWUtils.pathfinder.move_to(1850 + random.randint(-3, 3), 50 + random.randint(-2, 2), duration=0.05)
        time.sleep(random.uniform(0.15, 0.4))
        pyautogui.click(button='left')

    def __init__(self, window: WindowInfo):
        self.window = window

    def maximize(self):
        """Fullscreen a given window"""
        win32gui.ShowWindow(self.window.hwnd, win32con.SW_MAXIMIZE)

    def restore(self):
        """Unfullscreen the given window"""
        win32gui.ShowWindow(self.window.hwnd, win32con.SW_RESTORE)

    def close(self):
        """Close the given window"""
        win32gui.PostMessage(self.window.hwnd, win32con.WM_CLOSE, 0, 0)