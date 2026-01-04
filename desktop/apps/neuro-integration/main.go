// desktop/native/go-neuro-integration/main.go
package main

import (
	"encoding/json"
	"log"
	"os"
)

// IPC Command to Rust binary
type IPCCommand struct {
	Type       CommandType            `json:"type"`
	Params     map[string]interface{} `json:"params"`
	ExecuteNow bool                   `json:"execute_now"`
	ClearAfter bool                   `json:"clear_after"`
}

// IPC Response from Rust binary
type IPCResponse struct {
	Success bool                   `json:"success"`
	Data    map[string]interface{} `json:"data,omitempty"`
	Error   string                 `json:"error,omitempty"`
}

func (n *NeuroIntegration) listen() {
	for {
		var msg NeuroMessage
		err := n.ws.ReadJSON(&msg)
		if err != nil {
			log.Printf("WebSocket read error: %v", err)
			return
		}

		switch msg.Command {
		case "action":
			var action IncomingAction
			if err := json.Unmarshal(msg.Data, &action); err != nil {
				log.Printf("Failed to parse action: %v", err)
				continue
			}
			go n.handleAction(action)

		case "actions/reregister_all":
			log.Println("Reregistering actions...")
			n.registerActions()

		case "shutdown/graceful":
			var shutdownReq struct {
				WantsShutdown bool `json:"wants_shutdown"`
			}
			if err := json.Unmarshal(msg.Data, &shutdownReq); err != nil {
				log.Printf("Failed to parse shutdown request: %v", err)
				continue
			}
			if shutdownReq.WantsShutdown {
				log.Println("Graceful shutdown requested, preparing...")

				// Tell Rust to save state
				resp, err := n.sendToRust(IPCCommand{
					Type: CmdShutdownGracefully,
				})

				if err != nil || !resp.Success {
					log.Printf("Warning: Rust shutdown failed: %v", err)
				}

				// Send ready signal
				n.sendShutdownReady()
				n.Close()
				return
			}

		case "shutdown/immediate":
			log.Println("Immediate shutdown requested!")

			// Tell Rust to save what it can
			n.sendToRust(IPCCommand{
				Type: CmdShutdownImmediately,
			})

			// Send ready signal
			n.sendShutdownReady()
			n.Close()
			return

		default:
			log.Printf("Unknown command: %s", msg.Command)
		}
	}
}

func (n *NeuroIntegration) Close() error {
	// Tell neuro the integration is shutting down. So that she has some sense, of what happened.
	n.sendContext("Neuro Desktop integration is shutting down. Websocket will close.", true)

	// Unregister actions, to properly clear
	// the action list for the next integration
	n.unregisterActions()
	return n.ws.Close()
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Configuration from environment
	wsURL := os.Getenv("NEURO_SDK_WS_URL")
	if wsURL == "" {
		wsURL = "ws://localhost:8000"
	}

	ipcPath := os.Getenv("NEURO_IPC_FILE")
	if ipcPath == "" {
		ipcPath = "./neuro-integration-code-ipc.json"
	}

	// Create integration
	log.Printf("Connecting to Neuro at %s...", wsURL)
	integration, err := NewNeuroIntegration(wsURL, "Neuro Desktop", ipcPath)
	if err != nil {
		log.Fatalf("Failed to create integration: %v", err)
	}
	defer integration.Close()

	log.Println("Connected to Neuro!")

	// Send startup
	if err := integration.startup(); err != nil {
		log.Fatalf("Failed to send startup: %v", err)
	}

	// Send initial context
	integration.sendContext("Neuro Desktop is ready. You can control the mouse, keyboard, and run scripts.", true)
	content, err := os.ReadFile("./integration-docs/Action Script Documentation.md")
	if err != nil {
		log.Fatalf("Failed to read documentation file: %v", err)
	}
	integration.sendContext(string(content), true)

	// Register actions
	log.Println("Registering actions...")
	if err := integration.registerActions(); err != nil {
		log.Fatalf("Failed to register actions: %v", err)
	}

	log.Println("Neuro Desktop Go integration running!")
	log.Println("Listening for actions from Neuro...")

	// Listen for messages
	integration.listen()
}
