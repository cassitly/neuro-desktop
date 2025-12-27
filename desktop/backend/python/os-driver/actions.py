import pyautogui
import time

def move_mouse(x, y):
    pyautogui.moveTo(x, y, duration=0.1)

def click(x, y, button="left"):
    pyautogui.click(x=x, y=y, button=button)

def type_text(text):
    pyautogui.write(text, interval=0.02)
