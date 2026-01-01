package main

import (
	"fmt"

	"github.com/gorilla/websocket"
)

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
