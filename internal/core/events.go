package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// SubscribeParams picks what to hear: everything, one terminal, a terminal's children or terminals by labels, and
// output watches (a core with the output_watches feature).
type SubscribeParams struct {
	Terminal   string            `json:"terminal,omitempty"`
	ChildrenOf string            `json:"children_of,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	Output     []OutputWatch     `json:"output,omitempty"`
}

// OutputWatch tells a row of a terminal's last Lines rows that matches it, as an `output` event.
type OutputWatch struct {
	ID       string `json:"id"`
	Terminal string `json:"terminal"`
	Match    Match  `json:"match"`
	Lines    int    `json:"lines,omitempty"`
}

// Match is a piece of text or a Rust regex (Type "substring" or "regex").
type Match struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// The kinds of notice a subscription hands out.
const (
	NoticeEvent     = "event"
	NoticeHeartbeat = "heartbeat"
	// NoticeMissed carries a fresh snapshot: the subscriber fell behind and events were dropped.
	NoticeMissed = "missed"
)

// Notice is one line of the event stream.
type Notice struct {
	Kind string
	Seq  uint64
	// Event is set for NoticeEvent.
	Event Event
	// Terminals is the fresh snapshot of NoticeMissed.
	Terminals []Terminal
}

// Event is one fact change; Data's shape is the type's (schema/README.md, "Events").
type Event struct {
	Type     string          `json:"type"`
	Terminal string          `json:"terminal"`
	Machine  string          `json:"machine"`
	Data     json.RawMessage `json:"data"`
}

// Subscription is one event stream: Seq and Terminals are the snapshot it started from, Next the changes since.
type Subscription struct {
	conn      *conn
	channel   uint32
	Seq       uint64
	Terminals []Terminal

	mu   sync.Mutex
	wake bell
	// partial is a line not ended yet, credited as it arrives (a missed snapshot may outgrow the window).
	partial         []byte
	partialCredited int
	lines           []line
	after           uint64
	err             error
}

// line is a whole line and the credit its consumption owes the core.
type line struct {
	text []byte
	owed int
}

// Subscribe starts an event stream.
func (c *Client) Subscribe(ctx context.Context, params SubscribeParams) (*Subscription, error) {
	cn, err := c.connection(ctx)
	if err != nil {
		return nil, err
	}
	return cn.subscribe(ctx, params)
}

// subscribe starts an event stream on this very connection.
func (cn *conn) subscribe(ctx context.Context, params SubscribeParams) (*Subscription, error) {
	if len(params.Output) > 0 && !cn.hello.Has("output_watches") {
		return nil, &Error{Kind: "needs_update", Message: "this remuda-core has no output watches"}
	}
	sub := &Subscription{conn: cn, wake: make(bell, 1)}
	var opened struct {
		Seq       uint64     `json:"seq"`
		Terminals []Terminal `json:"terminals"`
		Channel   uint32     `json:"channel"`
	}
	bind := func(result json.RawMessage) (func(), error) {
		if err := decode(result, &opened); err != nil {
			return nil, err
		}
		sub.channel, sub.Seq, sub.Terminals, sub.after = opened.Channel, opened.Seq, opened.Terminals, opened.Seq
		cn.subs[opened.Channel] = sub
		early := cn.orphans[opened.Channel]
		delete(cn.orphans, opened.Channel)
		return func() {
			for _, payload := range early {
				sub.take(frameData, payload)
			}
		}, nil
	}
	if err := cn.call(ctx, "events.subscribe", params, nil, bind); err != nil {
		return nil, err
	}
	return sub, nil
}

// Next waits for the next notice. Events the snapshot or a missed snapshot already covers are left out.
func (s *Subscription) Next(ctx context.Context) (Notice, error) {
	for {
		s.mu.Lock()
		if len(s.lines) > 0 {
			next := s.lines[0]
			s.lines = s.lines[1:]
			s.mu.Unlock()
			s.conn.credit(s.channel, next.owed)
			notice, err := parseNotice(next.text)
			if err != nil {
				return Notice{}, err
			}
			s.mu.Lock()
			stale := notice.Kind == NoticeEvent && notice.Seq <= s.after
			if notice.Kind == NoticeMissed {
				s.after = notice.Seq
			}
			s.mu.Unlock()
			if stale {
				continue
			}
			return notice, nil
		}
		err := s.err
		s.mu.Unlock()
		if err != nil {
			return Notice{}, err
		}
		select {
		case <-s.wake:
		case <-ctx.Done():
			return Notice{}, ctx.Err()
		}
	}
}

// Close ends the subscription; Next answers io.EOF.
func (s *Subscription) Close() error {
	s.mu.Lock()
	wasOpen := s.err == nil
	s.lines = nil
	s.mu.Unlock()
	s.end(io.EOF)
	if wasOpen {
		return s.conn.send(s.channel, frameClose, nil, false)
	}
	return nil
}

func parseNotice(text []byte) (Notice, error) {
	var message struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(text, &message); err != nil {
		return Notice{}, fmt.Errorf("remuda-core sent the event line %.120q: %w", text, err)
	}
	var params struct {
		Seq       uint64     `json:"seq"`
		Terminals []Terminal `json:"terminals"`
		Event
	}
	if err := decode(message.Params, &params); err != nil {
		return Notice{}, err
	}
	return Notice{Kind: message.Method, Seq: params.Seq, Event: params.Event, Terminals: params.Terminals}, nil
}

func (s *Subscription) take(kind byte, payload []byte) {
	switch kind {
	case frameData:
		s.conn.credit(s.channel, s.add(payload))
		s.wake.ring()
	case frameClose:
		s.end(io.EOF)
	}
}

// add splits what came into whole lines and answers the credit it can give at once: the bytes of a line not ended.
func (s *Subscription) add(payload []byte) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return 0
	}
	s.partial = append(s.partial, payload...)
	for {
		end := bytes.IndexByte(s.partial, '\n')
		if end < 0 {
			break
		}
		s.lines = append(s.lines, line{text: bytes.Clone(s.partial[:end]), owed: end + 1 - s.partialCredited})
		s.partialCredited = 0
		s.partial = s.partial[end+1:]
	}
	now := len(s.partial) - s.partialCredited
	s.partialCredited = len(s.partial)
	return now
}

func (s *Subscription) end(err error) {
	s.mu.Lock()
	if s.err == nil {
		s.err = err
	}
	s.mu.Unlock()
	s.wake.ring()
}
