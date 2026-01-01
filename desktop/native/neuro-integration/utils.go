package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

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

func (n *NeuroIntegration) sendShutdownReady() error {
	return n.sendMessage(NeuroMessage{
		Command: "shutdown/ready",
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
	for i := 0; i < 500; i++ {
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
