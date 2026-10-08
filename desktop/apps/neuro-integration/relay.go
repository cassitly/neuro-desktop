package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Neuro Relay support.
//
// Neuro Relay (https://github.com/Nakashireyumi/neuro-relay) multiplexes
// several integrations behind one Neuro connection. Neuro Desktop takes part in
// two ways:
//
//  1. As a Neuro API client pointed at the relay instead of the raw backend
//     (NEURO_SDK_WS_URL / --ws-url). In that mode the relay owns action
//     arbitration and namespaces each integration's actions.
//  2. As a relay *integration*: Neuro Desktop registers on the relay's
//     intermediary socket so Neuro-OS watchers can see it and drive it. That
//     registration is what this file implements.
//
// The earlier shipped code spawned `neuro-relay -name X -neuro-url Y
// -emulated-addr Z`, flags that the real relay does not have: its configuration
// lives in a YAML file. Here the supervisor runs an explicit command template
// (or nothing at all) and only health-checks the socket.
type RelayConfig struct {
	Enabled         bool
	IntermediaryURL string
	Token           string
	Name            string
	// ReservedActions are action names owned by another integration. They are
	// left unregistered so Neuro Desktop cannot shadow a game integration.
	ReservedActions []string
	// Command launches the relay process when Neuro Desktop should host it.
	Command []string
	// BackupCommand is tried when Command fails, for Python-style installs.
	// %config% is replaced with the config path.
	ConfigPath string
	// Restart limits.
	MaxRestarts int
}

func relayConfigFromEnv() RelayConfig {
	cfg := RelayConfig{
		Enabled:         getEnvBool("NEURO_RELAY_ENABLED", false),
		IntermediaryURL: strings.TrimSpace(os.Getenv("NEURO_RELAY_URL")),
		Token:           strings.TrimSpace(os.Getenv("NEURO_RELAY_TOKEN")),
		Name:            nonEmptyOr(strings.TrimSpace(os.Getenv("NEURO_RELAY_NAME")), "Neuro Desktop"),
		ConfigPath:      strings.TrimSpace(os.Getenv("NEURO_RELAY_CONFIG")),
		MaxRestarts:     getEnvInt("NEURO_RELAY_MAX_RESTARTS", 5),
	}

	if cfg.IntermediaryURL == "" {
		addr := strings.TrimSpace(os.Getenv("NEURO_RELAY_EMULATED_ADDR"))
		if addr == "" {
			addr = "127.0.0.1:8765"
		}
		cfg.IntermediaryURL = "ws://" + strings.TrimPrefix(addr, "ws://")
	}

	if raw := strings.TrimSpace(os.Getenv("NEURO_RELAY_COMMAND")); raw != "" {
		cfg.Command = splitCommandLine(raw)
	}

	cfg.ReservedActions = append(cfg.ReservedActions, reservedActionsFromEnv()...)

	return cfg
}

// reservedActionsFromEnv reads the action names that belong to another
// integration (NEURO_RESERVED_ACTIONS, comma separated). Keeping this readable
// outside the relay config means reserved names work even when the relay link
// itself is switched off.
func reservedActionsFromEnv() []string {
	var out []string
	for _, name := range strings.Split(os.Getenv("NEURO_RESERVED_ACTIONS"), ",") {
		trimmed := strings.TrimSpace(name)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// splitCommandLine splits a command template on spaces, honouring double quotes.
func splitCommandLine(raw string) []string {
	var out []string
	var current strings.Builder
	inQuotes := false

	for _, r := range raw {
		switch {
		case r == '"':
			inQuotes = !inQuotes
		case r == ' ' && !inQuotes:
			if current.Len() > 0 {
				out = append(out, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		out = append(out, current.String())
	}
	return out
}

func (c RelayConfig) endpoint() string {
	return strings.TrimSuffix(c.IntermediaryURL, "/")
}

// RelayState is the observable status of the relay link.
type RelayState struct {
	mu         sync.Mutex
	cfg        RelayConfig
	cmd        *exec.Cmd
	connected  bool
	registered bool
	restarts   int
	lastError  string
	lastEvent  string
	lastSeen   time.Time
	// Peers are integration names the relay told us about.
	peers map[string]string
	stop  chan struct{}
	done  chan struct{}
	// owner is the integration this relay link belongs to (set during wiring).
	owner *NDIntegration
}

func newRelayState(cfg RelayConfig) *RelayState {
	return &RelayState{
		cfg:   cfg,
		peers: map[string]string{},
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}
}

// RelayStatus is the JSON shape used by the dashboard.
type RelayStatus struct {
	Enabled      bool              `json:"enabled"`
	URL          string            `json:"url,omitempty"`
	Connected    bool              `json:"connected"`
	Registered   bool              `json:"registered"`
	PeerCount    int               `json:"peer_count"`
	Peers        map[string]string `json:"peers,omitempty"`
	Restarts     int               `json:"process_restarts"`
	LastError    string            `json:"last_error,omitempty"`
	LastEvent    string            `json:"last_event,omitempty"`
	LastSeen     string            `json:"last_seen,omitempty"`
	ManagedByUs  bool              `json:"process_managed_here"`
	Reserved     []string          `json:"reserved_actions,omitempty"`
	SupervisedBy string            `json:"supervised_by,omitempty"`
}

func (r *RelayState) status() RelayStatus {
	if r == nil {
		return RelayStatus{Enabled: false}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	out := RelayStatus{
		Enabled:     r.cfg.Enabled,
		URL:         r.cfg.IntermediaryURL,
		Connected:   r.connected,
		Registered:  r.registered,
		PeerCount:   len(r.peers),
		Restarts:    r.restarts,
		LastError:   r.lastError,
		LastEvent:   r.lastEvent,
		ManagedByUs: r.cmd != nil,
		Reserved:    append([]string{}, r.cfg.ReservedActions...),
	}
	if len(r.peers) > 0 {
		out.Peers = map[string]string{}
		for name, kind := range r.peers {
			out.Peers[name] = kind
		}
	}
	if !r.lastSeen.IsZero() {
		out.LastSeen = r.lastSeen.UTC().Format(time.RFC3339)
	}
	if r.cmd != nil {
		out.SupervisedBy = "neuro-integration"
	}
	return out
}

func (r *RelayState) reservedSet() map[string]bool {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.cfg.ReservedActions) == 0 {
		return nil
	}
	out := make(map[string]bool, len(r.cfg.ReservedActions))
	for _, name := range r.cfg.ReservedActions {
		out[strings.ToLower(strings.TrimSpace(name))] = true
	}
	return out
}

func (r *RelayState) noteEvent(event string, err error) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastSeen = time.Now()
	if event != "" {
		r.lastEvent = event
	}
	if err != nil {
		r.lastError = err.Error()
	}
}

func (r *RelayState) notePeer(name, kind string) {
	if r == nil || name == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.peers[name] = kind
	r.lastSeen = time.Now()
}

func (r *RelayState) dropPeer(name string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.peers, name)
}

func (r *RelayState) isRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cmd != nil && r.cmd.Process != nil
}

// Start brings up the optional relay process. It is deliberately tolerant: a
// relay that cannot be launched must not stop Neuro Desktop from working as a
// direct Neuro integration.
func (r *RelayState) Start() error {
	if r == nil || !r.cfg.Enabled {
		return nil
	}

	if len(r.cfg.Command) > 0 {
		if err := r.startProcess(); err != nil {
			log.Printf("Relay process could not be started (%v); continuing against %s if it is already running",
				err, r.cfg.IntermediaryURL)
		}
	}

	go r.clientLoop()
	return nil
}

func (r *RelayState) startProcess() error {
	r.mu.Lock()
	if r.cmd != nil {
		r.mu.Unlock()
		return fmt.Errorf("relay process already running")
	}
	command := append([]string{}, r.cfg.Command...)
	r.mu.Unlock()

	if len(command) == 0 {
		return fmt.Errorf("no relay command configured")
	}

	// %config% lets operators point at the relay's real YAML config instead of
	// inventing CLI flags the relay does not support.
	for i, arg := range command {
		if strings.Contains(arg, "%config%") {
			if r.cfg.ConfigPath == "" {
				return fmt.Errorf("relay command needs %%config%% but NEURO_RELAY_CONFIG is unset")
			}
			command[i] = strings.ReplaceAll(arg, "%config%", r.cfg.ConfigPath)
		}
	}

	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}

	r.mu.Lock()
	r.cmd = cmd
	r.restarts++
	r.mu.Unlock()

	log.Printf("Relay process started (pid %d): %s", cmd.Process.Pid, strings.Join(command, " "))
	return nil
}

func (r *RelayState) Stop() {
	if r == nil {
		return
	}

	select {
	case <-r.stop:
	default:
		close(r.stop)
	}

	r.mu.Lock()
	cmd := r.cmd
	r.cmd = nil
	r.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		log.Printf("Relay process stopped")
	}
}

// waitForSocket makes the client retry until the relay is actually accepting
// connections (a freshly spawned relay needs a moment to bind).
func (r *RelayState) waitForSocket(timeout time.Duration) error {
	if r == nil {
		return fmt.Errorf("relay disabled")
	}

	parsed, err := url.Parse(r.cfg.endpoint())
	if err != nil {
		return err
	}
	host := parsed.Host
	if host == "" {
		host = parsed.Path
	}
	if !strings.Contains(host, ":") {
		host += ":80"
	}

	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case <-r.stop:
			return fmt.Errorf("relay stopped")
		default:
		}
		conn, err := net.DialTimeout("tcp", host, 750*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("relay socket %s not reachable: %w", host, lastErr)
}

func (r *RelayState) clientLoop() {
	backoff := time.Second

	for {
		select {
		case <-r.stop:
			close(r.done)
			return
		default:
		}

		if err := r.waitForSocket(15 * time.Second); err != nil {
			r.noteEvent("unreachable", err)
			if !r.sleep(backoff) {
				close(r.done)
				return
			}
			backoff = nextBackoff(backoff, 30*time.Second)
			continue
		}

		err := r.runSession()
		r.setConnectionState(false, false)
		if err != nil {
			r.noteEvent("disconnected", err)
		}
		if !r.sleep(backoff) {
			close(r.done)
			return
		}
		backoff = nextBackoff(backoff, 30*time.Second)
	}
}

func (r *RelayState) sleep(d time.Duration) bool {
	select {
	case <-r.stop:
		return false
	case <-time.After(d):
		return true
	}
}

func nextBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		return max
	}
	return next
}

func (r *RelayState) setConnectionState(connected bool, registered bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connected = connected
	r.registered = registered
	if !connected {
		r.peers = map[string]string{}
	}
}

// runSession registers with the relay intermediary and serves routed commands
// until the socket drops.
func (r *RelayState) runSession() error {
	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	conn, _, err := dialer.Dial(r.cfg.endpoint(), nil)
	if err != nil {
		return fmt.Errorf("relay dial failed: %w", err)
	}
	defer func() { _ = conn.Close() }()

	registration := map[string]interface{}{
		"type":       "integration",
		"name":       r.cfg.Name,
		"auth_token": r.cfg.Token,
		"role":       "neuro-desktop",
		"platform":   platformKey(),
	}
	if err := conn.WriteJSON(registration); err != nil {
		return fmt.Errorf("relay registration failed: %w", err)
	}

	r.setConnectionState(true, true)
	r.noteEvent("registered", nil)
	log.Printf("Registered with Neuro Relay as %q at %s", r.cfg.Name, r.cfg.endpoint())

	// Announce which actions Neuro Desktop provides so the relay can build its
	// unified action context for Neuro.
	if actions := r.actionSummary(); len(actions) > 0 {
		_ = conn.WriteJSON(map[string]interface{}{
			"event":   "register_actions",
			"actions": actions,
		})
	}

	if err := conn.WriteJSON(map[string]interface{}{
		"event": "status",
		"payload": map[string]interface{}{
			"executor_connected": r.hasExecutor(),
			"game_profiles":      r.gameProfileCount(),
			"platform":           platformKey(),
		},
	}); err != nil {
		return err
	}

	for {
		_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		_, message, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		_ = conn.SetReadDeadline(time.Time{})

		var envelope map[string]interface{}
		if err := json.Unmarshal(message, &envelope); err != nil {
			continue
		}

		r.handleRelayMessage(envelope)
	}
}

func (r *RelayState) handleRelayMessage(envelope map[string]interface{}) {
	if err, ok := envelope["error"].(string); ok && err != "" {
		r.noteEvent("error", fmt.Errorf("%s", err))
		log.Printf("Relay reported an error: %s", err)
		return
	}

	if event, ok := envelope["event"].(string); ok {
		r.noteEvent(event, nil)
		switch event {
		case "integration_connected":
			name, _ := envelope["name"].(string)
			r.notePeer(name, "integration")
			log.Printf("Relay: integration %q connected", name)
		case "integration_disconnected":
			name, _ := envelope["name"].(string)
			r.dropPeer(name)
			log.Printf("Relay: integration %q disconnected", name)
		case "neuroos_connected":
			name, _ := envelope["name"].(string)
			r.notePeer(name, "neuro-os")
			log.Printf("Relay: Neuro-OS watcher %q connected", name)
		case "neuroos_disconnected":
			name, _ := envelope["name"].(string)
			r.dropPeer(name)
		case "integration_message":
			from, _ := envelope["from"].(string)
			r.notePeer(from, "integration")
		}
		// Events still may carry a command (see below).
	}

	// A watcher (Neuro-OS / operator UI) can drive Neuro Desktop through the
	// relay. Commands are subject to the same permission policy as Neuro.
	if cmd, ok := envelope["cmd"].(map[string]interface{}); ok {
		from, _ := envelope["from_watcher"].(string)
		r.notePeer(nonEmptyOr(from, "watcher"), "neuro-os")
		go r.executeRelayCommand(from, cmd)
	}
}

// executeRelayCommand runs a watcher-issued command, e.g.
// {"target": "neuro-desktop", "cmd": {"action": "type_text", "params": {...}}}
func (r *RelayState) executeRelayCommand(from string, cmd map[string]interface{}) {
	name, _ := cmd["action"].(string)
	if name == "" {
		name, _ = cmd["name"].(string)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		log.Printf("Relay: ignoring command from %q without an action name", from)
		return
	}

	integration := r.integration()
	if integration == nil {
		return
	}

	policy := integration.policy()
	if policy != nil && !policy.IsAllowed(name) {
		integration.stats.noteDenied(name)
		log.Printf("Relay: refused %q from %q (denied by policy)", name, from)
		return
	}

	params := map[string]interface{}{}
	if raw, ok := cmd["params"].(map[string]interface{}); ok {
		params = raw
	}

	command, err := buildIPCCommand(CommandType(name), params, true, true)
	if err != nil {
		log.Printf("Relay: command %q from %q is not supported: %v", name, from, err)
		return
	}

	if _, err := integration.sendToRust(command); err != nil {
		log.Printf("Relay: command %q from %q failed: %v", name, from, err)
		return
	}
	log.Printf("Relay: executed %q for %q", name, from)
}

// integration back-references the owning integration (set during wiring).
func (r *RelayState) integration() *NDIntegration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.owner
}

func (r *RelayState) setOwner(integration *NDIntegration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.owner = integration
}

func (r *RelayState) hasExecutor() bool {
	integration := r.integration()
	if integration == nil || integration.executorHub == nil {
		return false
	}
	return integration.executorHub.HasClient()
}

func (r *RelayState) gameProfileCount() int {
	integration := r.integration()
	if integration == nil || integration.games == nil {
		return 0
	}
	return len(integration.games.Profiles())
}

// actionSummary lists the Neuro-facing actions this build provides.
func (r *RelayState) actionSummary() map[string]string {
	integration := r.integration()
	if integration == nil {
		return nil
	}

	out := map[string]string{}
	for _, spec := range append(append(HLActionSpecs, LLActionSpecs...), gameActionSpecs()...) {
		out[string(spec.Name)] = spec.Description
	}
	return out
}

// ReservedActionNames returns the sorted list of actions left to other
// integrations (used by the dashboard and by registration filtering). It merges
// the environment list with the relay configuration.
func (r *RelayState) ReservedActionNames() []string {
	reserved := map[string]bool{}
	for _, name := range reservedActionsFromEnv() {
		reserved[strings.ToLower(strings.TrimSpace(name))] = true
	}
	for name := range r.reservedSet() {
		reserved[name] = true
	}
	if len(reserved) == 0 {
		return nil
	}

	out := make([]string, 0, len(reserved))
	for name := range reserved {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
