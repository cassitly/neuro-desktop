package neuro

import (
	"encoding/json"
	"errors"
	"net/url"
	"sync"

	"github.com/gorilla/websocket"
)

// Message is the generic envelope for Neuro messages.
type Message struct {
	Command string          `json:"command"`
	Game    string          `json:"game,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// ActionData is the data for Neuro’s action message.
type ActionData struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Data json.RawMessage `json:"data,omitempty"`
}

type Action struct {
	ID   string
	Name string
	Data []byte
}

// Client wraps the websocket to Neuro.
type Client struct {
	conn        *websocket.Conn
	game        string
	sendMut     sync.Mutex
	recvMut     sync.Mutex
	ActionChan  chan Action
	ErrChan     chan error
	closed      bool
	closeSignal chan struct{}
}

func NewClient(game, wsURL string) (*Client, error) {
	u, err := url.Parse(wsURL)
	if err != nil {
		return nil, err
	}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, err
	}

	c := &Client{
		conn:        conn,
		game:        game,
		ActionChan:  make(chan Action, 8),
		ErrChan:     make(chan error, 1),
		closeSignal: make(chan struct{}),
	}

	go c.reader()
	return c, nil
}

func (c *Client) reader() {
	for {
		select {
		case <-c.closeSignal:
			return
		default:
			_, msgBytes, err := c.conn.ReadMessage()
			if err != nil {
				c.ErrChan <- err
				return
			}

			var m Message
			err = json.Unmarshal(msgBytes, &m)
			if err != nil {
				c.ErrChan <- err
				continue
			}

			if m.Command == "action" {
				var ad ActionData
				json.Unmarshal(m.Data, &ad)
				c.ActionChan <- Action{
					ID:   ad.ID,
					Name: ad.Name,
					Data: ad.Data,
				}
			}
		}
	}
}

func (c *Client) send(msg any) error {
	c.sendMut.Lock()
	defer c.sendMut.Unlock()

	if c.closed {
		return errors.New("connection closed")
	}
	return c.conn.WriteJSON(msg)
}

func (c *Client) Startup() error {
	return c.send(Message{Command: "startup", Game: c.game})
}

func (c *Client) SendContext(message string, silent bool) error {
	data := struct {
		Message string `json:"message"`
		Silent  bool   `json:"silent"`
	}{message, silent}

	return c.send(Message{Command: "context", Game: c.game, Data: toRawMessage(data)})
}

func (c *Client) RegisterActions(actions []map[string]interface{}) error {
	data := struct {
		Actions []map[string]interface{} `json:"actions"`
	}{actions}
	return c.send(Message{"actions/register", c.game, toRawMessage(data)})
}

func (c *Client) UnregisterActions(names []string) error {
	data := struct {
		Names []string `json:"action_names"`
	}{names}

	return c.send(Message{"actions/unregister", c.game, toRawMessage(data)})
}

func (c *Client) ForceActions(state, query string, names []string) error {
	data := struct {
		State     string   `json:"state,omitempty"`
		Query     string   `json:"query"`
		Ephemeral bool     `json:"ephemeral_context,omitempty"`
		Names     []string `json:"action_names"`
	}{state, query, true, names}

	return c.send(Message{"actions/force", c.game, toRawMessage(data)})
}

func (c *Client) SendActionResult(id string, success bool, message string) error {
	data := struct {
		ID      string `json:"id"`
		Success bool   `json:"success"`
		Message string `json:"message,omitempty"`
	}{id, success, message}

	return c.send(Message{"action/result", c.game, toRawMessage(data)})
}

func toRawMessage(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return json.RawMessage(b)
}

func (c *Client) Close() error {
	close(c.closeSignal)
	c.closed = true
	return c.conn.Close()
}
