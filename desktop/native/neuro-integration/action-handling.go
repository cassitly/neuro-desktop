package main

import (
	"encoding/json"
	"fmt"
	"log"
)

func (n *NeuroIntegration) handleAction(action IncomingAction) {
	log.Printf("Handling action: %s (ID: %s)", action.Name, action.ID)

	var cmd IPCCommand
	var params map[string]interface{}

	// Parse action data
	if len(action.Data) > 0 {
		// Step 1: extract string
		var raw string
		if err := json.Unmarshal(action.Data, &raw); err == nil {
			// Step 2: parse JSON inside string
			if err := json.Unmarshal([]byte(raw), &params); err != nil {
				log.Printf("Failed to parse action data (inner JSON): %v", err)
				n.sendActionResult(action.ID, false, "Invalid action parameters")
				return
			}
		} else {
			// If it's not a string, try direct object (future-proof)
			if err := json.Unmarshal(action.Data, &params); err != nil {
				log.Printf("Failed to parse action data: %v", err)
				n.sendActionResult(action.ID, false, "Invalid action parameters")
				return
			}
		}
	}

	// Extract execute_now and clear_after (default to true)
	executeNow := true
	clearAfter := true

	if val, ok := params["execute_now"].(bool); ok {
		executeNow = val
	}
	if val, ok := params["clear_after"].(bool); ok {
		clearAfter = val
	}

	// Build IPC command based on action
	switch action.Name {
	case string(CmdMouseMove):
		x, _ := params["x"].(float64)
		y, _ := params["y"].(float64)
		cmd = IPCCommand{
			Type: CmdMouseMove,
			Params: map[string]interface{}{
				"x":           int(x),
				"y":           int(y),
				"execute_now": executeNow,
				"clear_after": clearAfter,
			},
		}

	case string(CmdMouseClick):
		button, _ := params["button"].(string)
		clickParams := map[string]interface{}{
			"button":      button,
			"execute_now": executeNow,
			"clear_after": clearAfter,
		}
		// Add coordinates if provided
		if x, ok := params["x"].(float64); ok {
			cmd.Params["x"] = int(x)
		}
		if y, ok := params["y"].(float64); ok {
			cmd.Params["y"] = int(y)
		}

		cmd = IPCCommand{
			Type:   CmdMouseClick,
			Params: clickParams,
		}

	case string(CmdTypeText):
		text, _ := params["text"].(string)
		cmd = IPCCommand{
			Type: CmdTypeText,
			Params: map[string]interface{}{
				"text":        text,
				"execute_now": executeNow,
				"clear_after": clearAfter,
			},
		}

	case string(CmdKeyPress):
		key, _ := params["key"].(string)
		keyParams := map[string]interface{}{
			"key":         key,
			"execute_now": executeNow,
			"clear_after": clearAfter,
		}

		if modifiers, ok := params["modifiers"].([]interface{}); ok {
			modStrs := make([]string, len(modifiers))
			for i, m := range modifiers {
				modStrs[i] = m.(string)
			}
			keyParams["modifiers"] = modStrs
		}

		cmd = IPCCommand{
			Type:   CmdKeyPress,
			Params: keyParams,
		}

	case string(CmdRunScript):
		script, _ := params["script"].(string)
		cmd = IPCCommand{
			Type: CmdRunScript,
			Params: map[string]interface{}{
				"script":      script,
				"execute_now": executeNow,
				"clear_after": clearAfter,
			},
		}

	case string(CmdExecuteQueue):
		cmd = IPCCommand{
			Type: CmdExecuteQueue,
		}

	case string(CmdClearActionQueue):
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
