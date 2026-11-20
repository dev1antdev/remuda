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
	"slices"
	"sync"
	"sync/atomic"
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
	// helloWithin bounds a connection's dial and opening.
	helloWithin = 15 * time.Second
)

// writeWithin bounds one frame's write: a core that stops reading ends the connection rather than every caller.
var writeWithin = 30 * time.Second

// conn is one framed connection: one goroutine reads every frame, one writes what the others queue, and nobody else
// touches the wire, so a caller only ever waits for its answer or its context.
type conn struct {
	nc     net.Conn
	reader *bufio.Reader
	hello  Hello
	// writeWithin is the package's, as it was when the connection opened.
	writeWithin time.Duration

	nextID atomic.Uint64

	mu      sync.Mutex
	pending map[uint64]*pending
	streams map[uint32]*Stream
	subs    map[uint32]*Subscription
	// orphans holds event frames that came before the reply naming their channel: a subscription's channel is
	// open from the moment the core takes the call.
	orphans     map[uint32][][]byte
	subscribing int
	// queue is what the writer sends in order; credits, owed per channel, go before it.
	queue   []*outgoing
	credits map[uint32]int
	pulse   bell
	done    chan struct{}
	err     error
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

// open says hello in plain mode, switches the connection to frames and starts its reader and writer. Ending ctx
// abandons the hello.
func open(ctx context.Context, nc net.Conn) (*conn, error) {
	stop := context.AfterFunc(ctx, func() { nc.SetDeadline(time.Unix(1, 0)) })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		nc.SetDeadline(deadline)
	}
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
	if !stop() {
		return nil, &TransportError{ctx.Err()}
	}
	nc.SetDeadline(time.Time{})
	c := &conn{
		nc: nc, reader: reader, hello: hello, writeWithin: writeWithin,
		pending: map[uint64]*pending{}, streams: map[uint32]*Stream{}, subs: map[uint32]*Subscription{},
		orphans: map[uint32][][]byte{}, credits: map[uint32]int{}, pulse: make(bell, 1), done: make(chan struct{}),
	}
	go c.read()
	go c.write()
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

// call queues one request and waits for its answer or ctx; bind, when set, opens the channel the answer names. A
// request ctx ended before the writer took it is never sent; one already on its way is followed by a cancel.
func (c *conn) call(ctx context.Context, method string, params, result any, bind func(json.RawMessage) (func(), error)) error {
	if params == nil {
		params = struct{}{}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	id := c.nextID.Add(1)
	body, err := json.Marshal(request{JSONRPC: "2.0", ID: id, Method: method, Params: params}) // outside the lock: params may be anyone's Marshaler
	if err != nil {
		return err
	}
	c.mu.Lock()
	if err := ctx.Err(); err != nil {
		c.mu.Unlock()
		return err
	}
	if c.err != nil {
		c.mu.Unlock()
		return c.err
	}
	wait := &pending{answer: make(chan answer, 1), bind: bind}
	c.pending[id] = wait
	if method == "events.subscribe" {
		c.subscribing++
		defer c.subscribed()
	}
	out := c.queueLocked(0, frameData, body, nil)
	out.ctx = ctx
	c.mu.Unlock()
	var got answer
	select {
	case got = <-wait.answer:
	case <-ctx.Done():
		if c.abandon(id, out) {
			return ctx.Err()
		}
		got = <-wait.answer // the answer won: it is taken, its channel open, and on its way
	}
	if got.err != nil {
		return got.err
	}
	return decode(got.result, result)
}

// abandon takes a call back unless its answer was taken already, saying whether it did: whichever of the two takes
// the call from pending settles it. Its request leaves the queue unsent, or is cancelled once sent.
func (c *conn) abandon(id uint64, out *outgoing) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, waiting := c.pending[id]; !waiting {
		return false
	}
	delete(c.pending, id)
	if out.taken {
		cancel, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "$/cancelRequest", "params": map[string]any{"id": id}})
		c.queueLocked(0, frameData, cancel, nil)
	} else {
		c.queue = slices.DeleteFunc(c.queue, func(o *outgoing) bool { return o == out })
	}
	return true
}

func (c *conn) subscribed() {
	c.mu.Lock()
	c.subscribing--
	if c.subscribing == 0 {
		clear(c.orphans)
	}
	c.mu.Unlock()
}

// fail ends the connection once and answers why: every call waiting, write queued, stream and subscription hears it.
func (c *conn) fail(cause error) error {
	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return err
	}
	c.err = &TransportError{cause}
	close(c.done)
	waiting, streams, subs, queued := c.pending, c.streams, c.subs, c.queue
	c.pending, c.streams, c.subs, c.queue = map[uint64]*pending{}, map[uint32]*Stream{}, map[uint32]*Subscription{}, nil
	c.mu.Unlock()
	c.nc.Close()
	for _, wait := range waiting {
		wait.answer <- answer{err: c.err}
	}
	for _, out := range queued {
		if out.written != nil {
			out.written <- c.err
		}
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
	if wait == nil {
		c.closeAbandonedLocked(reply.Result)
	}
	c.mu.Unlock()
	if after != nil {
		after()
	}
	if wait != nil {
		wait.answer <- got
	}
}

// closeAbandonedLocked ends a channel opened by a call nobody waits for any more: the core keeps it for the
// connection.
func (c *conn) closeAbandonedLocked(result json.RawMessage) {
	var opened struct {
		Channel uint32 `json:"channel"`
	}
	if json.Unmarshal(result, &opened) == nil && opened.Channel != 0 && c.err == nil {
		c.queueLocked(opened.Channel, frameClose, nil, nil)
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
