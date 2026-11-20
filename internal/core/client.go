// Package core speaks remuda-core's protocol (its schema/README.md): one framed connection per core, calls on
// channel 0 side by side, an attach stream or an event subscription on a channel of its own.
package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
)

// Generation is the protocol generation this client speaks.
const Generation = 1

// Dialer opens a fresh byte stream to a core: its socket here, `ssh host remuda-core connect` over a master there.
type Dialer func(ctx context.Context) (net.Conn, error)

// Error is a core's refusal: a stable kind (`not_found`, `conflict`, `unavailable`, …) and a hint.
type Error struct {
	Code    int
	Kind    string
	Message string
	Hint    string
}

func (e *Error) Error() string {
	if e.Hint == "" {
		return e.Message
	}
	return e.Message + " (" + e.Hint + ")"
}

// IsKind says whether err is the core's refusal of that kind.
func IsKind(err error, kind string) bool {
	var refusal *Error
	return errors.As(err, &refusal) && refusal.Kind == kind
}

// TransportError is a failure to reach the core or a connection that ended: never the core's own answer.
type TransportError struct{ Err error }

func (e *TransportError) Error() string { return "remuda-core: " + e.Err.Error() }
func (e *TransportError) Unwrap() error { return e.Err }

// IsTransport says whether err is a failure to reach the core rather than its refusal.
func IsTransport(err error) bool {
	var transport *TransportError
	return errors.As(err, &transport)
}

// Hello is what the core said of itself when the connection opened.
type Hello struct {
	Generation int      `json:"generation"`
	Version    string   `json:"version"`
	Core       CoreInfo `json:"core"`
	Framed     bool     `json:"framed"`
	Methods    []string `json:"methods"`
	Features   []string `json:"features"`
	// KeepsRunningAfterLogout is nil where it does not apply, false on a Linux without lingering.
	KeepsRunningAfterLogout *bool `json:"keeps_running_after_logout"`
}

// CoreInfo is a core's permanent identity.
type CoreInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Has says whether the core advertises a feature (`keep_size`, `output_watches`, …).
func (h Hello) Has(feature string) bool {
	for _, have := range h.Features {
		if have == feature {
			return true
		}
	}
	return false
}

// Client is one core: a connection dialled when first needed and again after it ended.
type Client struct {
	dial   Dialer
	mu     sync.Mutex
	live   *conn
	closed bool
}

// New is a client that reaches its core through dial.
func New(dial Dialer) *Client {
	return &Client{dial: dial}
}

// Hello answers what the core said on the current connection, connecting first if there is none.
func (c *Client) Hello(ctx context.Context) (Hello, error) {
	cn, err := c.connection(ctx)
	if err != nil {
		return Hello{}, err
	}
	return cn.hello, nil
}

// Call sends one request on channel 0 and decodes its result into result (nil to drop it).
func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	cn, err := c.connection(ctx)
	if err != nil {
		return err
	}
	return cn.call(ctx, method, params, result, nil)
}

// Close ends the connection: calls in flight, streams and subscriptions end with a transport error.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.live != nil {
		c.live.fail(net.ErrClosed)
	}
	return nil
}

func (c *Client) connection(ctx context.Context) (*conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, &TransportError{net.ErrClosed}
	}
	if c.live != nil && !c.live.ended() {
		return c.live, nil
	}
	nc, err := c.dial(ctx)
	if err != nil {
		return nil, &TransportError{err}
	}
	cn, err := open(ctx, nc)
	if err != nil {
		nc.Close()
		return nil, err
	}
	c.live = cn
	return cn, nil
}

type request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      uint64 `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type response struct {
	ID     *uint64         `json:"id"`
	Method string          `json:"method"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Kind string `json:"kind"`
			Hint string `json:"hint"`
		} `json:"data"`
	} `json:"error"`
}

func (r response) err() error {
	if r.Error == nil {
		return nil
	}
	return &Error{Code: r.Error.Code, Kind: r.Error.Data.Kind, Message: r.Error.Message, Hint: r.Error.Data.Hint}
}

func decode(raw json.RawMessage, into any) error {
	if into == nil {
		return nil
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("remuda-core answered %.120s: %w", raw, err)
	}
	return nil
}
