package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	neuro "github.com/cassitly/neuro-integration-sdk"
)

// reservedActionNames lists action names owned by another integration (for
// example a dedicated Minecraft integration running beside Neuro Desktop
// through the relay). Neuro Desktop must not register those, or the two
// integrations would shadow each other.
func (n *NDIntegration) reservedActionNames() map[string]bool {
	reserved := map[string]bool{}
	for _, name := range n.reservedActionList() {
		reserved[strings.ToLower(name)] = true
	}
	return reserved
}

// registeredActionNames returns the action names currently registered with
// Neuro (empty when Neuro has not connected yet).
func (n *NDIntegration) registeredActionNames() []string {
	currentActionListMu.Lock()
	out := make([]string, 0, len(currentActionList))
	for name := range currentActionList {
		out = append(out, name)
	}
	currentActionListMu.Unlock()
	sort.Strings(out)
	return out
}

func (n *NDIntegration) reservedActionList() []string {
	if n.relay == nil {
		return nil
	}
	return n.relay.ReservedActionNames()
}

func specAllowed(spec actionSpec, reserved map[string]bool) bool {
	return !reserved[strings.ToLower(string(spec.Name))]
}

func (n *NDIntegration) registerActions() error {
	handlers := make([]neuro.ActionHandler, 0)
	reserved := n.reservedActionNames()
	skipped := make([]string, 0)

	actionModeMu.Lock()
	registerHL := RegisterHLActionsOnStartup
	registerLL := RegisterLLActionsOnStartup
	registerGame := RegisterGameActionsOnStartup
	actionModeMu.Unlock()

	var specs []actionSpec
	if registerHL {
		specs = append(specs, HLActionSpecs...)
	}
	if registerLL {
		specs = append(specs, LLActionSpecs...)
	}
	// The game interface stays registered in both high- and low-level modes:
	// it is how Neuro plays a game that has no integration of its own.
	if registerGame {
		specs = append(specs, gameActionSpecs()...)
	}
	// Same for the shell: on a headless machine it is the main capability.
	specs = append(specs, ShellActionSpecs...)
	// The guide is always registered: it is the cheapest way to make a small
	// model pick the right action.
	specs = append(specs, guideActionSpecs()...)

	for _, spec := range specs {
		if !specAllowed(spec, reserved) {
			skipped = append(skipped, string(spec.Name))
			continue
		}
		handlers = append(handlers, &IPCProxyAction{
			integration: n,
			spec:        spec,
		})
	}

	if len(skipped) > 0 {
		log.Printf("Left %d action(s) to other integrations: %s", len(skipped), strings.Join(skipped, ", "))
	}

	currentActionListMu.Lock()
	currentActionList = map[string]neuro.ActionHandler{}
	for _, handler := range handlers {
		currentActionList[handler.GetName()] = handler
	}
	currentActionListMu.Unlock()

	log.Printf("Registering %d action(s)", len(handlers))
	return n.client.RegisterActions(handlers)
}

func (n *NDIntegration) unregisterActions() error {
	currentActionListMu.Lock()
	names := make([]string, 0, len(currentActionList))
	for actionName := range currentActionList {
		names = append(names, actionName)
	}
	currentActionList = map[string]neuro.ActionHandler{}
	currentActionListMu.Unlock()

	if len(names) == 0 {
		return nil
	}

	return n.client.UnregisterActions(names)
}

func (n *NDIntegration) switchActionMode(enableHL bool, enableLL bool) error {
	actionModeMu.Lock()
	RegisterHLActionsOnStartup = enableHL
	RegisterLLActionsOnStartup = enableLL
	actionModeMu.Unlock()

	if err := n.unregisterActions(); err != nil {
		return fmt.Errorf("failed to unregister actions: %w", err)
	}

	if err := n.registerActions(); err != nil {
		return fmt.Errorf("failed to register actions: %w", err)
	}

	return nil
}

func (n *NDIntegration) executeScriptIntent(script string) neuro.ExecutionResult {
	cmd := IPCCommand{
		Type: CmdRunScript,
		Params: map[string]interface{}{
			"script": script,
		},
		ExecuteNow: true,
		ClearAfter: true,
	}

	resp, err := n.sendToRust(cmd)
	if err != nil {
		return neuro.NewFailureResult(fmt.Sprintf("IPC error: %v", err))
	}

	if !resp.Success {
		message := resp.Error
		if message == "" {
			message = "Intent execution failed"
		}
		return neuro.NewFailureResult(message)
	}

	return neuro.NewSuccessResult("ok")
}

func (n *NDIntegration) handleGracefulShutdown(data json.RawMessage) {
	var shutdownReq struct {
		WantsShutdown bool `json:"wants_shutdown"`
	}

	if err := json.Unmarshal(data, &shutdownReq); err != nil {
		log.Printf("Failed to parse graceful shutdown request: %v", err)
		return
	}

	if !shutdownReq.WantsShutdown {
		return
	}

	log.Println("Graceful shutdown requested")

	resp, err := n.sendToRust(IPCCommand{
		Type: CmdShutdownGracefully,
	})
	if err != nil || !resp.Success {
		log.Printf("Warning: Rust graceful shutdown failed: %v", err)
	}

	if err := n.client.SendShutdownReady(); err != nil {
		log.Printf("Failed to send shutdown/ready: %v", err)
	}

	n.markDone()
}

func (n *NDIntegration) handleImmediateShutdown(_ json.RawMessage) {
	log.Println("Immediate shutdown requested")

	_, _ = n.sendToRust(IPCCommand{
		Type: CmdShutdownImmediately,
	})

	if err := n.client.SendShutdownReady(); err != nil {
		log.Printf("Failed to send shutdown/ready: %v", err)
	}

	n.markDone()
}

type CatalogIndex struct {
	Items []CatalogItem `json:"items"`
}

type CatalogItem struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Type        string   `json:"type,omitempty"`
	Repository  string   `json:"repository,omitempty"`
	Homepage    string   `json:"homepage,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	LaunchHints []string `json:"launch_hints,omitempty"`
}

func catalogFilePath() string {
	catalogPath := os.Getenv("NEURO_CATALOG_FILE")
	if catalogPath == "" {
		catalogPath = "./catalog/index.json"
	}
	return catalogPath
}

func loadCatalogIndex(catalogPath string) (CatalogIndex, error) {
	data, err := os.ReadFile(catalogPath)
	if err != nil {
		return CatalogIndex{}, fmt.Errorf("catalog file not found: %s", catalogPath)
	}

	var index CatalogIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return CatalogIndex{}, fmt.Errorf("catalog file is invalid JSON")
	}

	return index, nil
}

func (n *NDIntegration) listCatalogItems() neuro.ExecutionResult {
	index, err := loadCatalogIndex(catalogFilePath())
	if err != nil {
		return neuro.NewFailureResult(err.Error())
	}

	if len(index.Items) == 0 {
		return neuro.NewSuccessResult("Catalog is empty")
	}

	lines := make([]string, 0, len(index.Items))
	for _, item := range index.Items {
		lines = append(lines, fmt.Sprintf("%s: %s", item.Name, item.Description))
	}

	return neuro.NewSuccessResult("Catalog items -> " + strings.Join(lines, " | "))
}

func (n *NDIntegration) findCatalogItems(query string, limit int) neuro.ExecutionResult {
	index, err := loadCatalogIndex(catalogFilePath())
	if err != nil {
		return neuro.NewFailureResult(err.Error())
	}

	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}

	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" {
		return neuro.NewFailureResult("query is required")
	}

	matches := make([]CatalogItem, 0, limit)
	for _, item := range index.Items {
		if catalogItemMatches(item, needle) {
			matches = append(matches, item)
			if len(matches) >= limit {
				break
			}
		}
	}

	if len(matches) == 0 {
		return neuro.NewSuccessResult(fmt.Sprintf("No catalog items found for query: %s", query))
	}

	lines := make([]string, 0, len(matches))
	for _, item := range matches {
		lines = append(lines, fmt.Sprintf("%s (%s): %s", item.ID, item.Name, item.Description))
	}

	return neuro.NewSuccessResult("Catalog search -> " + strings.Join(lines, " | "))
}

func (n *NDIntegration) getCatalogItem(itemID string) neuro.ExecutionResult {
	index, err := loadCatalogIndex(catalogFilePath())
	if err != nil {
		return neuro.NewFailureResult(err.Error())
	}

	target := strings.ToLower(strings.TrimSpace(itemID))
	if target == "" {
		return neuro.NewFailureResult("item_id is required")
	}

	for _, item := range index.Items {
		if strings.ToLower(strings.TrimSpace(item.ID)) != target {
			continue
		}

		parts := []string{
			fmt.Sprintf("id=%s", item.ID),
			fmt.Sprintf("name=%s", item.Name),
			fmt.Sprintf("description=%s", item.Description),
		}
		if item.Type != "" {
			parts = append(parts, fmt.Sprintf("type=%s", item.Type))
		}
		if item.Repository != "" {
			parts = append(parts, fmt.Sprintf("repository=%s", item.Repository))
		}
		if item.Homepage != "" {
			parts = append(parts, fmt.Sprintf("homepage=%s", item.Homepage))
		}
		if len(item.Tags) > 0 {
			parts = append(parts, fmt.Sprintf("tags=%s", strings.Join(item.Tags, ",")))
		}
		if len(item.LaunchHints) > 0 {
			parts = append(parts, fmt.Sprintf("launch_hints=%s", strings.Join(item.LaunchHints, ",")))
		}

		return neuro.NewSuccessResult("Catalog item -> " + strings.Join(parts, " ; "))
	}

	return neuro.NewFailureResult(fmt.Sprintf("Catalog item not found: %s", itemID))
}

func catalogItemMatches(item CatalogItem, needle string) bool {
	if strings.Contains(strings.ToLower(item.ID), needle) {
		return true
	}
	if strings.Contains(strings.ToLower(item.Name), needle) {
		return true
	}
	if strings.Contains(strings.ToLower(item.Description), needle) {
		return true
	}

	for _, tag := range item.Tags {
		if strings.Contains(strings.ToLower(tag), needle) {
			return true
		}
	}

	return false
}
