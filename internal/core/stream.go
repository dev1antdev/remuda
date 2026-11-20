package core

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"sync"
)

// AttachParams asks for a terminal's stream. KeepSize needs a core with the keep_size feature.
type AttachParams struct {
	Terminal string `json:"terminal"`
	Cols     uint16 `json:"cols"`
	Rows     uint16 `json:"rows"`
	// Observe never types or resizes; KeepSize types but leaves the size to the other viewers.
	Observe  bool `json:"observe,omitempty"`
	KeepSize bool `json:"keep_size,omitempty"`
	// ResizeNotifications asks an observer or a keep-size viewer to be told the grid before each repaint.
	ResizeNotifications bool `json:"resize_notifications,omitempty"`
	History             int  `json:"history,omitempty"`
	// Resumed: back after a dropped line, it takes the size only once active again.
	Resumed bool `json:"resumed,omitempty"`
	// Resized hears the grid a RESIZE frame announces, in order with the bytes Read hands out.
	Resized func(cols, rows uint16) `json:"-"`
}

// Ended is why the core closed a stream: `exited` with its code, `killed`, `server_stop`, or an error.
type Ended struct {
	Reason   string `json:"reason"`
	ExitCode *int   `json:"exit_code"`
	Signal   *int   `json:"signal"`
	Error    string `json:"error"`
}

// Stream is one attach: Read the program's output (the first paint, then live bytes), Write keys.
type Stream struct {
	conn     *conn
	channel  uint32
	Terminal Terminal
	// ResizeNotifications: the core confirmed RESIZE frames before each repaint; false, it sends none and the size
	// is to be followed through `size` events.
	ResizeNotifications bool
	// AutomaticInput: every core on the path takes a terminal's own replies as kind 6 rather than as typing.
	AutomaticInput bool
	resized        func(cols, rows uint16)

	mu     sync.Mutex
	wake   bell
	queue  []piece
	err    error
	closed *Ended
}

type piece struct {
	data       []byte
	cols, rows uint16
	resize     bool
}

// Attach opens a stream on a terminal.
func (c *Client) Attach(ctx context.Context, params AttachParams) (*Stream, error) {
	cn, err := c.connection(ctx)
	if err != nil {
		return nil, err
	}
	if params.KeepSize && !cn.hello.Has("keep_size") {
		return nil, &Error{Kind: "needs_update", Message: "this remuda-core cannot attach without taking the size"}
	}
	stream := &Stream{conn: cn, resized: params.Resized, wake: make(bell, 1)}
	var opened struct {
		Channel             uint32   `json:"channel"`
		Terminal            Terminal `json:"terminal"`
		ResizeNotifications bool     `json:"resize_notifications"`
		AutomaticInput      bool     `json:"automatic_input"`
	}
	bind := func(result json.RawMessage) (func(), error) {
		if err := decode(result, &opened); err != nil {
			return nil, err
		}
		stream.channel, stream.Terminal = opened.Channel, opened.Terminal
		stream.ResizeNotifications, stream.AutomaticInput = opened.ResizeNotifications, opened.AutomaticInput
		cn.streams[opened.Channel] = stream
		return nil, nil
	}
	if err := cn.call(ctx, "terminal.attach", params, nil, bind); err != nil {
		return nil, err
	}
	return stream, nil
}

// Read hands out the program's bytes; io.EOF once the stream closed (Ended says why), a transport error if the
// connection went.
func (s *Stream) Read(p []byte) (int, error) {
	for {
		s.mu.Lock()
		if len(s.queue) > 0 {
			next := &s.queue[0]
			if next.resize {
				cols, rows := next.cols, next.rows
				s.queue = s.queue[1:]
				s.mu.Unlock()
				if s.resized != nil {
					s.resized(cols, rows)
				}
				continue
			}
			n := copy(p, next.data)
			next.data = next.data[n:]
			if len(next.data) == 0 {
				s.queue = s.queue[1:]
			}
			s.mu.Unlock()
			s.conn.credit(s.channel, n)
			return n, nil
		}
		err := s.err
		s.mu.Unlock()
		if err != nil {
			return 0, err
		}
		<-s.wake
	}
}

// Write sends keys and pasted text to the program.
func (s *Stream) Write(p []byte) (int, error) {
	if err := s.open(); err != nil {
		return 0, err
	}
	for sent := 0; sent < len(p); {
		chunk := min(len(p)-sent, 1<<20)
		if err := s.conn.write(s.channel, frameData, p[sent:sent+chunk]); err != nil {
			return sent, err
		}
		sent += chunk
	}
	return len(p), nil
}

// Resize tells the core this viewer's size.
func (s *Stream) Resize(cols, rows uint16) error {
	if err := s.open(); err != nil {
		return err
	}
	var size [4]byte
	binary.LittleEndian.PutUint16(size[0:2], cols)
	binary.LittleEndian.PutUint16(size[2:4], rows)
	return s.conn.write(s.channel, frameResize, size[:])
}

// Focus tells the core this viewer gained or lost the focus.
func (s *Stream) Focus(focused bool) error {
	if err := s.open(); err != nil {
		return err
	}
	flag := []byte{0}
	if focused {
		flag[0] = 1
	}
	return s.conn.write(s.channel, frameFocus, flag)
}

// Close detaches: what is still queued is dropped and Read answers io.EOF.
func (s *Stream) Close() error {
	s.mu.Lock()
	wasOpen := s.err == nil
	s.mu.Unlock()
	s.end(io.EOF, nil)
	if wasOpen {
		return s.conn.write(s.channel, frameClose, nil)
	}
	return nil
}

// Ended says why the core closed the stream, nil while it is open or after a Close of this side.
func (s *Stream) Ended() *Ended {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *Stream) open() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return errors.Join(errClosedStream, s.err)
	}
	return nil
}

var errClosedStream = errors.New("the stream is closed")

func (s *Stream) take(kind byte, payload []byte) {
	switch kind {
	case frameData:
		s.push(piece{data: payload})
	case frameResize:
		if len(payload) == 4 {
			s.push(piece{resize: true, cols: binary.LittleEndian.Uint16(payload[0:2]), rows: binary.LittleEndian.Uint16(payload[2:4])})
		}
	case frameClose:
		var why Ended
		if json.Unmarshal(payload, &why) != nil {
			why = Ended{}
		}
		s.end(io.EOF, &why)
	}
}

func (s *Stream) push(p piece) {
	s.mu.Lock()
	if s.err == nil {
		s.queue = append(s.queue, p)
	}
	s.mu.Unlock()
	s.wake.ring()
}

// end stops the stream: a close by the core keeps the queued bytes for Read, a close of this side or a lost
// connection drops them.
func (s *Stream) end(err error, why *Ended) {
	s.mu.Lock()
	if s.err == nil {
		s.err = err
		s.closed = why
		if why == nil {
			s.queue = nil
		}
	}
	s.mu.Unlock()
	s.wake.ring()
}

// bell wakes the one goroutine waiting on it; a ring nobody waits for stays until it is heard.
type bell chan struct{}

func (b bell) ring() {
	select {
	case b <- struct{}{}:
	default:
	}
}
