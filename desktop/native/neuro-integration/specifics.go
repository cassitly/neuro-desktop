package main

import "encoding/json"

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
