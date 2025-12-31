// desktop/native/go-neuro-integration/main.go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

// Neuro API Message Types
type NeuroMessage struct {
	Command string          `json:"command"`
	Game    string          `json:"game,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type ActionDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Schema      map[string]interface{} `json:"schema,omitempty"`
}

type IncomingAction struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Data json.RawMessage `json:"data,omitempty"`
}

// Command types that match your Rust implementation
type CommandType string

const (
	CmdMouseMove        CommandType = "mouse_move"
	CmdMouseClick       CommandType = "mouse_click"
	CmdKeyPress         CommandType = "key_press"
	CmdKeyType          CommandType = "key_type"
	CmdRunScript        CommandType = "run_script"
	CmdClearActionQueue CommandType = "clear_action_queue"

	CmdShutdownGracefully  CommandType = "shutdown/gracefully"
	CmdShutdownImmediately CommandType = "shutdown/immediately"
)

// IPC Command to Rust binary
type IPCCommand struct {
	Type   CommandType            `json:"type"`
	Params map[string]interface{} `json:"params"`
}

// IPC Response from Rust binary
type IPCResponse struct {
	Success bool                   `json:"success"`
	Data    map[string]interface{} `json:"data,omitempty"`
	Error   string                 `json:"error,omitempty"`
}

type NeuroIntegration struct {
	ws          *websocket.Conn
	gameName    string
	ipcFilePath string
}

func NewNeuroIntegration(wsURL, gameName, ipcPath string) (*NeuroIntegration, error) {
	// Connect to Neuro WebSocket
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Neuro: %w", err)
	}

	integration := &NeuroIntegration{
		ws:          ws,
		gameName:    gameName,
		ipcFilePath: ipcPath,
	}

	return integration, nil
}

func (n *NeuroIntegration) sendMessage(msg NeuroMessage) error {
	msg.Game = n.gameName
	return n.ws.WriteJSON(msg)
}

func (n *NeuroIntegration) startup() error {
	return n.sendMessage(NeuroMessage{
		Command: "startup",
	})
}

func (n *NeuroIntegration) sendContext(message string, silent bool) error {
	data := map[string]interface{}{
		"message": message,
		"silent":  silent,
	}
	dataBytes, _ := json.Marshal(data)

	return n.sendMessage(NeuroMessage{
		Command: "context",
		Data:    dataBytes,
	})
}

func (n *NeuroIntegration) registerActions() error {
	actions := []ActionDefinition{
		{
			Name:        "move_mouse",
			Description: "Move the mouse cursor to specific screen coordinates",
			Schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"x": map[string]interface{}{
						"type":        "integer",
						"description": "X coordinate",
					},
					"y": map[string]interface{}{
						"type":        "integer",
						"description": "Y coordinate",
					},
				},
				"required": []string{"x", "y"},
			},
		},
		{
			Name:        "click_mouse",
			Description: "Click the mouse at current position or specific coordinates",
			Schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"x": map[string]interface{}{
						"type":        "integer",
						"description": "X coordinate (optional)",
					},
					"y": map[string]interface{}{
						"type":        "integer",
						"description": "Y coordinate (optional)",
					},
					"button": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"left", "right", "middle"},
						"description": "Mouse button to click",
					},
				},
				"required": []string{},
			},
		},
		{
			Name:        "type_text",
			Description: "Type text using the keyboard",
			Schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"text": map[string]interface{}{
						"type":        "string",
						"description": "Text to type",
						"maxLength":   1000,
					},
				},
				"required": []string{"text"},
			},
		},
		{
			Name:        "press_key",
			Description: "Press a specific keyboard key or shortcut. Common keys: enter, escape, tab, space, backspace, shift, ctrl, alt",
			Schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"key": map[string]interface{}{
						"type":        "string",
						"description": "Key to press",
					},
					"modifiers": map[string]interface{}{
						"type":        "array",
						"description": "Modifier keys (shift, ctrl, alt)",
						"items": map[string]interface{}{
							"type": "string",
						},
					},
				},
				"required": []string{"key"},
			},
		},
		{
			Name:        "clear_action_queue",
			Description: "Clear the action queue. The action queue persists across every action, you must clear it manually. Unless you are creating a macro.",
			Schema:      map[string]interface{}{},
		},
		{
			Name:        "run_script",
			Description: "Execute a sequence of actions using a simple script language. Commands: TYPE \"text\", ENTER, MOVE x y, CLICK x y, WAIT seconds, PRESS key",
			Schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"script": map[string]interface{}{
						"type":        "string",
						"description": "Script with multiple commands, one per line",
					},
				},
				"required": []string{"script"},
			},
		},
	}

	data := map[string]interface{}{
		"actions": actions,
	}
	dataBytes, _ := json.Marshal(data)

	return n.sendMessage(NeuroMessage{
		Command: "actions/register",
		Data:    dataBytes,
	})
}

func (n *NeuroIntegration) unregisterActions() error {
	data := map[string]interface{}{
		"actions": []string{
			"move_mouse",
			"click_mouse",
			"type_text",
			"press_key",
			"clear_action_queue",
			"run_script",
		},
	}
	dataBytes, _ := json.Marshal(data)

	return n.sendMessage(NeuroMessage{
		Command: "actions/unregister",
		Data:    dataBytes,
	})
}

func (n *NeuroIntegration) sendActionResult(actionID string, success bool, message string) error {
	data := map[string]interface{}{
		"id":      actionID,
		"success": success,
		"message": message,
	}
	dataBytes, _ := json.Marshal(data)

	return n.sendMessage(NeuroMessage{
		Command: "action/result",
		Data:    dataBytes,
	})
}

// Send command to Rust binary via IPC file
func (n *NeuroIntegration) sendToRust(cmd IPCCommand) (*IPCResponse, error) {
	// Write command to IPC file
	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return nil, err
	}

	err = os.WriteFile(n.ipcFilePath, cmdBytes, 0644)
	if err != nil {
		return nil, err
	}

	// Wait for response (polling)
	responseFile := n.ipcFilePath + ".response"
	for i := 0; i < 100; i++ {
		if data, err := os.ReadFile(responseFile); err == nil {
			var resp IPCResponse
			if err := json.Unmarshal(data, &resp); err == nil {
				os.Remove(responseFile) // Clean up
				return &resp, nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	return nil, fmt.Errorf("timeout waiting for Rust response")
}

func (n *NeuroIntegration) handleAction(action IncomingAction) {
	log.Printf("Handling action: %s (ID: %s)", action.Name, action.ID)

	var cmd IPCCommand
	var params map[string]interface{}

	// Parse action data
	if len(action.Data) > 0 {
		json.Unmarshal(action.Data, &params)
	}

	// Build IPC command based on action
	switch action.Name {
	case "move_mouse":
		x, _ := params["x"].(float64)
		y, _ := params["y"].(float64)
		cmd = IPCCommand{
			Type: CmdMouseMove,
			Params: map[string]interface{}{
				"x": int(x),
				"y": int(y),
			},
		}

	case "click_mouse":
		button, _ := params["button"].(string)
		if button == "" {
			button = "left"
		}
		cmd = IPCCommand{
			Type: CmdMouseClick,
			Params: map[string]interface{}{
				"button": button,
			},
		}
		// Add coordinates if provided
		if x, ok := params["x"].(float64); ok {
			cmd.Params["x"] = int(x)
		}
		if y, ok := params["y"].(float64); ok {
			cmd.Params["y"] = int(y)
		}

	case "type_text":
		text, _ := params["text"].(string)
		cmd = IPCCommand{
			Type: CmdKeyType,
			Params: map[string]interface{}{
				"text": text,
			},
		}

	case "press_key":
		key, _ := params["key"].(string)
		cmd = IPCCommand{
			Type: CmdKeyPress,
			Params: map[string]interface{}{
				"key": key,
			},
		}
		if modifiers, ok := params["modifiers"].([]interface{}); ok {
			modStrs := make([]string, len(modifiers))
			for i, m := range modifiers {
				modStrs[i] = m.(string)
			}
			cmd.Params["modifiers"] = modStrs
		}

	case "run_script":
		script, _ := params["script"].(string)
		cmd = IPCCommand{
			Type: CmdRunScript,
			Params: map[string]interface{}{
				"script": script,
			},
		}

	case "clear_action_queue":
		cmd = IPCCommand{
			Type: CmdClearActionQueue,
		}

	default:
		n.sendActionResult(action.ID, false, fmt.Sprintf("Unknown action: %s", action.Name))
		return
	}

	// Send to Rust
	resp, err := n.sendToRust(cmd)
	if err != nil {
		log.Printf("Error sending to Rust: %v", err)
		n.sendActionResult(action.ID, false, fmt.Sprintf("IPC error: %v", err))
		return
	}

	// Send result back to Neuro
	message := ""
	if resp.Error != "" {
		message = resp.Error
	}
	n.sendActionResult(action.ID, resp.Success, message)
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
				log.Println("Graceful shutdown was requested, closing Integration Code...")
				n.sendToRust(IPCCommand{ // Tell the main executable that we want to shut down gracefully
					Type: CmdShutdownGracefully,
				})
				n.Close()
				return
			}

		case "shutdown/immediate":
			log.Println("Immediate shutdown was requested, closing Integration Code...")
			n.sendToRust(IPCCommand{ // Tell the main executable that we want to shut down immediately
				Type: CmdShutdownImmediately,
			})
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
		ipcPath = "./neuro_ipc.json"
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
