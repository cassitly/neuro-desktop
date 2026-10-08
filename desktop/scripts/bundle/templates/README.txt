Neuro Desktop
=============

What is in this folder
----------------------
  @SERVER@            the server: connects to Neuro, serves the dashboard, and
                      forwards commands to the agent running on this PC
  agent/              the process that executes those commands (Python 3.9+)
  frontend/           dashboard client, served at http://127.0.0.1:8300/ui/
  catalog/            game profiles, extension state and the audio/vision catalog
  config/             example policy and integration config
  permissions.json    the active permission policy (edit it, or use the dashboard)
  integration-docs/   action reference and platform notes

Run it
------
  - Windows: start.bat
  - Linux/macOS: ./start.sh

Two machines (the usual setup for playing a game on a different PC)
------------------------------------------------------------------
  1. On the PC that runs Neuro:
       ./@SERVER@ --ws-url ws://<neuro-host>:8000 --executor-listen 0.0.0.0:9876
     Set NEURO_EXECUTOR_TOKEN so only your agent can connect, and open the port
     in the firewall.
  2. On the PC Neuro should control:
       cd agent
       python3 -m pip install -r requirements.txt
       python3 -m controller.agent --bridge <server-ip>:9876 --token <token>

Environment variables worth knowing
-----------------------------------
  NEURO_SDK_WS_URL         Neuro API websocket (default ws://localhost:8000)
  NEURO_EXECUTOR_TOKEN     shared secret required from agents
  NEURO_ADMIN_TOKEN        token for destructive dashboard calls
  NEURO_HEADLESS=1         no display session: mouse/keyboard refuse to run and
                           Neuro is told to use shell_command instead
  NEURO_SHELL_ALLOWLIST    comma-separated programs the shell action may run
  NEURO_DENY_ACTIONS       comma-separated actions that never run
  NEURO_AUDIT_LOG          JSON-lines file with every action decision
  NEURO_PERMISSIONS_FILE   path to the policy file

Safety
------
Nothing runs unless the permission policy allows it: hard denials, the deny
list, per-scope budgets (input/filesystem/process/network/system/vision/game/
shell), a rate limit, the shell firewall and the kill switch are all evaluated
for every action, and the decision is written to the audit log. The dashboard
can pause everything or flip the kill switch at any time.
