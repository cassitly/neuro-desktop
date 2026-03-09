package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	neuro "github.com/cassitly/neuro-integration-sdk"
)

func NewNDIntegration(
	wsURL string,
	gameName string,
	ipcPath string,
	permissionsPath string,
) (*NDIntegration, error) {
	client, err := neuro.NewClient(neuro.ClientConfig{
		Game:         gameName,
		WebsocketURL: wsURL,
	})
	if err != nil {
		return nil, err
	}

	policy, err := loadPermissionPolicy(permissionsPath)
	if err != nil {
		return nil, err
	}

	integration := &NDIntegration{
		client:      client,
		ipcFilePath: ipcPath,
		permissions: policy,
		done:        make(chan struct{}),
	}

	integration.client.OnCommand("shutdown/graceful", integration.handleGracefulShutdown)
	integration.client.OnCommand("shutdown/immediate", integration.handleImmediateShutdown)

	return integration, nil
}

func (n *NDIntegration) Start() error {
	if err := n.client.Connect(); err != nil {
		return err
	}

	go func() {
		for err := range n.client.Errors() {
			log.Printf("SDK error: %v", err)
			n.markDone()
		}
	}()

	if err := n.client.SendContext(
		"Neuro Desktop is ready. You can control the mouse, keyboard, and run scripts.",
		true,
	); err != nil {
		log.Printf("Warning: failed to send initial context: %v", err)
	}

	content, contentPath, err := loadActionScriptDocumentation()
	if err != nil {
		log.Printf("Warning: failed to load action script documentation: %v", err)
	} else {
		log.Printf("Loaded action script documentation from %s", contentPath)
		if err := n.client.SendContext(content, true); err != nil {
			log.Printf("Warning: failed to send action script documentation: %v", err)
		}
	}

	return n.registerActions()
}

func (n *NDIntegration) Close() error {
	if err := n.client.SendContext(
		"Neuro Desktop integration is shutting down. WebSocket will close.",
		true,
	); err != nil {
		log.Printf("Warning: failed to send shutdown context: %v", err)
	}

	if err := n.unregisterActions(); err != nil {
		log.Printf("Warning: failed to unregister actions: %v", err)
	}

	n.markDone()
	return n.client.Close()
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	wsURL := os.Getenv("NEURO_SDK_WS_URL")
	if wsURL == "" {
		wsURL = "ws://localhost:8000"
	}

	ipcPath := os.Getenv("NEURO_IPC_FILE")
	if ipcPath == "" {
		ipcPath = "./neuro-integration-code-ipc.json"
	}

	permissionsPath := os.Getenv("NEURO_PERMISSIONS_FILE")
	if permissionsPath == "" {
		permissionsPath = "./permissions.json"
	}

	log.Printf("Starting Neuro Desktop integration")
	log.Printf("- WebSocket URL: %s", wsURL)
	log.Printf("- IPC file: %s", ipcPath)
	log.Printf("- Permissions file: %s", permissionsPath)

	integration, err := NewNDIntegration(wsURL, "Neuro Desktop", ipcPath, permissionsPath)
	if err != nil {
		log.Fatalf("Failed to create integration: %v", err)
	}

	if err := integration.Start(); err != nil {
		log.Fatalf("Failed to start integration: %v", err)
	}
	defer integration.Close()

	log.Println("Neuro Desktop integration running")
	log.Println("Press Ctrl+C to stop")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sigChan:
		log.Println("Interrupt signal received")
	case <-integration.done:
		log.Println("Shutdown requested by command channel")
	}
}
