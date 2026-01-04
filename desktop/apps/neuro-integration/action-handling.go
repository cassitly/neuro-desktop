package main

import (
	"encoding/json"
	"fmt"
	"log"
)

var CurrentActionList = map[string]ActionDefinition{}

func (n *NeuroIntegration) registerActions() error {
	actions := []ActionDefinition{}

	if RegisterHLActionsOnStartup {
		for _, action := range HLActionSchemas {
			actions = append(actions, ActionDefinition{
				Name:        action.Name,
				Description: action.Description,
				Schema:      action.Schema,
			})
		}
	}

	if RegisterLLActionsOnStartup {
		for _, action := range LLActionSchemas {
			actions = append(actions, ActionDefinition{
				Name:        action.Name,
				Description: action.Description,
				Schema:      action.Schema,
			})
		}
	}

	for _, action := range actions {
		CurrentActionList[action.Name] = action
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
		"action_names": []string{},
	}

	for _, action := range CurrentActionList {
		data["action_names"] = append(data["action_names"].([]string), action.Name)
	}

	CurrentActionList = map[string]ActionDefinition{}

	dataBytes, _ := json.Marshal(data)

	return n.sendMessage(NeuroMessage{
		Command: "actions/unregister",
		Data:    dataBytes,
	})
}

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

	// These two switch between registering and unregistering the actions
	case string(EnableLLControls):
		RegisterLLActionsOnStartup = true
		RegisterHLActionsOnStartup = false
		n.unregisterActions()
		n.sendActionResult(action.ID, true, "Unregistering High Level actions and registering Low Level actions")
		n.registerActions()
		return

	case string(DisableLLControls):
		RegisterLLActionsOnStartup = false
		RegisterHLActionsOnStartup = true
		n.unregisterActions()
		n.sendActionResult(action.ID, true, "Unregistering Low Level actions and registering High Level actions")
		n.registerActions()
		return

	case string(CmdMouseMove):
		x, _ := params["x"].(float64)
		y, _ := params["y"].(float64)
		cmd = IPCCommand{
			Type: CmdMouseMove,
			Params: map[string]interface{}{
				"x": int(x),
				"y": int(y),
			},
		}

	case string(CmdMouseClick):
		button, _ := params["button"].(string)
		clickParams := map[string]interface{}{
			"button": button,
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
				"text": text,
			},
		}

	case string(CmdKeyPress):
		key, _ := params["key"].(string)
		keyParams := map[string]interface{}{
			"key": key,
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
				"script": script,
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

	cmd = IPCCommand{
		Type:       cmd.Type,
		Params:     cmd.Params,
		ExecuteNow: executeNow,
		ClearAfter: clearAfter,
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
