package core

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

const (
	frameData   = 0
	frameCredit = 1
	frameClose  = 2
	frameResize = 3
	frameFocus  = 4

	headerSize = 9
	maxPayload = 16 << 20
	// helloWithin bounds a connection's opening when the caller's context sets no deadline.
	helloWithin = 15 * time.Second
)

// writeWithin bounds one frame's write: a core that stops reading ends the connection rather than every caller.
var writeWithin = 30 * time.Second

// conn is one framed connection: one goroutine reads every frame, writers take turns.
type conn struct {
	nc     net.Conn
	reader *bufio.Reader
	hello  Hello
	// writing is held by the one write under way; waiting for it ends with the caller's context.
	writing chan struct{}

	mu      sync.Mutex
	nextID  uint64
	pending map[uint64]*pending
	streams map[uint32]*Stream
	subs    map[uint32]*Subscription
	// orphans holds event frames that came before the reply naming their channel: a subscription's channel is
	// open from the moment the core takes the call.
	orphans     map[uint32][][]byte
	subscribing int
	done        chan struct{}
	err         error
}

type pending struct {
	answer chan answer
	// bind opens the channel a reply names, on the reading goroutine before it reads another frame, under conn.mu;
	// what it returns runs right after, outside the lock.
	bind func(result json.RawMessage) (after func(), err error)
}

type answer struct {
	result json.RawMessage
	err    error
}

// open says hello in plain mode, switches the connection to frames and starts reading them.
func open(ctx context.Context, nc net.Conn) (*conn, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(helloWithin)
	}
	nc.SetDeadline(deadline)
	line, _ := json.Marshal(request{JSONRPC: "2.0", ID: 0, Method: "hello", Params: map[string]any{"generation": Generation, "framed": true}})
	if _, err := nc.Write(append(line, '\n')); err != nil {
		return nil, &TransportError{err}
	}
	reader := bufio.NewReaderSize(nc, 64<<10)
	said, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, &TransportError{fmt.Errorf("no answer to hello: %w", err)}
	}
	var reply response
	if err := json.Unmarshal(said, &reply); err != nil {
		return nil, &TransportError{fmt.Errorf("hello answered %.120q", said)}
	}
	if err := reply.err(); err != nil {
		return nil, err
	}
	var hello Hello
	if err := decode(reply.Result, &hello); err != nil {
		return nil, &TransportError{err}
	}
	if !hello.Framed {
		return nil, &TransportError{errors.New("the core did not switch to framed mode")}
	}
	nc.SetDeadline(time.Time{})
	c := &conn{
		nc: nc, reader: reader, hello: hello, nextID: 1, writing: make(chan struct{}, 1),
		pending: map[uint64]*pending{}, streams: map[uint32]*Stream{}, subs: map[uint32]*Subscription{},
		orphans: map[uint32][][]byte{}, done: make(chan struct{}),
	}
	go c.read()
	return c, nil
}

func (c *conn) ended() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

// call sends one request and waits for its answer; bind, when set, opens the channel the answer names.
func (c *conn) call(ctx context.Context, method string, params, result any, bind func(json.RawMessage) (func(), error)) error {
	if params == nil {
		params = struct{}{}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	if c.err != nil {
		c.mu.Unlock()
		return c.err
	}
	id := c.nextID
	c.nextID++
	wait := &pending{answer: make(chan answer, 1), bind: bind}
	c.pending[id] = wait
	if method == "events.subscribe" {
		c.subscribing++
		defer c.subscribed()
	}
	c.mu.Unlock()
	body, err := json.Marshal(request{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	if err == nil {
		err = c.write(ctx, 0, frameData, body)
	}
	if err != nil {
		c.forget(id)
		return err
	}
	var got answer
	select {
	case got = <-wait.answer:
	case <-ctx.Done():
		if c.forget(id) {
			cancel, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "$/cancelRequest", "params": map[string]any{"id": id}})
			go c.write(context.Background(), 0, frameData, cancel)
			return ctx.Err()
		}
		got = <-wait.answer // the answer won: it is taken, its channel open, and on its way
	}
	if got.err != nil {
		return got.err
	}
	return decode(got.result, result)
}

// forget takes a call back unless its answer was taken already, saying whether it did: whichever of the two takes
// the call from pending settles it.
func (c *conn) forget(id uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, waiting := c.pending[id]
	delete(c.pending, id)
	return waiting
}

func (c *conn) subscribed() {
	c.mu.Lock()
	c.subscribing--
	if c.subscribing == 0 {
		clear(c.orphans)
	}
	c.mu.Unlock()
}

// write sends one frame whole once the write before it is done, or nothing if ctx ends first. A write that fails or
// stalls past writeWithin may have left half a frame: it ends the connection.
func (c *conn) write(ctx context.Context, channel uint32, kind byte, payload []byte) error {
	select {
	case c.writing <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return c.failure()
	}
	defer func() { <-c.writing }()
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.ended() {
		return c.failure()
	}
	frame := make([]byte, headerSize+len(payload))
	binary.LittleEndian.PutUint32(frame[0:4], channel)
	frame[4] = kind
	binary.LittleEndian.PutUint32(frame[5:9], uint32(len(payload)))
	copy(frame[headerSize:], payload)
	c.nc.SetWriteDeadline(time.Now().Add(writeWithin))
	_, err := c.nc.Write(frame)
	if err != nil {
		return c.fail(fmt.Errorf("a write failed or stalled: %w", err))
	}
	c.nc.SetWriteDeadline(time.Time{}) // an ssh exec's conn ends at any deadline left standing
	return nil
}

func (c *conn) failure() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (c *conn) credit(channel uint32, bytes int) {
	if bytes <= 0 {
		return
	}
	var amount [4]byte
	binary.LittleEndian.PutUint32(amount[:], uint32(bytes))
	c.write(context.Background(), channel, frameCredit, amount[:])
}

// fail ends the connection once and answers why: every call waiting, stream and subscription hears it.
func (c *conn) fail(cause error) error {
	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return err
	}
	c.err = &TransportError{cause}
	close(c.done)
	waiting, streams, subs := c.pending, c.streams, c.subs
	c.pending, c.streams, c.subs = map[uint64]*pending{}, map[uint32]*Stream{}, map[uint32]*Subscription{}
	c.mu.Unlock()
	c.nc.Close()
	for _, wait := range waiting {
		wait.answer <- answer{err: c.err}
	}
	for _, stream := range streams {
		stream.end(c.err, nil)
	}
	for _, sub := range subs {
		sub.end(c.err)
	}
	return c.err
}

func (c *conn) read() {
	var header [headerSize]byte
	for {
		if _, err := io.ReadFull(c.reader, header[:]); err != nil {
			c.fail(fmt.Errorf("the connection ended: %w", err))
			return
		}
		channel := binary.LittleEndian.Uint32(header[0:4])
		kind := header[4]
		size := binary.LittleEndian.Uint32(header[5:9])
		if size > maxPayload {
			c.fail(fmt.Errorf("a frame of %d bytes", size))
			return
		}
		payload := make([]byte, size)
		if _, err := io.ReadFull(c.reader, payload); err != nil {
			c.fail(fmt.Errorf("the connection ended mid-frame: %w", err))
			return
		}
		if channel == 0 {
			if kind == frameData {
				c.reply(payload)
			}
			continue
		}
		c.route(channel, kind, payload)
	}
}

// reply hands an answer to its caller, opening the channel it names first.
func (c *conn) reply(payload []byte) {
	var reply response
	if json.Unmarshal(payload, &reply) != nil || reply.Method != "" || reply.ID == nil {
		return
	}
	got := answer{result: reply.Result, err: reply.err()}
	var after func()
	c.mu.Lock()
	wait := c.pending[*reply.ID]
	delete(c.pending, *reply.ID)
	if wait != nil && wait.bind != nil && got.err == nil {
		after, got.err = wait.bind(reply.Result)
	}
	c.mu.Unlock()
	if after != nil {
		after()
	}
	if wait == nil {
		c.closeAbandoned(reply.Result)
		return
	}
	wait.answer <- got
}

// closeAbandoned ends a channel opened by a call nobody waits for any more: the core keeps it for the connection.
func (c *conn) closeAbandoned(result json.RawMessage) {
	var opened struct {
		Channel uint32 `json:"channel"`
	}
	if json.Unmarshal(result, &opened) == nil && opened.Channel != 0 {
		go c.write(context.Background(), opened.Channel, frameClose, nil)
	}
}

func (c *conn) route(channel uint32, kind byte, payload []byte) {
	c.mu.Lock()
	stream, sub := c.streams[channel], c.subs[channel]
	if kind == frameClose {
		delete(c.streams, channel)
		delete(c.subs, channel)
	}
	if stream == nil && sub == nil && c.subscribing > 0 && kind == frameData {
		c.orphans[channel] = append(c.orphans[channel], payload)
	}
	c.mu.Unlock()
	switch {
	case stream != nil:
		stream.take(kind, payload)
	case sub != nil:
		sub.take(kind, payload)
	}
}
