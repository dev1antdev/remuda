package preview

// The previews open now: each a listener of the cockpit's own, made when the person presses Open on a port and closed
// once nothing has asked it for IdleAfter — so the tailnet hears no port of a machine's by itself.

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// ErrNotReady is a Listen that cannot open anything yet: the tailnet is not up.
var ErrNotReady = errors.New("the tailnet is not up yet")

// unsafePorts are the ports above 1023 a browser refuses to ask (Chrome's and Firefox's lists): a preview never gets one
var unsafePorts = map[int]bool{1719: true, 1720: true, 1723: true, 2049: true, 3659: true, 4045: true, 4190: true, 5060: true, 5061: true, 6000: true, 6566: true, 6665: true, 6666: true, 6667: true, 6668: true, 6669: true, 6679: true, 6697: true, 10080: true}

// Preview is a machine's port open on the cockpit.
type Preview struct {
	Machine string    `json:"machine"`
	Port    int       `json:"port"`  // the dev server's, on the machine
	Local   int       `json:"local"` // the cockpit's port it is opened on: the same number when that one is free
	URL     string    `json:"url"`
	Opened  time.Time `json:"opened"`
}

// Service keeps the previews open.
type Service struct {
	// Listen opens the cockpit's listener on a port and says the address a browser asks for there — the tailnet's
	// name over TLS, or the host the page was asked at; ErrNotReady while there is nowhere to listen
	Listen    func(port int, host string) (net.Listener, string, error)
	Allow     func(*http.Request) bool // the person's own signed-in device or the token
	IdleAfter time.Duration

	mu     sync.Mutex
	open   map[string]*entry
	sweep  sync.Once
	stop   chan struct{}
	closed bool // Shutdown ran: nothing opens any more
}

// errShutdown is an Open after remuda began to stop.
var errShutdown = errors.New("remuda is stopping")

type entry struct {
	Preview
	srv   *http.Server
	last  atomic.Int64 // when it was last asked, in Unix nanoseconds
	holds atomic.Int64 // requests being answered now, an upgraded one for as long as its connection lasts
}

func key(machine string, port int) string { return machine + "|" + strconv.Itoa(port) }

// Open opens the machine's port on the cockpit — or answers the preview already open there — after the dial reached
// the machine. host is the name the page was asked at, signIn the page's own address for a browser not signed in.
func (s *Service) Open(machine string, port int, dial func() (string, error), host, signIn string) (Preview, error) {
	if _, err := dial(); err != nil {
		return Preview{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Preview{}, errShutdown
	}
	if s.open == nil {
		s.open = map[string]*entry{}
		s.stop = make(chan struct{})
	}
	if e := s.open[key(machine, port)]; e != nil {
		e.last.Store(time.Now().UnixNano())
		return e.Preview, nil
	}
	ln, url, local, err := s.listen(port, host)
	if err != nil {
		return Preview{}, err
	}
	e := &entry{Preview: Preview{Machine: machine, Port: port, Local: local, URL: url, Opened: time.Now()}}
	e.last.Store(time.Now().UnixNano())
	seen := func() { e.last.Store(time.Now().UnixNano()) }
	e.srv = &http.Server{Handler: Guarded(s.Allow, signIn, Proxy(dial, port, url, seen, &e.holds)), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = e.srv.Serve(ln) }()
	s.open[key(machine, port)] = e
	s.sweep.Do(func() { go s.closeIdle(s.stop) })
	return e.Preview, nil
}

// listen opens the first cockpit port free for the machine's: its own number, else one of a range of its own
func (s *Service) listen(port int, host string) (net.Listener, string, int, error) {
	taken := map[int]bool{}
	for _, e := range s.open {
		taken[e.Local] = true
	}
	var last error
	for _, candidate := range candidates(port) {
		if taken[candidate] || unsafePorts[candidate] {
			continue
		}
		ln, url, err := s.Listen(candidate, host)
		if err == nil {
			local := candidate
			if addr, ok := ln.Addr().(*net.TCPAddr); ok && addr.Port != 0 {
				local = addr.Port
			}
			return ln, url, local, nil
		}
		last = err
		if errors.Is(err, ErrNotReady) {
			break
		}
	}
	return nil, "", 0, fmt.Errorf("no port of the cockpit to open the preview on: %w", last)
}

// candidates are the cockpit ports tried for a machine's: the same number first, then forty of a range of its own
func candidates(port int) []int {
	out := []int{port}
	for i := 0; i < 40; i++ {
		out = append(out, 41000+(port+i)%1000)
	}
	return out
}

// List is the previews open now, by machine and port.
func (s *Service) List() []Preview {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Preview, 0, len(s.open))
	for _, e := range s.open {
		out = append(out, e.Preview)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Machine < out[j].Machine || out[i].Machine == out[j].Machine && out[i].Port < out[j].Port
	})
	return out
}

// Close shuts one preview, its connections with it; false when none was open.
func (s *Service) Close(machine string, port int) bool {
	s.mu.Lock()
	e := s.open[key(machine, port)]
	delete(s.open, key(machine, port))
	s.mu.Unlock()
	if e == nil {
		return false
	}
	_ = e.srv.Close()
	return true
}

// CloseMachine shuts every preview of a machine, when it leaves the fleet.
func (s *Service) CloseMachine(machine string) {
	s.mu.Lock()
	var gone []*entry
	for k, e := range s.open {
		if e.Machine == machine {
			gone = append(gone, e)
			delete(s.open, k)
		}
	}
	s.mu.Unlock()
	for _, e := range gone {
		_ = e.srv.Close()
	}
}

// Shutdown shuts every preview, when remuda stops.
func (s *Service) Shutdown() {
	s.mu.Lock()
	open := s.open
	s.open = nil
	s.closed = true
	if s.stop != nil {
		close(s.stop)
		s.stop = nil
	}
	s.mu.Unlock()
	for _, e := range open {
		_ = e.srv.Close()
	}
}

// closeIdle shuts, every half minute, the previews nothing has asked for IdleAfter and that hold no connection, until stop
func (s *Service) closeIdle(stop chan struct{}) {
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return
		case now := <-tick.C:
			s.closeIdleAt(now)
		}
	}
}

// closeIdleAt is one sweep at a moment.
func (s *Service) closeIdleAt(now time.Time) {
	s.mu.Lock()
	var gone []*entry
	for k, e := range s.open {
		if e.holds.Load() == 0 && idle(&e.last, now) > s.IdleAfter {
			gone = append(gone, e)
			delete(s.open, k)
		}
	}
	s.mu.Unlock()
	for _, e := range gone {
		_ = e.srv.Close()
	}
}
