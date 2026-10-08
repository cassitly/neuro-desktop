package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	neuro "github.com/cassitly/neuro-integration-sdk"
)

// testIntegration builds an integration with a real policy file and game
// registry, matching how the bridge is wired in production.
func testIntegration(t *testing.T) *NDIntegration {
	t.Helper()

	permissionsPath := filepath.Join(t.TempDir(), "permissions.json")
	if err := os.WriteFile(permissionsPath, []byte(allowEverythingPolicy), 0644); err != nil {
		t.Fatalf("failed to write policy: %v", err)
	}

	profilesDir := t.TempDir()
	if err := writeProfileFixtures(profilesDir); err != nil {
		t.Fatalf("failed to write profiles: %v", err)
	}
	t.Setenv("NEURO_GAME_PROFILES_DIR", profilesDir)
	t.Setenv("NEURO_GAME_PROFILES_FILE", "")

	policy, err := loadPermissionPolicy(permissionsPath)
	if err != nil {
		t.Fatalf("failed to load policy: %v", err)
	}

	return &NDIntegration{
		ipcFilePath:     filepath.Join(t.TempDir(), "ipc.json"),
		permissionsPath: permissionsPath,
		permissions:     policy,
		games:           newGameRuntime(loadGameRegistry()),
		relay:           newRelayState(RelayConfig{}),
		stats:           newBridgeStats(),
	}
}

func newTestAdmin(t *testing.T, integration *NDIntegration, token string) *httptest.Server {
	t.Helper()
	admin := NewAdminServer(integration, "127.0.0.1:0", token)
	server := httptest.NewServer(admin.routes())
	t.Cleanup(server.Close)
	return server
}

func getJSON(t *testing.T, url string, headers map[string]string) (int, map[string]interface{}) {
	t.Helper()

	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer response.Body.Close()

	var payload map[string]interface{}
	_ = json.NewDecoder(response.Body).Decode(&payload)
	return response.StatusCode, payload
}

func sendJSON(t *testing.T, method string, url string, body string, headers map[string]string) (int, map[string]interface{}) {
	t.Helper()

	request, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer response.Body.Close()

	var payload map[string]interface{}
	_ = json.NewDecoder(response.Body).Decode(&payload)
	return response.StatusCode, payload
}

func TestAdminStatusReportsBridgeState(t *testing.T) {
	integration := testIntegration(t)
	server := newTestAdmin(t, integration, "")

	status, payload := getJSON(t, server.URL+"/api/status", nil)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if ok, _ := payload["ok"].(bool); !ok {
		t.Fatalf("expected ok, got %v", payload)
	}
	if version, _ := payload["version"].(string); version != Version {
		t.Fatalf("expected version %s, got %v", Version, payload["version"])
	}

	executor, _ := payload["executor"].(map[string]interface{})
	if connected, _ := executor["connected"].(bool); connected {
		t.Fatal("no executor is connected in this test")
	}

	games, _ := payload["game"].(map[string]interface{})
	if profiles, _ := games["profiles"].(float64); profiles < 1 {
		t.Fatalf("expected the game registry to be reported, got %v", games)
	}
}

func TestAdminPermissionSchemaCoversGameScope(t *testing.T) {
	integration := testIntegration(t)
	server := newTestAdmin(t, integration, "")

	status, payload := getJSON(t, server.URL+"/api/permissions/schema", nil)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}

	scopes, _ := payload["scopes"].([]interface{})
	foundGame := false
	for _, raw := range scopes {
		if scope, ok := raw.(map[string]interface{}); ok {
			if scope["name"] == "game" {
				foundGame = true
			}
		}
	}
	if !foundGame {
		t.Fatal("the game scope should be offered to the dashboard")
	}

	actions, _ := payload["actions"].([]interface{})
	found := false
	for _, raw := range actions {
		entry, _ := raw.(map[string]interface{})
		if entry["name"] == "game_move" && entry["scope"] == "game" {
			found = true
		}
	}
	if !found {
		t.Fatal("game_move should be listed with the game scope")
	}
}

func TestAdminPermissionUpdateAppliesLive(t *testing.T) {
	integration := testIntegration(t)
	server := newTestAdmin(t, integration, "")

	policy := `{
	  "version": "1.0.0",
	  "default_allow": false,
	  "denied_actions": ["type_text"],
	  "scopes": {"input": false, "game": true, "vision": true}
	}`

	status, payload := sendJSON(t, http.MethodPut, server.URL+"/api/permissions", policy, nil)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d (%v)", status, payload)
	}
	if applied, _ := payload["applied_live"].(bool); !applied {
		t.Fatalf("expected the policy to be applied live, got %v", payload)
	}

	// The running bridge must use the new policy immediately.
	if integration.policy().IsAllowed("type_text") {
		t.Fatal("type_text is denied by the new policy")
	}
	if !integration.policy().IsAllowed("game_move") {
		t.Fatal("the game scope is allowed by the new policy")
	}

	// And the file on disk is the normalised policy.
	data, err := os.ReadFile(integration.permissionsPath)
	if err != nil {
		t.Fatalf("policy file missing: %v", err)
	}
	var reloaded permissionPolicyFile
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatalf("saved policy is not valid JSON: %v", err)
	}
	if reloaded.DefaultAllow == nil || bool(*reloaded.DefaultAllow) {
		t.Fatalf("expected default_allow=false to be persisted, got %v", reloaded.DefaultAllow)
	}
}

func TestAdminRejectsBrokenPolicyWithoutBreakingTheBridge(t *testing.T) {
	integration := testIntegration(t)
	server := newTestAdmin(t, integration, "")

	before := integration.policy()

	status, _ := sendJSON(t, http.MethodPut, server.URL+"/api/permissions", `{"scopes": [`, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", status)
	}
	if integration.policy() != before {
		t.Fatal("a rejected policy must not change the running policy")
	}

	// A scope object of the wrong shape is also refused rather than silently
	// granting or dropping a capability.
	status, _ = sendJSON(t, http.MethodPut, server.URL+"/api/permissions",
		`{"scopes": {"game": {"allowed": "sometimes"}}}`, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for a malformed scope, got %d", status)
	}
}

func TestAdminTokenGuardsWrites(t *testing.T) {
	integration := testIntegration(t)
	server := newTestAdmin(t, integration, "letmein")

	policy := `{"version": "1.0.0", "default_allow": true}`

	// Reads are allowed for local operators.
	status, _ := getJSON(t, server.URL+"/api/status", nil)
	if status != http.StatusOK {
		t.Fatalf("expected status to be readable, got %d", status)
	}

	status, _ = sendJSON(t, http.MethodPut, server.URL+"/api/permissions", policy, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401 without a token, got %d", status)
	}

	status, _ = sendJSON(t, http.MethodPut, server.URL+"/api/permissions", policy,
		map[string]string{"Authorization": "Bearer letmein"})
	if status != http.StatusOK {
		t.Fatalf("expected the token to be accepted, got %d", status)
	}

	// X-ND-Token is the header the dashboard sends.
	status, _ = sendJSON(t, http.MethodPost, server.URL+"/api/extensions/some-extension/enable", "",
		map[string]string{"X-ND-Token": "letmein"})
	if status == http.StatusUnauthorized {
		t.Fatal("the X-ND-Token header should be accepted")
	}

	// A wrong token is rejected just like a missing one.
	status, _ = sendJSON(t, http.MethodPost, server.URL+"/api/extensions/some-extension/enable", "",
		map[string]string{"X-ND-Token": "nope"})
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401 for a wrong token, got %d", status)
	}
}

func TestAdminExtensionManagement(t *testing.T) {
	integration := testIntegration(t)

	statePath := filepath.Join(t.TempDir(), "extensions-state.json")
	state := `{"installed": {"nd-vision-server": {"enabled": false, "installed_at": "2026-01-01T00:00:00Z", "source": "test"}}}`
	if err := os.WriteFile(statePath, []byte(state), 0644); err != nil {
		t.Fatalf("failed to write extension state: %v", err)
	}
	t.Setenv("NEURO_EXTENSIONS_STATE_FILE", statePath)

	server := newTestAdmin(t, integration, "")

	status, payload := getJSON(t, server.URL+"/api/extensions", nil)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	installed, _ := payload["installed"].([]interface{})
	if len(installed) != 1 {
		t.Fatalf("expected one installed extension, got %v", payload["installed"])
	}

	status, payload = sendJSON(t, http.MethodPost, server.URL+"/api/extensions/nd-vision-server/enable", "", nil)
	if status != http.StatusOK {
		t.Fatalf("enable failed: %d %v", status, payload)
	}

	reloaded, err := loadExtensionState(statePath)
	if err != nil {
		t.Fatalf("failed to reload state: %v", err)
	}
	if !reloaded.Installed["nd-vision-server"].Enabled {
		t.Fatal("the extension should be enabled in the state file")
	}

	status, _ = sendJSON(t, http.MethodPost, server.URL+"/api/extensions/nd-vision-server/uninstall", "", nil)
	if status != http.StatusOK {
		t.Fatalf("uninstall failed: %d", status)
	}
	reloaded, _ = loadExtensionState(statePath)
	if _, stillThere := reloaded.Installed["nd-vision-server"]; stillThere {
		t.Fatal("the extension should be gone after uninstall")
	}

	// Unknown actions are refused, not silently ignored.
	status, _ = sendJSON(t, http.MethodPost, server.URL+"/api/extensions/nd-vision-server/frobnicate", "", nil)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for an unknown action, got %d", status)
	}
}

func TestAdminGamesPanel(t *testing.T) {
	integration := testIntegration(t)
	server := newTestAdmin(t, integration, "")

	status, payload := getJSON(t, server.URL+"/api/games", nil)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}

	profiles, _ := payload["profiles"].([]interface{})
	if len(profiles) != 2 {
		t.Fatalf("expected the two fixture profiles, got %v", payload["profiles"])
	}

	found := map[string]bool{}
	for _, raw := range profiles {
		entry, _ := raw.(map[string]interface{})
		name, _ := entry["id"].(string)
		found[name] = true
		if entry["keys"] == nil {
			t.Fatalf("profile %s should expose its keybinds to the UI", name)
		}
	}
	if !found["minecraft"] || !found["stardew-valley"] {
		t.Fatalf("expected minecraft and stardew-valley, got %v", found)
	}
}

func TestAdminServesDashboardAssets(t *testing.T) {
	integration := testIntegration(t)
	uiDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(uiDir, "index.html"), []byte("<html>dashboard</html>"), 0644); err != nil {
		t.Fatalf("failed to write index.html: %v", err)
	}
	t.Setenv("NEURO_UI_DIR", uiDir)

	server := newTestAdmin(t, integration, "")

	response, err := http.Get(server.URL + "/ui/")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer response.Body.Close()

	buffer := new(bytes.Buffer)
	_, _ = buffer.ReadFrom(response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.StatusCode)
	}
	if !strings.Contains(buffer.String(), "dashboard") {
		t.Fatalf("expected the dashboard shell, got %q", buffer.String())
	}

	// A deep link falls back to the shell (single page app routing).
	response, err = http.Get(server.URL + "/ui/games/minecraft")
	if err != nil {
		t.Fatalf("deep link request failed: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("deep links should serve the shell, got %d", response.StatusCode)
	}
}

func TestAdminReleaseWithoutExecutorReportsFailure(t *testing.T) {
	// With no executor attached the bridge waits for the file IPC timeout
	// before giving up; shrink it so the test stays fast.
	t.Setenv("NEURO_IPC_TIMEOUT_SECONDS", "1")

	integration := testIntegration(t)
	server := newTestAdmin(t, integration, "")

	status, payload := sendJSON(t, http.MethodPost, server.URL+"/api/games/release", "", nil)
	if status != http.StatusConflict {
		t.Fatalf("expected 409 when nothing is attached, got %d (%v)", status, payload)
	}
	if message, _ := payload["message"].(string); !strings.Contains(message, "release input") {
		t.Fatalf("expected a readable failure, got %v", payload)
	}
}

func TestAdminRelayStatusExposed(t *testing.T) {
	integration := testIntegration(t)
	integration.relay.notePeer("minecraft-integration", "integration")
	server := newTestAdmin(t, integration, "")

	status, payload := getJSON(t, server.URL+"/api/relay", nil)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	relay, _ := payload["relay"].(map[string]interface{})
	if peers, _ := relay["peer_count"].(float64); peers != 1 {
		t.Fatalf("expected one relay peer, got %v", relay)
	}
}

func TestWrapExecutionResult(t *testing.T) {
	wrapped := wrapExecutionResult(neuro.NewSuccessResult("done"))
	if !wrapped.OK || wrapped.Message != "done" {
		t.Fatalf("unexpected wrap: %+v", wrapped)
	}
	wrapped = wrapExecutionResult(neuro.NewFailureResult("nope"))
	if wrapped.OK {
		t.Fatalf("failures should not be reported as ok: %+v", wrapped)
	}
}
