package codexapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// Envelope is one app-server JSON-RPC message or notification.
type Envelope struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Client owns a single app-server connection. Its reader dispatches
// replies independently of notifications, so a pending question does not
// stall another RPC or notification.
type Client struct {
	socket        *codexSocket
	mu            sync.Mutex
	next          uint64
	pending       map[string]chan Envelope
	notifications chan Envelope
	done          chan struct{}
	closing       chan struct{}
	closeOnce     sync.Once
	err           error
}

// Open connects to and initializes one app-server session.
func Open(ctx context.Context, endpoint string) (*Client, error) {
	socket, err := dialCodexSocket(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	c := &Client{socket: socket, pending: make(map[string]chan Envelope), notifications: make(chan Envelope, 128), done: make(chan struct{}), closing: make(chan struct{})}
	go c.read()
	var result struct {
		UserAgent  string `json:"userAgent"`
		CodexHome  string `json:"codexHome"`
		PlatformOS string `json:"platformOs"`
	}
	if err := c.Call(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "steerline_worker", "version": "1"}, "capabilities": map[string]bool{"experimentalApi": true}}, &result); err != nil {
		c.Close()
		return nil, err
	}
	if result.UserAgent == "" || result.CodexHome == "" || result.PlatformOS == "" {
		c.Close()
		return nil, errors.New("Codex app-server omitted its identity")
	}
	if err := c.socket.Send(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func (c *Client) Close() {
	c.closeOnce.Do(func() { close(c.closing); _ = c.socket.Close() })
	<-c.done
}

// Notifications returns the client's read-only notification stream.
func (c *Client) Notifications() <-chan Envelope { return c.notifications }

func (c *Client) read() {
	defer close(c.done)
	defer close(c.notifications)
	for {
		line, err := c.socket.Receive()
		if err != nil {
			c.mu.Lock()
			c.err = err
			c.mu.Unlock()
			return
		}
		var envelope Envelope
		if err := json.Unmarshal(line, &envelope); err != nil {
			c.mu.Lock()
			c.err = err
			c.mu.Unlock()
			return
		}
		if len(envelope.ID) != 0 && envelope.Method == "" {
			c.mu.Lock()
			ch := c.pending[string(envelope.ID)]
			delete(c.pending, string(envelope.ID))
			c.mu.Unlock()
			if ch != nil {
				ch <- envelope
			}
			continue
		}
		select {
		case c.notifications <- envelope:
		case <-c.closing:
			return
		}
	}
}

func (c *Client) Call(ctx context.Context, method string, params any, result any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Cancellation must interrupt a blocked socket write as well as reply wait.
	stop := context.AfterFunc(ctx, c.Close)
	defer stop()
	c.mu.Lock()
	c.next++
	id := c.next
	key := fmt.Sprint(id)
	reply := make(chan Envelope, 1)
	c.pending[key] = reply
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.pending, key); c.mu.Unlock() }()
	if err := c.socket.Send(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		c.mu.Lock()
		err := c.err
		c.mu.Unlock()
		return fmt.Errorf("Codex app-server connection ended during %s: %w", method, err)
	case envelope := <-reply:
		if envelope.Error != nil {
			return fmt.Errorf("Codex %s (%d): %s", method, envelope.Error.Code, envelope.Error.Message)
		}
		if len(envelope.Result) == 0 {
			return fmt.Errorf("Codex %s omitted result", method)
		}
		if result != nil {
			return json.Unmarshal(envelope.Result, result)
		}
		return nil
	}
}

func (c *Client) ReplyRequest(id json.RawMessage, result any) error {
	if len(id) == 0 || !json.Valid(id) {
		return errors.New("Codex request has no valid JSON-RPC identity")
	}
	return c.socket.Send(struct {
		ID     json.RawMessage `json:"id"`
		Result any             `json:"result"`
	}{id, result})
}

func (c *Client) RejectRequest(id json.RawMessage, reason string) error {
	if len(id) == 0 || !json.Valid(id) {
		return errors.New("Codex request has no valid JSON-RPC identity")
	}
	return c.socket.Send(struct {
		ID    json.RawMessage `json:"id"`
		Error any             `json:"error"`
	}{id, map[string]any{"code": -32601, "message": reason}})
}
