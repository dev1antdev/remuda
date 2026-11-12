package herdr

// herdr's event stream (events.subscribe, protocol 22): one request on a connection that stays open, its subscriptions
// fixed for the connection's life — a second subscribe on it closes it, so a new set is a new connection. herdr answers
// subscription_started, then one event a line; a pane a subscription names that is gone refuses the whole request
// (pane_not_found), an output match whose regex herdr cannot read refuses it too (invalid_regex), and a reader too slow
// for a burst gets events_lost, or the connection closed under it.

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// Subscription is one entry of events.subscribe: a global kind ("pane.created"), or one bound to a pane
// ("pane.agent_status_changed", "pane.output_matched" with its match).
type Subscription struct {
	Type   string       `json:"type"`
	PaneID string       `json:"pane_id,omitempty"`
	Source string       `json:"source,omitempty"` // an output match's rows: recent (the screen and the scrollback above it)
	Lines  int          `json:"lines,omitempty"`  // how many of them herdr reads
	Match  *OutputMatch `json:"match,omitempty"`
}

// OutputMatch is what herdr looks for in a pane's rows: a substring, case and all, or a regex in Rust's dialect, read a
// line at a time.
type OutputMatch struct {
	Type  string `json:"type"` // substring | regex
	Value string `json:"value"`
}

// Event is one line of the stream, the fields remuda reads. herdr names the global kinds with an underscore
// ("pane_created") and the per-pane ones with a dot ("pane.agent_status_changed"); Kind is as herdr sent it.
type Event struct {
	Kind        string
	PaneID      string // the pane it concerns, when it names one
	AgentStatus string // pane.agent_status_changed
	MatchedLine string // pane.output_matched: the first row that matched
	Text        string // pane.output_matched: the rows herdr read
}

// Event kinds remuda tells apart.
const (
	EventAgentStatus   = "pane.agent_status_changed"
	EventOutputMatched = "pane.output_matched"
)

// ErrEventsLost is herdr saying the stream fell behind and dropped events.
const ErrEventsLost = "events_lost"

// Stream is an open subscription; Next reads it, Close ends it.
type Stream struct {
	conn   net.Conn
	reader *bufio.Reader
	onLost func(error) // the client's transport hook, for a stream that broke rather than one closed here

	mu     sync.Mutex
	closed bool
}

// maxEventLine is the longest line a stream reads: an output match carries the rows herdr read, a few KB.
const maxEventLine = 16 << 20

// Subscribe opens a stream with the subscriptions given, answering once herdr said subscription_started.
// A refusal is herdr's *Error; a failure to reach herdr goes to OnTransportError as a call's would.
func (c *Client) Subscribe(subs []Subscription) (*Stream, error) {
	s, err := c.subscribe(subs)
	var he *Error
	if err != nil && !errors.As(err, &he) && c.OnTransportError != nil {
		c.OnTransportError(err)
	}
	return s, err
}

func (c *Client) subscribe(subs []Subscription) (*Stream, error) {
	if subs == nil {
		subs = []Subscription{}
	}
	conn, err := c.dial(c.Timeout)
	if err != nil {
		return nil, err
	}
	conn.SetDeadline(time.Now().Add(c.Timeout))
	req, err := json.Marshal(map[string]any{
		"id":     fmt.Sprintf("remuda-%d", c.reqSeq.Add(1)),
		"method": "events.subscribe",
		"params": map[string]any{"subscriptions": subs},
	})
	if err != nil {
		conn.Close()
		return nil, err
	}
	if _, err := conn.Write(append(req, '\n')); err != nil {
		conn.Close()
		return nil, err
	}
	s := &Stream{conn: conn, reader: bufio.NewReaderSize(conn, 64*1024), onLost: c.OnTransportError}
	line, err := s.line()
	if err != nil {
		conn.Close()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("herdr closed the connection before answering events.subscribe")
		}
		return nil, err
	}
	var head struct {
		Result *struct {
			Type string `json:"type"`
		} `json:"result"`
		Error *Error `json:"error"`
	}
	if err := json.Unmarshal(line, &head); err != nil {
		conn.Close()
		return nil, fmt.Errorf("herdr sent unparseable frame: %.200s", line)
	}
	if head.Error != nil {
		conn.Close()
		return nil, head.Error
	}
	if head.Result == nil || head.Result.Type != "subscription_started" {
		conn.Close()
		return nil, fmt.Errorf("herdr answered events.subscribe with %.200s", line)
	}
	conn.SetDeadline(time.Time{})
	return s, nil
}

// line reads one newline-ended line, refusing one past maxEventLine.
func (s *Stream) line() ([]byte, error) {
	var out []byte
	for {
		chunk, err := s.reader.ReadSlice('\n')
		out = append(out, chunk...)
		if len(out) > maxEventLine {
			return nil, fmt.Errorf("herdr sent an event line over %d bytes", maxEventLine)
		}
		if err == nil {
			return out, nil
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return nil, err
		}
	}
}

// Next reads the next event. herdr's own refusal mid-stream (events_lost) is an *Error; a stream that broke or ended
// under it is a transport error, told to the client's hook (a remote checks its master); one closed here answers
// net.ErrClosed.
func (s *Stream) Next() (Event, error) {
	for {
		line, err := s.line()
		if err != nil {
			if s.isClosed() {
				return Event{}, net.ErrClosed
			}
			if errors.Is(err, io.EOF) {
				err = fmt.Errorf("herdr closed the event stream")
			}
			if s.onLost != nil {
				s.onLost(err)
			}
			return Event{}, err
		}
		if len(line) <= 1 {
			continue
		}
		event, err := ParseEvent(line)
		if err != nil {
			return Event{}, err
		}
		if event.Kind == "" {
			continue // a frame of no kind remuda knows: nothing to say
		}
		return event, nil
	}
}

// ParseEvent reads one stream line: an event, or herdr's error frame as an *Error.
func ParseEvent(line []byte) (Event, error) {
	var frame struct {
		Event string `json:"event"`
		Error *Error `json:"error"`
		Data  struct {
			PaneID      string `json:"pane_id"`
			AgentStatus string `json:"agent_status"`
			MatchedLine string `json:"matched_line"`
			Read        struct {
				Text string `json:"text"`
			} `json:"read"`
			Pane struct {
				PaneID string `json:"pane_id"`
			} `json:"pane"`
		} `json:"data"`
	}
	if err := json.Unmarshal(line, &frame); err != nil {
		return Event{}, fmt.Errorf("herdr sent unparseable event: %.200s", line)
	}
	if frame.Error != nil {
		return Event{}, frame.Error
	}
	pane := frame.Data.PaneID
	if pane == "" {
		pane = frame.Data.Pane.PaneID
	}
	return Event{Kind: frame.Event, PaneID: pane, AgentStatus: frame.Data.AgentStatus, MatchedLine: frame.Data.MatchedLine, Text: frame.Data.Read.Text}, nil
}

func (s *Stream) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// Close ends the stream; a Next waiting on it returns net.ErrClosed.
func (s *Stream) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	return s.conn.Close()
}

// Bridged says the client reaches herdr through one command per connection (`nc -U` over ssh) rather than a socket:
// a session of the ssh master each, so a stream holds one and never two.
func (c *Client) Bridged() bool { return c.Dial != nil }
