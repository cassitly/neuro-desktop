package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	neuro "github.com/cassitly/neuro-integration-sdk"
)

// AdminServer is Vedal's operator surface: bridge status, live permission
// policy, the plugin (extension) manager, the game registry, and the relay
// link. It also serves the compiled dashboard, so the UI is same-origin with
// the API instead of being a file:// page that cannot call the bridge.
type AdminServer struct {
	integration *NDIntegration
	addr        string
	token       string

	mu       sync.Mutex
	requests int
	lastErr  string
}

func NewAdminServer(integration *NDIntegration, addr string, token string) *AdminServer {
	if addr == "" {
		addr = "127.0.0.1:8300"
	}
	return &AdminServer{integration: integration, addr: addr, token: token}
}

// routes wires the dashboard API. It is separate from Start so tests can serve
// the same handlers with httptest.
func (a *AdminServer) routes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", a.handleHealth)
	mux.HandleFunc("/api/status", a.handleStatus)
	mux.HandleFunc("/api/permissions", a.guard(a.handlePermissions))
	mux.HandleFunc("/api/permissions/schema", a.handlePermissionSchema)
	mux.HandleFunc("/api/actions", a.handleActions)
	mux.HandleFunc("/api/catalog", a.handleCatalog)
	mux.HandleFunc("/api/extensions", a.guard(a.handleExtensions))
	mux.HandleFunc("/api/extensions/", a.guard(a.handleExtensionAction))
	mux.HandleFunc("/api/games", a.guard(a.handleGames))
	mux.HandleFunc("/api/games/session", a.guard(a.handleGameSession))
	mux.HandleFunc("/api/games/release", a.guard(a.handleGameRelease))
	mux.HandleFunc("/api/games/observe", a.guard(a.handleGameObserve))
	mux.HandleFunc("/api/relay", a.guard(a.handleRelay))
	mux.HandleFunc("/api/config", a.handleConfig)
	mux.HandleFunc("/", a.handleUI)
	return mux
}

func (a *AdminServer) Start() error {
	listener, err := net.Listen("tcp", a.addr)
	if err != nil {
		return err
	}

	if !isLoopbackAddr(a.addr) && a.token == "" {
		log.Printf("WARNING: dashboard is listening on %s without NEURO_ADMIN_TOKEN; destructive API calls are refused until a token is set", a.addr)
	}

	go func() {
		server := &http.Server{
			Handler:           a.routes(),
			ReadHeaderTimeout: 10 * time.Second,
		}
		log.Printf("Operator dashboard on http://%s/", a.addr)
		if err := server.Serve(listener); err != nil {
			log.Printf("Admin server stopped: %v", err)
		}
	}()

	return nil
}

func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// guard enforces the admin token on non-read requests when one is configured,
// and always refuses destructive calls on a non-loopback bind without a token.
func (a *AdminServer) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		destructive := r.Method != http.MethodGet && r.Method != http.MethodHead

		if destructive {
			if a.token == "" {
				if !isLoopbackAddr(a.addr) {
					writeError(w, http.StatusForbidden,
						"dashboard is bound to a network address; set NEURO_ADMIN_TOKEN before changing settings")
					return
				}
			} else if !a.tokenMatches(r) {
				a.writeAudit("rejected: bad admin token", true)
				writeError(w, http.StatusUnauthorized, "invalid or missing admin token")
				return
			}
		}

		next(w, r)
	}
}

func (a *AdminServer) tokenMatches(r *http.Request) bool {
	provided := strings.TrimSpace(r.Header.Get("X-ND-Token"))
	if provided == "" {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(header), "bearer ") {
			provided = strings.TrimSpace(header[len("bearer "):])
		}
	}
	if provided == "" {
		provided = strings.TrimSpace(r.URL.Query().Get("token"))
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(a.token)) == 1
}

func (a *AdminServer) writeAudit(message string, isError bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.requests++
	if isError {
		a.lastErr = message
	}
}

func (a *AdminServer) audits() (int, string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.requests, a.lastErr
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": message})
}

func (a *AdminServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]interface{}{"ok": true, "version": Version})
}

func (a *AdminServer) handleStatus(w http.ResponseWriter, _ *http.Request) {
	integration := a.integration

	executor := ExecutorInfo{}
	if integration.executorHub != nil {
		executor = integration.executorHub.Info()
	}

	session := integration.games.Session()
	var sessionPayload interface{}
	if session != nil {
		sessionPayload = session
	}

	requests, lastErr := a.audits()

	status := map[string]interface{}{
		"ok":          true,
		"version":     Version,
		"integration": "Neuro Desktop",
		"uptime":      int(time.Since(integration.startedAt).Seconds()),
		"started_at":  integration.startedAt.UTC().Format(time.RFC3339),
		"executor":    executor,
		"negotiation": map[string]interface{}{
			"executor_connected": executor.Connected,
			"file_ipc_path":      integration.ipcFilePath,
			"protocol_version":   bridgeProtocolVersion,
		},
		"permissions_path": integration.permissionsPath,
		"game": map[string]interface{}{
			"registry_source": integration.games.RegistrySource(),
			"profiles":        len(integration.games.Profiles()),
			"session":         sessionPayload,
		},
		"relay":   integration.relay.status(),
		"actions": integration.stats.snapshot(),
		"admin": map[string]interface{}{
			"listen":         a.addr,
			"token_required": a.token != "",
			"requests":       requests,
			"last_error":     lastErr,
		},
	}

	writeJSON(w, status)
}

// handlePermissionSchema describes the policy model so the dashboard does not
// have to hard-code scopes and action names.
func (a *AdminServer) handlePermissionSchema(w http.ResponseWriter, _ *http.Request) {
	actions := make([]map[string]interface{}, 0)
	for _, spec := range allActionSpecs() {
		action := map[string]interface{}{
			"name":        string(spec.Name),
			"description": spec.Description,
			"scope":       actionScopeName(string(spec.Name)),
			"kind":        spec.Kind,
		}
		actions = append(actions, action)
	}
	sort.Slice(actions, func(i, j int) bool {
		return actions[i]["name"].(string) < actions[j]["name"].(string)
	})

	scopes := []map[string]interface{}{}
	for _, scope := range allScopes() {
		scopes = append(scopes, map[string]interface{}{
			"name":    string(scope),
			"default": defaultPermissionPolicy().scopes[scope].Allowed,
		})
	}

	policy := a.integration.policy()
	effective := map[string]bool{}
	for _, spec := range allActionSpecs() {
		effective[string(spec.Name)] = policy == nil || policy.IsAllowed(string(spec.Name))
	}

	writeJSON(w, map[string]interface{}{
		"scopes":    scopes,
		"actions":   actions,
		"effective": effective,
		"defaults": map[string]interface{}{
			"default_allow": policy != nil && policy.DefaultAllow,
		},
	})
}

func allActionSpecs() []actionSpec {
	specs := make([]actionSpec, 0, 64)
	specs = append(specs, HLActionSpecs...)
	specs = append(specs, LLActionSpecs...)
	specs = append(specs, gameActionSpecs()...)
	return specs
}

func allScopes() []PermissionScope {
	return []PermissionScope{
		ScopeInput, ScopeGame, ScopeFilesystem, ScopeProcess, ScopeNetwork, ScopeSystem, ScopeVision,
	}
}

func actionScopeName(action string) string {
	if scope, ok := actionScope[action]; ok {
		return string(scope)
	}
	return "unknown"
}

func (a *AdminServer) handleActions(w http.ResponseWriter, _ *http.Request) {
	policy := a.integration.policy()

	// The dashboard edits permissions before Neuro has connected, so list every
	// action this build can register and mark the live ones, instead of only
	// reporting what has already been sent to Neuro.
	registered := a.integration.registeredActionNames()
	isRegistered := make(map[string]bool, len(registered))
	for _, name := range registered {
		isRegistered[name] = true
	}

	reserved := a.integration.reservedActionList()
	isReserved := make(map[string]bool, len(reserved))
	for _, name := range reserved {
		isReserved[name] = true
	}

	out := make([]map[string]interface{}, 0, len(allActionSpecs()))
	for _, spec := range allActionSpecs() {
		name := string(spec.Name)
		entry := map[string]interface{}{
			"name":        name,
			"scope":       actionScopeName(name),
			"description": spec.Description,
			"allowed":     policy == nil || policy.IsAllowed(name),
			"registered":  isRegistered[name],
			"reserved":    isReserved[name],
		}
		if spec.Kind == actionKindGame {
			entry["kind"] = "game"
		}
		out = append(out, entry)
	}

	writeJSON(w, map[string]interface{}{
		"actions":          out,
		"registered":       registered,
		"reserved":         reserved,
		"registered_count": len(registered),
		"reserved_count":   len(reserved),
	})
}

// handlePermissions reads and writes the policy. A PUT applies immediately in
// the running bridge (the previous behaviour required exporting a file and
// restarting), and rejects a policy that would not be loadable.
func (a *AdminServer) handlePermissions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		path := a.integration.permissionsPath
		data, err := os.ReadFile(path)
		if err != nil {
			policy := a.integration.policy()
			payload := map[string]interface{}{
				"note":             "policy file missing; showing runtime defaults",
				"permissions_path": path,
				"default_allow":    policy != nil && policy.DefaultAllow,
				"scopes":           scopeConfigMap(policy),
			}
			writeJSON(w, payload)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)

	case http.MethodPut, http.MethodPost:
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			writeError(w, http.StatusBadRequest, "could not read request body")
			return
		}

		// Validate before touching the disk: a dashboard typo must not leave the
		// bridge with an unparseable policy file.
		var parsed permissionPolicyFile
		if err := json.Unmarshal(body, &parsed); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid policy JSON: %v", err))
			return
		}
		normalized, err := json.MarshalIndent(parsed, "", "  ")
		if err != nil {
			writeError(w, http.StatusBadRequest, "could not normalise policy")
			return
		}

		path := a.integration.permissionsPath
		if path == "" {
			writeError(w, http.StatusBadRequest, "no permissions path configured (NEURO_PERMISSIONS_FILE)")
			return
		}
		if err := atomicWriteFile(path, normalized); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("could not write policy: %v", err))
			return
		}

		policy, err := loadPermissionPolicy(path)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		a.integration.setPolicy(policy)
		a.writeAudit("permissions updated", false)
		log.Printf("Operator updated the permission policy (%s)", path)

		writeJSON(w, map[string]interface{}{
			"ok":              true,
			"saved":           path,
			"applied_live":    true,
			"default_allow":   policy.DefaultAllow,
			"allowed_actions": len(policy.allowed),
			"denied_actions":  len(policy.denied),
		})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func scopeConfigMap(policy *PermissionPolicy) map[string]bool {
	out := map[string]bool{}
	for _, scope := range allScopes() {
		out[string(scope)] = false
	}
	if policy == nil {
		return out
	}
	for scope, cfg := range policy.scopes {
		out[string(scope)] = cfg.Allowed
	}
	return out
}

func (a *AdminServer) handleCatalog(w http.ResponseWriter, _ *http.Request) {
	index, err := loadCatalogIndex(catalogFilePath())
	if err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "error": err.Error(), "items": []CatalogItem{}})
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "items": index.Items, "path": catalogFilePath()})
}

func (a *AdminServer) handleExtensions(w http.ResponseWriter, _ *http.Request) {
	state, err := loadExtensionState(extensionStatePath())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	items := []CatalogItem{}
	if index, err := loadCatalogIndex(catalogFilePath()); err == nil {
		items = index.Items
	}

	installed := make([]map[string]interface{}, 0, len(state.Installed))
	for id, install := range state.Installed {
		entry := map[string]interface{}{
			"id":           id,
			"enabled":      install.Enabled,
			"installed_at": install.InstalledAt,
			"source":       install.Source,
			"path":         install.Path,
		}
		if item := findCatalogItemByID(CatalogIndex{Items: items}, id); item != nil {
			entry["name"] = item.Name
			entry["description"] = item.Description
			entry["type"] = item.Type
			entry["repository"] = item.Repository
		}
		installed = append(installed, entry)
	}
	sort.Slice(installed, func(i, j int) bool {
		return installed[i]["id"].(string) < installed[j]["id"].(string)
	})

	writeJSON(w, map[string]interface{}{
		"ok":            true,
		"installed":     installed,
		"catalog":       items,
		"install_mode":  extensionInstallMode(),
		"extension_dir": extensionRootPath(),
		"state_file":    extensionStatePath(),
	})
}

// handleExtensionAction implements POST /api/extensions/{id}/{action}
func (a *AdminServer) handleExtensionAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, "/api/extensions/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 {
		writeError(w, http.StatusBadRequest, "expected /api/extensions/{id}/{install|enable|disable|uninstall}")
		return
	}

	id := strings.TrimSpace(parts[0])
	action := strings.ToLower(strings.TrimSpace(parts[1]))
	if id == "" {
		writeError(w, http.StatusBadRequest, "extension id is required")
		return
	}

	var result permissiveResult
	switch action {
	case "install":
		result = wrapExecutionResult(a.integration.installExtension(id))
	case "uninstall":
		result = wrapExecutionResult(a.integration.uninstallExtension(id))
	case "enable":
		result = wrapExecutionResult(a.integration.setExtensionEnabled(id, true))
	case "disable":
		result = wrapExecutionResult(a.integration.setExtensionEnabled(id, false))
	default:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown extension action %q", action))
		return
	}

	status := http.StatusOK
	if !result.OK {
		status = http.StatusConflict
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(result)
	a.writeAudit(fmt.Sprintf("extension %s %s", id, action), !result.OK)
}

type permissiveResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	ID      string `json:"id,omitempty"`
	Action  string `json:"action,omitempty"`
}

func wrapExecutionResult(result neuro.ExecutionResult) permissiveResult {
	return permissiveResult{OK: result.Successful, Message: result.Message}
}

// handleGames lists profiles and reports which one matches right now.
func (a *AdminServer) handleGames(w http.ResponseWriter, _ *http.Request) {
	profiles := a.integration.games.Profiles()
	out := make([]map[string]interface{}, 0, len(profiles))
	for _, profile := range profiles {
		out = append(out, map[string]interface{}{
			"id":          profile.ID,
			"name":        profile.Name,
			"description": profile.Description,
			"mode":        profile.Control.Mode,
			"external":    profile.Control.ExternalName,
			"keys":        profile.Keys,
			"mouse_look":  profile.Control.MouseLook.Enabled,
			"vision":      profile.Vision.Recommended,
			"launchable":  len(profile.Launch.Commands) > 0,
			"tags":        profile.Tags,
		})
	}

	var detected *GameDetected
	if a.integration.executorHub != nil && a.integration.executorHub.HasClient() {
		value, err := a.integration.detectGame(true)
		if err == nil {
			detected = &value
		}
	}

	writeJSON(w, map[string]interface{}{
		"ok":              true,
		"profiles":        out,
		"registry_source": a.integration.games.RegistrySource(),
		"detected":        detected,
		"session":         a.integration.games.Session(),
	})
}

func (a *AdminServer) handleGameSession(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var body struct {
			ProfileID string `json:"profile_id"`
			Launch    bool   `json:"launch"`
			Mode      string `json:"mode"`
		}
		if r.Body != nil {
			_ = json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body)
		}

		session := a.integration.games.Session()
		if session != nil {
			a.integration.endGameSession("replaced by operator")
		}

		result := a.integration.startGameSession(
			strings.TrimSpace(body.ProfileID), body.Launch, strings.TrimSpace(body.Mode))
		if !result.Successful {
			writeError(w, http.StatusConflict, result.Message)
			return
		}
		writeJSON(w, map[string]interface{}{"ok": true, "message": result.Message, "session": a.integration.games.Session()})

	case http.MethodDelete:
		result := a.integration.endGameSession("stopped by operator")
		writeJSON(w, map[string]interface{}{"ok": result.Successful, "message": result.Message})

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *AdminServer) handleGameRelease(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	result := a.integration.releaseAllInput()
	status := http.StatusOK
	if !result.Successful {
		status = http.StatusConflict
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": result.Successful, "message": result.Message})
}

// handleGameObserve answers "what would Neuro see right now?" for the
// dashboard. It is the same capture + vision path game_observe uses, minus the
// context message.
func (a *AdminServer) handleGameObserve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var body struct {
		Vision bool   `json:"vision"`
		Prompt string `json:"prompt"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body)
	}
	if !body.Vision && body.Prompt == "" {
		// The zero value of a JSON body is "no vision"; the dashboard's default
		// is to ask the vision server when one is configured.
		body.Vision = visionServerURL() != ""
	}

	message, err := a.integration.buildGameObservation(body.Vision, strings.TrimSpace(body.Prompt))
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	a.writeAudit("game observe", false)
	writeJSON(w, map[string]interface{}{"ok": true, "observation": message})
}

func (a *AdminServer) handleRelay(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]interface{}{
		"ok":    true,
		"relay": a.integration.relay.status(),
	})
}

func (a *AdminServer) handleConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]interface{}{
		"ok":      true,
		"version": Version,
		"paths": map[string]interface{}{
			"permissions":   a.integration.permissionsPath,
			"ipc":           a.integration.ipcFilePath,
			"catalog":       catalogFilePath(),
			"extensions":    extensionStatePath(),
			"game_profiles": a.integration.games.RegistrySource(),
			"extension_dir": extensionRootPath(),
			"install_mode":  extensionInstallMode(),
		},
		"features": map[string]interface{}{
			"game_actions":   RegisterGameActionsOnStartup,
			"relay_enabled":  a.integration.relay.status().Enabled,
			"vision_enabled": visionServerURL() != "",
			"executor_token": a.integration.executorHub != nil && a.integration.executorHub.tokenRequired(),
		},
	})
}

// ---------------------------------------------------------------
// Static dashboard hosting
// ---------------------------------------------------------------

func (a *AdminServer) handleUI(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		http.Redirect(w, r, "/ui/", http.StatusFound)
		return
	}

	if !strings.HasPrefix(path, "ui") && path != "favicon.ico" {
		http.NotFound(w, r)
		return
	}

	root, err := resolveFrontendRoot()
	if err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "Neuro Desktop dashboard assets not found: %v\n\nBuild the UI with `npm run build` in desktop/frontend, or use the bundled release.", err)
		return
	}

	relative := strings.TrimPrefix(strings.TrimPrefix(path, "ui"), "/")
	if relative == "" {
		relative = "index.html"
	}

	candidate := filepath.Join(root, filepath.Clean("/"+relative))
	if info, err := os.Stat(candidate); err != nil || info.IsDir() {
		// Single-page app: unknown paths fall back to the shell.
		candidate = filepath.Join(root, "index.html")
	}

	if _, err := os.Stat(candidate); err != nil {
		http.NotFound(w, r)
		return
	}

	if filepath.Base(candidate) == "index.html" {
		a.serveDashboardShell(w, r, candidate)
		return
	}

	http.ServeFile(w, r, candidate)
}

// serveDashboardShell serves index.html, injecting the admin token when the
// caller is already local. Without it every dashboard action would need the
// operator to copy NEURO_ADMIN_TOKEN into the browser by hand; with it, a remote
// browser is never handed write access it could not otherwise obtain.
func (a *AdminServer) serveDashboardShell(w http.ResponseWriter, r *http.Request, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	shell := string(data)
	if token := a.dashboardToken(r); token != "" {
		payload, err := json.Marshal(map[string]interface{}{
			"token":      token,
			"version":    Version,
			"api_base":   "",
			"nativeHost": false,
		})
		if err == nil {
			snippet := "<script>window.__ND_BOOTSTRAP=" + string(payload) + ";</script>"
			if strings.Contains(shell, "</head>") {
				shell = strings.Replace(shell, "</head>", snippet+"</head>", 1)
			} else {
				shell = snippet + shell
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The shell is regenerated per request (and a rebuilt UI must not be served
	// from the browser cache), so hashed assets aside, do not cache it.
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(shell))
}

// dashboardToken decides whether the admin token may be handed to this request.
func (a *AdminServer) dashboardToken(r *http.Request) string {
	if a.token == "" {
		return ""
	}
	// Either the server itself is only reachable locally...
	if isLoopbackAddr(a.addr) {
		return a.token
	}
	// ...or this particular caller is local.
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		if host == "localhost" {
			return a.token
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return a.token
		}
	}
	return ""
}

func resolveFrontendRoot() (string, error) {
	candidates := []string{}

	// An explicit override wins: useful for a custom build or a packaged UI
	// placed outside the executable's directory.
	if override := strings.TrimSpace(os.Getenv("NEURO_UI_DIR")); override != "" {
		candidates = append(candidates, override)
	}

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "frontend"),
			filepath.Join(exeDir, "ui"),
		)
	}

	candidates = append(candidates,
		filepath.Join("frontend", "dist"),
		filepath.Join("desktop", "frontend", "dist"),
		filepath.Join("..", "..", "frontend", "dist"),
	)

	for _, candidate := range candidates {
		if info, err := os.Stat(filepath.Join(candidate, "index.html")); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no frontend/dist/index.html in any of: %s", strings.Join(candidates, ", "))
}
