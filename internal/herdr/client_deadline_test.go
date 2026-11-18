package herdr

import (
	"net"
	"testing"
	"time"
)

// deadlineConn notes the deadline a request sets on it.
type deadlineConn struct {
	net.Conn
	set time.Time
}

func (c *deadlineConn) SetDeadline(t time.Time) error {
	c.set = t
	return c.Conn.SetDeadline(t)
}

// TestWaitingForAConnectionComesOutOfTheTimeout: a bridged machine may queue the dial for a free ssh session.
func TestWaitingForAConnectionComesOutOfTheTimeout(t *testing.T) {
	for name, ask := range map[string]func(c *Client) error{
		"a request":      func(c *Client) error { return c.call("ping", nil, nil, c.Timeout) },
		"a subscription": func(c *Client) error { _, err := c.subscribe(nil); return err },
	} {
		ours, theirs := net.Pipe()
		theirs.Close()
		conn := &deadlineConn{Conn: ours}
		c := NewClient("/x.sock")
		c.Timeout = 2 * time.Second
		c.Dial = func(time.Duration) (net.Conn, error) {
			time.Sleep(200 * time.Millisecond)
			return conn, nil
		}
		began := time.Now()
		ask(c)
		if limit := began.Add(c.Timeout + 100*time.Millisecond); conn.set.IsZero() || conn.set.After(limit) {
			t.Errorf("%s: the deadline is %s after the call began", name, conn.set.Sub(began))
		}
	}
}
