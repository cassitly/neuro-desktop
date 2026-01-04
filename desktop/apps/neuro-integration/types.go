package main

import (
	"encoding/json"

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

type NeuroIntegration struct {
	ws       *websocket.Conn
	gameName string

	// Neuro Desktop Specific
	ipcFilePath string
}

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
