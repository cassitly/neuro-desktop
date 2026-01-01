package main

import "encoding/json"

func (n *NeuroIntegration) registerActions() error {
	actions := []ActionDefinition{
		// THESE 3 ACTIONS ARE BROKEN ASF, DON'T UNCOMMENT
		ActionSchemas[string(CmdMouseMove)],
		ActionSchemas[string(CmdMouseClick)],
		ActionSchemas[string(CmdTypeText)],
		ActionSchemas[string(CmdKeyPress)],
		ActionSchemas[string(CmdClearActionQueue)],
		ActionSchemas[string(CmdRunScript)],
		ActionSchemas[string(CmdExecuteQueue)],
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
			string(CmdMouseMove),
			string(CmdMouseClick),
			string(CmdTypeText),
			string(CmdKeyPress),
			string(CmdClearActionQueue),
			string(CmdRunScript),
			string(CmdExecuteQueue),
		},
	}
	dataBytes, _ := json.Marshal(data)

	return n.sendMessage(NeuroMessage{
		Command: "actions/unregister",
		Data:    dataBytes,
	})
}
