package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// fakeRelay speaks what Nakashireyumi/neuro-relay actually speaks.
//
// The behaviours below come from reading the upstream sources:
//
//   - intermediary.py: the first frame must be
//     {"type": "integration" | "neuro-os", "name": ..., "auth_token": ...};
//     a bad token answers {"error": "invalid auth token"} and closes.
//   - intermediary.py: {"event": "register_actions", "actions": {...}} is kept
//     as a name -> schema mapping and namespaced "<integration>.<action>".
//   - intermediary.py: watcher commands arrive as
//     {"from_watcher": "...", "cmd": {"action": ..., "params": {...}}}.
//   - intermediary.py: everything else is forwarded to the Neuro backend.
//
// The test asserts our side of each of those, which is what "the relay might be
// broken" was about: the message shapes are easy to get subtly wrong.
type fakeRelay struct {
	t       *testing.T
	token   string
	mu      sync.Mutex
	first   map[string]interface{}
	actions map[string]interface{}
	status  map[string]interface{}
	other   []map[string]interface{}
	conns   []*websocket.Conn
}

func newFakeRelay(t *testing.T, token string) (*fakeRelay, *httptest.Server) {
	relay := &fakeRelay{t: t, token: token}
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("fake relay upgrade: %v", err)
			return
		}
		relay.mu.Lock()
		relay.conns = append(relay.conns, conn)
		relay.mu.Unlock()
		go relay.serve(conn)
	}))
	return relay, server
}

func (f *fakeRelay) serve(conn *websocket.Conn) {
	defer func() { _ = conn.Close() }()

	_, raw, err := conn.ReadMessage()
	if err != nil {
		return
	}
	var registration map[string]interface{}
	if err := json.Unmarshal(raw, &registration); err != nil {
		_ = conn.WriteJSON(map[string]interface{}{"error": "registration must be JSON"})
		return
	}

	f.mu.Lock()
	f.first = registration
	f.mu.Unlock()

	token, _ := registration["auth_token"].(string)
	if token != f.token {
		_ = conn.WriteJSON(map[string]interface{}{"error": "invalid auth token"})
		return
	}
	if registration["type"] != "integration" {
		_ = conn.WriteJSON(map[string]interface{}{"error": "unknown registration type"})
		return
	}

	// Watcher-visible connection event, exactly like _notify_watchers does.
	_ = conn.WriteJSON(map[string]interface{}{
		"event": "integration_connected",
		"name":  registration["name"],
	})

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var envelope map[string]interface{}
		if err := json.Unmarshal(message, &envelope); err != nil {
			continue
		}

		f.mu.Lock()
		switch envelope["event"] {
		case "register_actions":
			f.actions = map[string]interface{}{}
			if actions, ok := envelope["actions"].(map[string]interface{}); ok {
				for name, schema := range actions {
					f.actions[name] = schema
				}
			}
		case "status":
			if payload, ok := envelope["payload"].(map[string]interface{}); ok {
				f.status = payload
			}
		default:
			f.other = append(f.other, envelope)
		}
		f.mu.Unlock()
	}
}

func (f *fakeRelay) snapshot() (map[string]interface{}, map[string]interface{}, map[string]interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var actions map[string]interface{}
	if f.actions != nil {
		actions = map[string]interface{}{}
		for name, schema := range f.actions {
			actions[name] = schema
		}
	}
	return f.first, actions, f.status
}

func newRelayTestHarness(t *testing.T, endpoint string, token string) (*NDIntegration, *RelayState) {
	t.Helper()

	integration := &NDIntegration{
		stats: newBridgeStats(),
		stop:  newStopSwitch(),
		audit: newAuditor(),
		games: newGameRuntime(&GameRegistry{}),
	}

	cfg := RelayConfig{
		Enabled:         true,
		IntermediaryURL: endpoint,
		Token:           token,
		Name:            "Neuro Desktop",
	}
	relay := newRelayState(cfg)
	relay.setOwner(integration)
	integration.relay = relay
	return integration, relay
}

func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestRelayRegistrationMatchesIntermediaryProtocol(t *testing.T) {
	relay, server := newFakeRelay(t, "super-secret-token")
	defer server.Close()

	integration, state := newRelayTestHarness(t, "ws"+strings.TrimPrefix(server.URL, "http"), "super-secret-token")
	go state.clientLoop()
	defer state.Stop()

	waitFor(t, "registration and action announcement", func() bool {
		_, actions, status := relay.snapshot()
		return actions != nil && status != nil
	})

	first, actions, status := relay.snapshot()

	if first["type"] != "integration" {
		t.Fatalf(`the first frame must be a registration, got %#v`, first)
	}
	if first["name"] != "Neuro Desktop" {
		t.Fatalf("registration name is %v, want Neuro Desktop", first["name"])
	}
	if first["auth_token"] != "super-secret-token" {
		t.Fatalf("registration must carry the intermediary auth_token, got %v", first["auth_token"])
	}

	// register_actions must be a name -> schema mapping: the intermediary does
	// `for act_name, schema in actions.items()`.
	if len(actions) == 0 {
		t.Fatal("no actions were registered with the relay")
	}
	for name, raw := range actions {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("action %q schema is %T, want an object", name, raw)
		}
		if entry["description"] == nil && entry["name"] == nil {
			t.Fatalf("action %q has neither name nor description: %#v", name, entry)
		}
	}
	for _, expected := range []string{"move_mouse_to", "game_move", "desktop_guide"} {
		if _, ok := actions[expected]; !ok {
			t.Fatalf("expected %s to be announced to the relay", expected)
		}
	}

	// A status event tells watchers we are alive; it must carry the payload the
	// bridge actually reports.
	if status["platform"] == nil {
		t.Fatalf("status payload should carry the platform, got %#v", status)
	}

	if !state.status().Registered {
		t.Fatalf("relay state should report registered, got %#v", state.status())
	}
	_ = integration
}

func TestRelayRejectsBadTokenWithAClearError(t *testing.T) {
	relay, server := newFakeRelay(t, "the-real-token")
	defer server.Close()

	_, state := newRelayTestHarness(t, "ws"+strings.TrimPrefix(server.URL, "http"), "wrong-token")

	err := state.runSession()
	if err == nil {
		t.Fatal("a rejected registration must return an error, not look healthy")
	}
	if !strings.Contains(err.Error(), "invalid auth token") {
		t.Fatalf("the error should quote the relay's answer, got: %v", err)
	}
	if !strings.Contains(err.Error(), "NEURO_RELAY_TOKEN") {
		t.Fatalf("the error should tell the operator which setting to check, got: %v", err)
	}
	_ = relay
}

func TestRelayCapturesPeerActionsFromWatcherEvents(t *testing.T) {
	relay, server := newFakeRelay(t, "token")
	defer server.Close()

	_, state := newRelayTestHarness(t, "ws"+strings.TrimPrefix(server.URL, "http"), "token")
	go state.clientLoop()
	defer state.Stop()

	waitFor(t, "the relay connection", func() bool {
		relay.mu.Lock()
		defer relay.mu.Unlock()
		return len(relay.conns) > 0
	})

	relay.mu.Lock()
	conn := relay.conns[0]
	relay.mu.Unlock()

	send := func(envelope map[string]interface{}) {
		payload, err := json.Marshal(envelope)
		if err != nil {
			t.Fatalf("marshal envelope: %v", err)
		}
		if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			t.Fatalf("write envelope: %v", err)
		}
	}

	// A peer integration connected and registered its actions: the relay tells
	// watchers, which is how we learn the names to hand to Neuro.
	send(map[string]interface{}{"event": "integration_connected", "name": "minecraft"})
	send(map[string]interface{}{
		"event":   "integration_registered_actions",
		"from":    "minecraft",
		"actions": []interface{}{"minecraft.move_forward", "minecraft.jump"},
	})

	waitFor(t, "peer actions to be recorded", func() bool {
		status := state.status()
		return len(status.PeerActions["minecraft"]) == 2
	})

	status := state.status()
	if status.Peers["minecraft"] != "integration" {
		t.Fatalf("peer kind not recorded: %#v", status.Peers)
	}
	hint := state.peerActionHint("minecraft")
	for _, name := range []string{"minecraft.move_forward", "minecraft.jump"} {
		if !strings.Contains(hint, name) {
			t.Fatalf("hint %q should name %s", hint, name)
		}
	}

	// The external-mode message must now point Neuro at real action names.
	profile := &GameProfile{ID: "minecraft", Name: "Minecraft"}
	profile.Control.Mode = ControlModeExternal
	profile.Control.ExternalName = "minecraft"
	err := inputAllowed(profile, nil, "game_move", state)
	if err == nil {
		t.Fatal("external mode must refuse local input")
	}
	if !strings.Contains(err.Error(), "minecraft.move_forward") {
		t.Fatalf("the refusal should name the peer's actions, got: %v", err)
	}

	// Disconnecting the peer clears the cache.
	send(map[string]interface{}{"event": "integration_disconnected", "name": "minecraft"})
	waitFor(t, "peer removal", func() bool {
		status := state.status()
		if len(status.Peers) == 0 && len(status.PeerActions) == 0 {
			return true
		}
		t.Logf("still present: peers=%v actions=%v last_event=%s", status.Peers, status.PeerActions, status.LastEvent)
		return false
	})
}

func TestRelayActionNamesAcceptAllShapes(t *testing.T) {
	list := actionNamesFrom([]interface{}{"a", "b", map[string]interface{}{"name": "c"}})
	if strings.Join(list, ",") != "a,b,c" {
		t.Fatalf("list shape parsed as %v", list)
	}

	dict := actionNamesFrom(map[string]interface{}{"z": map[string]interface{}{}, "y": map[string]interface{}{}})
	if strings.Join(dict, ",") != "y,z" {
		t.Fatalf("dict shape parsed as %v", dict)
	}

	if got := actionNamesFrom("nonsense"); got != nil {
		t.Fatalf("a string should yield nothing, got %v", got)
	}
}

func TestRelayStatusReportsCoexistence(t *testing.T) {
	_, state := newRelayTestHarness(t, "ws://127.0.0.1:9", "token")
	state.notePeer("minecraft", "integration")
	state.notePeerActions("minecraft", []string{"minecraft.move"})

	status := state.status()
	if status.PeerCount != 1 {
		t.Fatalf("peer count = %d, want 1", status.PeerCount)
	}
	if got := status.PeerActions["minecraft"]; len(got) != 1 || got[0] != "minecraft.move" {
		t.Fatalf("peer actions not reported: %#v", status.PeerActions)
	}
	if status.Enabled != true || status.URL != "ws://127.0.0.1:9" {
		t.Fatalf("unexpected status: %#v", status)
	}
}
