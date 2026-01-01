package main

const (
	CmdMouseMove        CommandType = "move_mouse_to"
	CmdMouseClick       CommandType = "mouse_click"
	CmdKeyPress         CommandType = "key_press"
	CmdTypeText         CommandType = "type_text"
	CmdRunScript        CommandType = "run_script"
	CmdExecuteQueue     CommandType = "execute_queue"
	CmdClearActionQueue CommandType = "clear_action_queue"

	CmdShutdownGracefully  CommandType = "shutdown_gracefully"
	CmdShutdownImmediately CommandType = "shutdown_immediately"
)

var ActionSchemas = map[string]ActionDefinition{
	string(CmdMouseMove): {
		Name:        string(CmdMouseMove),
		Description: "Move the mouse cursor (in a human-like way), towards specific screen coordinates (relative to the top left corner of the screen)",
		Schema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"x": map[string]interface{}{
					"type":        "integer",
					"description": "X coordinate",
				},
				"y": map[string]interface{}{
					"type":        "integer",
					"description": "Y coordinate",
				},
				"execute_now": map[string]interface{}{
					"type":        "boolean",
					"description": "Execute immediately (true) or queue for macro (false). Default: true",
					"default":     true,
				},
				"clear_after": map[string]interface{}{
					"type":        "boolean",
					"description": "Clear action queue after execution (true) or keep for macro (false). Default: true",
					"default":     true,
				},
			},
			"required": []string{"x", "y"},
		},
	},
	string(CmdMouseClick): {
		Name:        string(CmdMouseClick),
		Description: "Click the mouse at current position or specific coordinates",
		Schema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"x": map[string]interface{}{
					"type":        "integer",
					"description": "X coordinate (optional)",
				},
				"y": map[string]interface{}{
					"type":        "integer",
					"description": "Y coordinate (optional)",
				},
				"button": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"left", "right", "middle"},
					"description": "Mouse button to click",
				},
				"execute_now": map[string]interface{}{
					"type":        "boolean",
					"description": "Execute immediately (true) or queue for macro (false). Default: true",
					"default":     true,
				},
				"clear_after": map[string]interface{}{
					"type":        "boolean",
					"description": "Clear action queue after execution (true) or keep for macro (false). Default: true",
					"default":     true,
				},
			},
			"required": []string{"button"},
		},
	},
	string(CmdTypeText): {
		Name:        string(CmdTypeText),
		Description: "Type text using the keyboard",
		Schema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"text": map[string]interface{}{
					"type":        "string",
					"description": "Text to type",
					"maxLength":   1000,
				},
				"execute_now": map[string]interface{}{
					"type":        "boolean",
					"description": "Execute immediately (true) or queue for macro (false). Default: true",
					"default":     true,
				},
				"clear_after": map[string]interface{}{
					"type":        "boolean",
					"description": "Clear action queue after execution (true) or keep for macro (false). Default: true",
					"default":     true,
				},
			},
			"required": []string{"text"},
		},
	},
	string(CmdKeyPress): {
		Name:        string(CmdKeyPress),
		Description: "Press a specific keyboard key or shortcut. Common keys: enter, escape, tab, space, backspace, shift, ctrl, alt",
		Schema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"key": map[string]interface{}{
					"type":        "string",
					"description": "Key to press",
				},
				"modifiers": map[string]interface{}{
					"type":        "array",
					"description": "Modifier keys (shift, ctrl, alt)",
					"items": map[string]interface{}{
						"type": "string",
					},
				},
				"execute_now": map[string]interface{}{
					"type":        "boolean",
					"description": "Execute immediately (true) or queue for macro (false). Default: true",
					"default":     true,
				},
				"clear_after": map[string]interface{}{
					"type":        "boolean",
					"description": "Clear action queue after execution (true) or keep for macro (false). Default: true",
					"default":     true,
				},
			},
			"required": []string{"key"},
		},
	},
	string(CmdClearActionQueue): {
		Name:        string(CmdClearActionQueue),
		Description: "Clear the action queue. The action queue persists across every action, you must clear it manually. Unless you are creating a macro.",
		Schema:      map[string]interface{}{},
	},
	string(CmdRunScript): {
		Name:        string(CmdRunScript),
		Description: "Execute a sequence of actions using a simple script language. Commands: TYPE \"text\", ENTER, MOVE x y, CLICK x y, WAIT seconds, PRESS key",
		Schema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"script": map[string]interface{}{
					"type":        "string",
					"description": "Script with multiple commands, one per line",
				},
				"execute_now": map[string]interface{}{
					"type":        "boolean",
					"description": "Execute immediately (true) or queue for macro (false). Default: true",
					"default":     true,
				},
				"clear_after": map[string]interface{}{
					"type":        "boolean",
					"description": "Clear action queue after execution (true) or keep for macro (false). Default: true",
					"default":     true,
				},
			},
			"required": []string{"script"},
		},
	},
	string(CmdExecuteQueue): {
		Name:        string(CmdExecuteQueue),
		Description: "Execute the action queue.",
		Schema:      map[string]interface{}{},
	},
}
