package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

func (n *NDIntegration) markDone() {
	n.doneOnce.Do(func() {
		close(n.done)
	})
}

func (n *NDIntegration) sendToRust(cmd IPCCommand) (*IPCResponse, error) {
	n.ipcMu.Lock()
	defer n.ipcMu.Unlock()

	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(n.ipcFilePath, cmdBytes, 0644); err != nil {
		return nil, err
	}

	responseFile := n.ipcFilePath + ".response"
	for i := 0; i < 500; i++ {
		data, err := os.ReadFile(responseFile)
		if err == nil {
			var resp IPCResponse
			if err := json.Unmarshal(data, &resp); err == nil {
				_ = os.Remove(responseFile)
				return &resp, nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	return nil, fmt.Errorf("timeout waiting for Rust response")
}
