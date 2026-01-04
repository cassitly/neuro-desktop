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
