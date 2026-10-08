package main

import (
	"bufio"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

// ExecutorHub is the server-side socket that remote (or local) executor
// clients connect to. Neuro actions are forwarded here instead of only
// using same-machine file IPC.
//
// Design notes (why this is not a naive lock-and-read):
//   - Each connection owns a reader goroutine that dispatches results to the
//     goroutine that sent the matching request id. A slow action therefore
//     never blocks /api/status, the dashboard, or another action.
//   - A timed-out request only abandons that request; the executor session
//     stays alive (a long script is not a dead executor).
//   - Optional shared-secret token: the hub listens on 0.0.0.0 by default so
//     that a second PC can be controlled, so an unauthenticated executor
//     socket would be a remote-control hole.
type ExecutorHub struct {
	addr  string
	token string

	mu       sync.Mutex
	listener net.Listener
	client   *executorConn

	// Telemetry for the operator dashboard.
	statsMu    sync.Mutex
	connected  int
	completed  int
	failed     int
	lastError  string
	lastRemote string
	lastSeen   time.Time
}

type executorEnvelope struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Command json.RawMessage `json:"command,omitempty"`
	Success bool            `json:"success,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
	Role    string          `json:"role,omitempty"`
	Version string          `json:"version,omitempty"`
	Token   string          `json:"token,omitempty"`
	Event   string          `json:"event,omitempty"`
}

type executorResult struct {
	response *IPCResponse
	err      error
}

// executorConn wraps a single connected executor client.
type executorConn struct {
	conn   net.Conn
	reader *bufio.Reader
	remote string

	writeMu sync.Mutex

	pendingMu sync.Mutex
	pending   map[string]chan executorResult

	closeOnce sync.Once
	closed    chan struct{}
}

func (c *executorConn) close() {
	c.closeOnce.Do(func() {
		close(c.closed)
		_ = c.conn.Close()
		c.pendingMu.Lock()
		for id, ch := range c.pending {
			delete(c.pending, id)
			select {
			case ch <- executorResult{err: fmt.Errorf("executor connection closed")}:
			default:
			}
		}
		c.pendingMu.Unlock()
	})
}

func (c *executorConn) writeLine(payload []byte) error {
	payload = append(payload, '\n')
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
	_, err := c.conn.Write(payload)
	_ = c.conn.SetWriteDeadline(time.Time{})
	return err
}

func (c *executorConn) register(id string) chan executorResult {
	ch := make(chan executorResult, 1)
	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()
	return ch
}

func (c *executorConn) resolve(id string, result executorResult) bool {
	c.pendingMu.Lock()
	ch, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	c.pendingMu.Unlock()
	if !ok {
		return false
	}
	select {
	case ch <- result:
	default:
	}
	return true
}

func (c *executorConn) abandon(id string) {
	c.pendingMu.Lock()
	delete(c.pending, id)
	c.pendingMu.Unlock()
}

func NewExecutorHub(addr string, token string) *ExecutorHub {
	if addr == "" {
		addr = "127.0.0.1:9876"
	}
	return &ExecutorHub{addr: addr, token: token}
}

func (h *ExecutorHub) Start() error {
	ln, err := net.Listen("tcp", h.addr)
	if err != nil {
		return fmt.Errorf("executor hub listen %s: %w", h.addr, err)
	}
	h.mu.Lock()
	h.listener = ln
	// Record the address the OS actually bound (tests and logs need the real
	// port when listening on :0).
	h.addr = ln.Addr().String()
	h.mu.Unlock()

	log.Printf("Executor hub listening on %s (clients connect here)", h.addr)
	if h.tokenRequired() && !h.addrIsLoopback() {
		log.Printf("Executor hub requires a token; non-loopback clients must send it in the hello frame")
	}
	if !h.tokenRequired() && !h.addrIsLoopback() {
		log.Printf("WARNING: executor hub is reachable from the network without NEURO_EXECUTOR_TOKEN set")
	}
	go h.acceptLoop(ln)
	return nil
}

func (h *ExecutorHub) tokenRequired() bool {
	return h.token != ""
}

func (h *ExecutorHub) addrIsLoopback() bool {
	host, _, err := net.SplitHostPort(h.addr)
	if err != nil {
		return false
	}
	switch host {
	case "127.0.0.1", "localhost", "::1":
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Addr returns the address the hub is listening on.
func (h *ExecutorHub) Addr() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.addr
}

func (h *ExecutorHub) Close() {
	h.mu.Lock()
	ln := h.listener
	client := h.client
	h.listener = nil
	h.client = nil
	h.mu.Unlock()

	if client != nil {
		client.close()
	}
	if ln != nil {
		_ = ln.Close()
	}
}

func (h *ExecutorHub) HasClient() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.client != nil
}

// ClientInfo describes the connected executor for the dashboard.
type ExecutorInfo struct {
	Connected   bool   `json:"connected"`
	Remote      string `json:"remote,omitempty"`
	Version     string `json:"version,omitempty"`
	Completed   int    `json:"completed_commands"`
	Failed      int    `json:"failed_commands"`
	LastError   string `json:"last_error,omitempty"`
	LastSeen    string `json:"last_seen,omitempty"`
	Connections int    `json:"total_connections"`
}

func (h *ExecutorHub) Info() ExecutorInfo {
	h.mu.Lock()
	client := h.client
	h.mu.Unlock()

	h.statsMu.Lock()
	defer h.statsMu.Unlock()

	info := ExecutorInfo{
		Connected:   client != nil,
		Completed:   h.completed,
		Failed:      h.failed,
		LastError:   h.lastError,
		Connections: h.connected,
	}
	if client != nil {
		info.Remote = client.remote
	}
	if !h.lastSeen.IsZero() {
		info.LastSeen = h.lastSeen.UTC().Format(time.RFC3339)
	}
	return info
}

func (h *ExecutorHub) noteResult(err error) {
	h.statsMu.Lock()
	defer h.statsMu.Unlock()
	h.lastSeen = time.Now()
	if err != nil {
		h.failed++
		h.lastError = err.Error()
		return
	}
	h.completed++
}

func (h *ExecutorHub) acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go h.handleConnection(conn)
	}
}

func (h *ExecutorHub) handleConnection(conn net.Conn) {
	remote := conn.RemoteAddr().String()

	reader := bufio.NewReader(conn)
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	line, err := reader.ReadBytes('\n')
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		log.Printf("Executor handshake read failed from %s: %v", remote, err)
		_ = conn.Close()
		return
	}

	var hello executorEnvelope
	if err := json.Unmarshal(line, &hello); err != nil {
		h.rejectConnection(conn, fmt.Sprintf("invalid hello frame: %v", err))
		return
	}
	if hello.Type != "hello" || hello.Role != "executor" {
		h.rejectConnection(conn, fmt.Sprintf("expected hello from executor, got %s/%s", hello.Type, hello.Role))
		return
	}
	if !h.tokenAccepted(hello.Token, remote) {
		h.rejectConnection(conn, "invalid or missing executor token")
		return
	}

	client := &executorConn{
		conn:    conn,
		reader:  reader,
		remote:  remote,
		pending: map[string]chan executorResult{},
		closed:  make(chan struct{}),
	}

	h.mu.Lock()
	previous := h.client
	h.client = client
	h.mu.Unlock()

	h.statsMu.Lock()
	h.connected++
	h.lastRemote = remote
	h.lastSeen = time.Now()
	h.statsMu.Unlock()

	if previous != nil {
		log.Printf("Replacing previous executor client %s with %s", previous.remote, remote)
		previous.close()
	}

	ack, _ := json.Marshal(executorEnvelope{Type: "hello_ack", Role: "bridge", Version: bridgeProtocolVersion})
	if err := client.writeLine(ack); err != nil {
		log.Printf("Executor handshake ack failed for %s: %v", remote, err)
		client.close()
		return
	}

	log.Printf("Executor client connected from %s (protocol v%s)", remote, hello.Version)
	client.readLoop(h)
}

func (h *ExecutorHub) tokenAccepted(provided string, remote string) bool {
	if !h.tokenRequired() {
		return true
	}
	if subtle.ConstantTimeCompare([]byte(provided), []byte(h.token)) == 1 {
		return true
	}
	log.Printf("Executor at %s presented an invalid token", remote)
	return false
}

func (h *ExecutorHub) rejectConnection(conn net.Conn, reason string) {
	payload, _ := json.Marshal(executorEnvelope{Type: "hello_nack", Error: reason})
	_, _ = conn.Write(append(payload, '\n'))
	_ = conn.Close()
	log.Printf("Executor rejected: %s", reason)
}

// readLoop dispatches results for one connected executor until it disconnects.
func (c *executorConn) readLoop(h *ExecutorHub) {
	defer func() {
		c.close()
		h.mu.Lock()
		if h.client == c {
			h.client = nil
		}
		h.mu.Unlock()
		log.Printf("Executor client %s disconnected", c.remote)
	}()

	for {
		line, err := c.reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				log.Printf("Executor %s read error: %v", c.remote, err)
			}
			return
		}

		var env executorEnvelope
		if err := json.Unmarshal(line, &env); err != nil {
			log.Printf("Executor %s sent malformed frame: %v", c.remote, err)
			continue
		}

		switch env.Type {
		case "result":
			result := executorResult{}
			if !env.Success {
				result.err = fmt.Errorf("%s", env.Error)
			}
			response := &IPCResponse{Success: env.Success, Error: env.Error}
			if len(env.Data) > 0 {
				var data map[string]interface{}
				if err := json.Unmarshal(env.Data, &data); err == nil {
					response.Data = data
				}
			}
			result.response = response
			if !c.resolve(env.ID, result) {
				log.Printf("Executor %s returned result for unknown request %s", c.remote, env.ID)
			}
		case "pong":
			// Liveness only.
		case "event":
			log.Printf("Executor %s event: %s %s", c.remote, env.Event, string(env.Data))
		default:
			log.Printf("Executor %s sent unexpected frame type %q", c.remote, env.Type)
		}
	}
}

// SendCommand forwards one command to the connected executor and waits for its
// result. The executor session survives a timeout or a slow action.
func (h *ExecutorHub) SendCommand(cmd IPCCommand, timeout time.Duration) (*IPCResponse, error) {
	h.mu.Lock()
	client := h.client
	h.mu.Unlock()
	if client == nil {
		return nil, fmt.Errorf("no executor client connected")
	}

	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return nil, err
	}
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	env := executorEnvelope{Type: "command", ID: id, Command: cmdBytes}
	payload, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}

	ch := client.register(id)
	if err := client.writeLine(payload); err != nil {
		client.abandon(id)
		client.close()
		h.noteResult(err)
		return nil, fmt.Errorf("failed to send command to executor: %w", err)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case result := <-ch:
		if result.response == nil {
			h.noteResult(result.err)
			return nil, result.err
		}
		if !result.response.Success {
			h.noteResult(fmt.Errorf("%s", nonEmptyOr(result.response.Error, "command failed")))
			return result.response, nil
		}
		h.noteResult(nil)
		return result.response, nil
	case <-timer.C:
		client.abandon(id)
		err := fmt.Errorf("timeout after %s waiting for executor result (the action may still be running)", timeout)
		h.noteResult(err)
		return nil, err
	case <-client.closed:
		client.abandon(id)
		err := fmt.Errorf("executor disconnected while the command was running")
		h.noteResult(err)
		return nil, err
	}
}
