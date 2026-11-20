package core

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// The live terminals of a machine on remuda-core: every viewer its own attach, so each gets the core's first paint of
// the screen, any number of viewers share a terminal without a refusal, and a phone that keeps the size or a link
// that only watches never resizes the desk's.

// attachWithin bounds the attach call itself.
const attachWithin = 15 * time.Second

// viewerChunk is the most one read hands a viewer.
const viewerChunk = 32 << 10

// Terminals opens a machine's terminals through its core client.
type Terminals struct {
	client *Client
	mu     sync.Mutex
	open   map[string]int
}

var _ herdr.Terminals = (*Terminals)(nil)

// NewTerminals follows the terminals the client reaches.
func NewTerminals(client *Client) *Terminals {
	return &Terminals{client: client, open: map[string]int{}}
}

// Open attaches one viewer: Keep types but leaves the size to the others, Observe only watches.
func (t *Terminals) Open(termID string, view herdr.View) (herdr.Viewer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), attachWithin)
	defer cancel()
	stream, err := t.client.Attach(ctx, AttachParams{
		Terminal:            termID,
		Cols:                cells(view.Cols),
		Rows:                cells(view.Rows),
		Observe:             view.Observe,
		KeepSize:            view.Keep && !view.Observe,
		ResizeNotifications: view.Keep || view.Observe,
	})
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	t.open[termID]++
	t.mu.Unlock()
	v := &coreViewer{stream: stream, observe: view.Observe, bytes: make(chan []byte, 8), done: make(chan struct{})}
	v.release = func() {
		t.mu.Lock()
		if t.open[termID]--; t.open[termID] <= 0 {
			delete(t.open, termID)
		}
		t.mu.Unlock()
	}
	go v.pump()
	return v, nil
}

// Viewers is how many of this remuda's viewers have the terminal open now.
func (t *Terminals) Viewers(termID string) int {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.open[termID]
}

// cells is a size as the core takes it: 0 for one out of its range.
func cells(n int) uint16 {
	if n < 0 || n > 0xffff {
		return 0
	}
	return uint16(n)
}

// errWatching refuses the keys of a viewer that only watches.
var errWatching = errors.New("this viewer only watches the terminal")

type coreViewer struct {
	stream  *Stream
	observe bool
	bytes   chan []byte
	done    chan struct{}
	closing sync.Once
	once    sync.Once // release, by whichever of the pump's end and Close comes first
	release func()
}

// pump hands the stream's bytes to the viewer until the stream ends or the viewer goes: a viewer that does not read
// holds the next read back, and with it the core's credit, so the core repaints it once it has fallen far behind.
func (v *coreViewer) pump() {
	defer close(v.bytes)
	defer v.once.Do(v.release)
	buf := make([]byte, viewerChunk)
	for {
		n, err := v.stream.Read(buf)
		if n > 0 {
			select {
			case v.bytes <- append([]byte(nil), buf[:n]...):
			case <-v.done:
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (v *coreViewer) Bytes() <-chan []byte { return v.bytes }

func (v *coreViewer) Write(data []byte) error {
	if v.observe {
		return errWatching
	}
	_, err := v.stream.Write(data)
	return err
}

func (v *coreViewer) Resize(cols, rows int) error {
	if v.observe || cells(cols) == 0 || cells(rows) == 0 {
		return nil
	}
	return v.stream.Resize(uint16(cols), uint16(rows))
}

// Close detaches the viewer; its bytes channel closes once the pump has gone.
func (v *coreViewer) Close() {
	v.closing.Do(func() {
		close(v.done)
		_ = v.stream.Close()
		v.once.Do(v.release)
	})
}
