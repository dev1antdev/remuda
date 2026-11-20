package core

import (
	"context"
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

// Open attaches one viewer: Keep types but leaves the size to the others, Observe only watches. A viewer of either
// kind is handed the terminal's grid in order with the bytes, an Out of its own ahead of the paint the core sends at
// that grid — never a sequence in the bytes, which the program itself could print.
func (t *Terminals) Open(termID string, view herdr.View) (herdr.Viewer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), attachWithin)
	defer cancel()
	v := &coreViewer{out: make(chan herdr.Out, 8), done: make(chan struct{})}
	params := AttachParams{
		Terminal:            termID,
		Observe:             view.Observe,
		KeepSize:            view.Keep && !view.Observe,
		ResizeNotifications: view.Keep || view.Observe,
		Resized:             v.grid, // called by the pump's Read, in order with the bytes
	}
	if herdr.ValidSize(view.Cols, view.Rows) {
		params.Cols, params.Rows = uint16(view.Cols), uint16(view.Rows)
	}
	stream, err := t.client.Attach(ctx, params)
	if err != nil {
		return nil, err
	}
	v.stream = stream
	if !view.Observe { // a viewer that only watches is not the person looking
		t.mu.Lock()
		t.open[termID]++
		t.mu.Unlock()
		v.release = func() {
			t.mu.Lock()
			if t.open[termID]--; t.open[termID] <= 0 {
				delete(t.open, termID)
			}
			t.mu.Unlock()
		}
	}
	go v.pump()
	return v, nil
}

// Viewers is how many of the person's viewers have the terminal open through this remuda now: a link that only watches
// counts for nothing.
func (t *Terminals) Viewers(termID string) int {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.open[termID]
}

type coreViewer struct {
	stream  *Stream
	out     chan herdr.Out
	done    chan struct{}
	closing sync.Once
	once    sync.Once // release, by whichever of the pump's end and Close comes first
	release func()    // nil for a viewer nobody counts
}

// pump hands the stream's bytes to the viewer until the stream ends or the viewer goes: a viewer that does not read
// holds the next read back, and with it the core's credit, so the core repaints it once it has fallen far behind.
func (v *coreViewer) pump() {
	defer close(v.out)
	defer v.once.Do(v.released)
	buf := make([]byte, viewerChunk)
	for {
		n, err := v.stream.Read(buf)
		if n > 0 && !v.hand(herdr.Out{Data: append([]byte(nil), buf[:n]...)}) {
			return
		}
		if err != nil {
			return
		}
	}
}

// hand gives the viewer the next Out; false once the viewer has gone.
func (v *coreViewer) hand(out herdr.Out) bool {
	select {
	case v.out <- out:
		return true
	case <-v.done:
		return false
	}
}

// grid tells the viewer the terminal's grid, ahead of the bytes drawn at it.
func (v *coreViewer) grid(cols, rows uint16) {
	v.hand(herdr.Out{Cols: int(cols), Rows: int(rows)})
}

func (v *coreViewer) released() {
	if v.release != nil {
		v.release()
	}
}

func (v *coreViewer) Out() <-chan herdr.Out { return v.out }

// Write types into the terminal; the core ignores what a viewer that only watches sends.
func (v *coreViewer) Write(data []byte) error {
	_, err := v.stream.Write(data)
	return err
}

// Resize tells the core this viewer's size: the terminal's when the viewer is the active one, a keep-size viewer's
// own record, nothing for one that only watches.
func (v *coreViewer) Resize(cols, rows int) error {
	if !herdr.ValidSize(cols, rows) {
		return nil
	}
	return v.stream.Resize(uint16(cols), uint16(rows))
}

// Close detaches the viewer; its bytes channel closes once the pump has gone.
func (v *coreViewer) Close() {
	v.closing.Do(func() {
		close(v.done)
		_ = v.stream.Close()
		v.once.Do(v.released)
	})
}
