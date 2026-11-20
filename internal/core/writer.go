package core

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"
)

// outgoing is one frame waiting for the writer; written, when set, hears how its write went. A request's frame
// carries its call's context: the writer drops it unsent once that context ended.
type outgoing struct {
	frame   []byte
	ctx     context.Context
	taken   bool
	written chan error
}

// send queues one frame for the writer; with wait, it answers once the frame is written or the connection ended.
func (c *conn) send(channel uint32, kind byte, payload []byte, wait bool) error {
	var written chan error
	if wait {
		written = make(chan error, 1)
	}
	c.mu.Lock()
	if c.err != nil {
		c.mu.Unlock()
		return c.err
	}
	c.queueLocked(channel, kind, payload, written)
	c.mu.Unlock()
	if !wait {
		return nil
	}
	return <-written
}

func (c *conn) queueLocked(channel uint32, kind byte, payload []byte, written chan error) *outgoing {
	out := &outgoing{frame: frameOf(channel, kind, payload), written: written}
	c.queue = append(c.queue, out)
	c.pulse.ring()
	return out
}

func frameOf(channel uint32, kind byte, payload []byte) []byte {
	frame := make([]byte, headerSize+len(payload))
	binary.LittleEndian.PutUint32(frame[0:4], channel)
	frame[4] = kind
	binary.LittleEndian.PutUint32(frame[5:9], uint32(len(payload)))
	copy(frame[headerSize:], payload)
	return frame
}

// credit owes the core bytes read on a channel; the writer sends them ahead of the queue. It never waits.
func (c *conn) credit(channel uint32, bytes int) {
	if bytes <= 0 {
		return
	}
	c.mu.Lock()
	if c.err == nil {
		c.credits[channel] += bytes
		c.pulse.ring()
	}
	c.mu.Unlock()
}

// write is the writer: credits first, then the queue in order. A write that fails or stalls past writeWithin may
// have left half a frame: it ends the connection.
func (c *conn) write() {
	for {
		out := c.next()
		if out == nil {
			select {
			case <-c.pulse:
				continue
			case <-c.done:
				return
			}
		}
		c.nc.SetWriteDeadline(time.Now().Add(c.writeWithin))
		_, err := c.nc.Write(out.frame)
		c.nc.SetWriteDeadline(time.Time{}) // an ssh exec's conn ends at any deadline left standing
		if err != nil {
			err = c.fail(fmt.Errorf("a write failed or stalled: %w", err))
		}
		if out.written != nil {
			out.written <- err
		}
		if err != nil {
			return
		}
	}
}

func (c *conn) next() *outgoing {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return nil
	}
	for channel, owed := range c.credits {
		delete(c.credits, channel)
		return &outgoing{frame: frameOf(channel, frameCredit, binary.LittleEndian.AppendUint32(nil, uint32(owed))), taken: true}
	}
	for len(c.queue) > 0 {
		out := c.queue[0]
		c.queue = c.queue[1:]
		if out.ctx != nil && out.ctx.Err() != nil {
			continue // its caller gave up before it went: abandon finds it gone and sends nothing either
		}
		out.taken = true
		return out
	}
	return nil
}
