import sys
import json
from actions import move_mouse, click, type_text

def handle_command(cmd):
    action = cmd.get("action")

    if action == "move_mouse":
        move_mouse(cmd["x"], cmd["y"])

    elif action == "click":
        click(cmd["x"], cmd["y"], cmd.get("button", "left"))

    elif action == "type":
        type_text(cmd["text"])

    elif action == "ping":
        pass

    else:
        raise ValueError(f"Unknown action: {action}")

def main():
    for line in sys.stdin:
        try:
            cmd = json.loads(line)
            handle_command(cmd)
            print(json.dumps({"status": "ok"}), flush=True)
        except Exception as e:
            print(json.dumps({
                "status": "error",
                "error": str(e)
            }), flush=True)

if __name__ == "__main__":
    main()
