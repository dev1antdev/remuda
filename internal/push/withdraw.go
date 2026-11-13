package push

import (
	"strconv"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
)

// A notification takes itself back. Every question, finish and watchdog note handed to a device stands for its pane
// until what it told of no longer holds — the pane stops asking, its finish is looked at, it moves on or closes — and
// then the devices it went to are told: a socket device (the Mac app) at once over its connection, or when it next
// connects; a browser with every push it gets for a day, whose service worker closes those tags before it shows the
// new one, since Web Push carries nothing silent; and every page of ours through the roster, which closes them in the
// browser it runs in. An alert's tag names its episode, so a withdrawal never takes down a newer one of the same pane.

// keepFor is how long a notification is followed: a finish left on a lock screen overnight still comes down in the morning.
const keepFor = 24 * time.Hour

const (
	maxStanding  = 500 // alerts followed at once, the oldest let go first
	maxWithdrawn = 50  // withdrawals the roster lists for the pages
	maxOwed      = 20  // tags one push carries for its service worker to close
)

// standsWhile is the pane status an alert of a kind tells of: once the pane is in another, the alert no longer stands.
var standsWhile = map[string]string{
	"input":   herdr.StatusBlocked,
	"waiting": herdr.StatusBlocked,
	"done":    herdr.StatusDone,
	"stuck":   herdr.StatusWorking,
	"loop":    herdr.StatusWorking,
}

// episodeTag names one alert of a pane: the machine, the pane, the kind and the moment its episode began.
func episodeTag(pane events.PaneRef, kind string, began time.Time) string {
	return pane.String() + "|" + kind + "|" + strconv.FormatInt(began.UnixMilli(), 10)
}

type standingAlert struct {
	tag, kind string
	pane      events.PaneRef
	at        time.Time
	devices   []string // the endpoints it was handed to
}

type owedTag struct {
	tag string
	at  time.Time
}

// withdrawals keeps the alerts standing and the ones taken back, under a lock of its own: a timer delivering an alert
// and a status change withdrawing one meet here, never under the service's lock.
type withdrawals struct {
	mu       sync.Mutex
	standing []standingAlert
	recent   []owedTag            // newest last, for the roster
	owed     map[string][]owedTag // by endpoint: what its next pushes, or its next connection, close
}

func newWithdrawals() *withdrawals { return &withdrawals{owed: map[string][]owedTag{}} }

// stand notes an alert handed to devices; one raised again under a tag taken back before stands again, so no page
// and no later push closes it.
func (w *withdrawals) stand(p Payload, devices []string, now time.Time) {
	if len(devices) == 0 || p.Tag == "" || p.Machine == "" || p.Pane == "" || standsWhile[p.Kind] == "" {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pruneLocked(now)
	w.recent = without(w.recent, p.Tag)
	for endpoint, owed := range w.owed {
		w.owed[endpoint] = without(owed, p.Tag)
	}
	for i, alert := range w.standing {
		if alert.tag == p.Tag { // a reminder of the same episode: the devices it reached join the first's
			w.standing[i].devices = union(alert.devices, devices)
			return
		}
	}
	w.standing = append(w.standing, standingAlert{tag: p.Tag, kind: p.Kind, pane: events.PaneRef{Machine: p.Machine, Pane: p.Pane}, at: now, devices: devices})
	if len(w.standing) > maxStanding {
		w.standing = w.standing[len(w.standing)-maxStanding:]
	}
}

// take removes the pane's standing alerts that match and notes them taken back, each device owed its tags; the
// alerts taken are answered.
func (w *withdrawals) take(pane events.PaneRef, match func(kind string) bool, now time.Time) []standingAlert {
	w.mu.Lock()
	defer w.mu.Unlock()
	var taken []standingAlert
	kept := w.standing[:0]
	for _, alert := range w.standing {
		if alert.pane == pane && match(alert.kind) {
			taken = append(taken, alert)
		} else {
			kept = append(kept, alert)
		}
	}
	w.standing = kept
	for _, alert := range taken {
		w.recent = append(without(w.recent, alert.tag), owedTag{alert.tag, now})
		for _, endpoint := range alert.devices {
			w.owed[endpoint] = append(without(w.owed[endpoint], alert.tag), owedTag{alert.tag, now})
			if len(w.owed[endpoint]) > maxOwed {
				w.owed[endpoint] = w.owed[endpoint][len(w.owed[endpoint])-maxOwed:]
			}
		}
	}
	if len(w.recent) > maxWithdrawn {
		w.recent = w.recent[len(w.recent)-maxWithdrawn:]
	}
	return taken
}

// owedTo is the tags a device is owed, newest first, less one about to be shown under the same tag.
func (w *withdrawals) owedTo(endpoint, except string, now time.Time) []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pruneLocked(now)
	return tagsOf(w.owed[endpoint], except)
}

// settle forgets what a device was owed once it took it: a socket device's connection, which acts on it at once.
func (w *withdrawals) settle(endpoint string, tags []string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	owed := w.owed[endpoint]
	for _, tag := range tags {
		owed = without(owed, tag)
	}
	if len(owed) == 0 {
		delete(w.owed, endpoint)
	} else {
		w.owed[endpoint] = owed
	}
}

// forget drops a device that went away.
func (w *withdrawals) forget(endpoint string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.owed, endpoint)
}

// withdrawn is the tags taken back within keepFor, newest first: the pages close them wherever they still show.
func (w *withdrawals) withdrawn(now time.Time) []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pruneLocked(now)
	return tagsOf(w.recent, "")
}

func (w *withdrawals) pruneLocked(now time.Time) {
	fresh := func(at time.Time) bool { return now.Sub(at) < keepFor }
	standing := w.standing[:0]
	for _, alert := range w.standing {
		if fresh(alert.at) {
			standing = append(standing, alert)
		}
	}
	w.standing = standing
	w.recent = freshOnly(w.recent, fresh)
	for endpoint, owed := range w.owed {
		if owed = freshOnly(owed, fresh); len(owed) == 0 {
			delete(w.owed, endpoint)
		} else {
			w.owed[endpoint] = owed
		}
	}
}

func freshOnly(list []owedTag, fresh func(time.Time) bool) []owedTag {
	kept := list[:0]
	for _, item := range list {
		if fresh(item.at) {
			kept = append(kept, item)
		}
	}
	return kept
}

func without(list []owedTag, tag string) []owedTag {
	kept := list[:0]
	for _, item := range list {
		if item.tag != tag {
			kept = append(kept, item)
		}
	}
	return kept
}

func tagsOf(list []owedTag, except string) []string {
	var tags []string
	for i := len(list) - 1; i >= 0; i-- {
		if list[i].tag != except {
			tags = append(tags, list[i].tag)
		}
	}
	return tags
}

func union(a, b []string) []string {
	out := append([]string(nil), a...)
	for _, item := range b {
		found := false
		for _, have := range out {
			found = found || have == item
		}
		if !found {
			out = append(out, item)
		}
	}
	return out
}

// withdraw takes back the pane's standing alerts that match: a socket device that takes withdrawals hears it at once,
// every other device with its next pushes, and the pages through the roster.
func (s *Service) withdraw(pane events.PaneRef, match func(kind string) bool) {
	taken := s.withdrawals.take(pane, match, time.Now())
	if len(taken) == 0 {
		return
	}
	byDevice := map[string][]string{}
	for _, alert := range taken {
		for _, endpoint := range alert.devices {
			if IsSocket(endpoint) {
				byDevice[endpoint] = append(byDevice[endpoint], alert.tag)
			}
		}
	}
	for endpoint, tags := range byDevice {
		if s.Sockets.Withdraw(endpoint, tags) {
			s.withdrawals.settle(endpoint, tags)
		}
	}
	if s.OnWithdraw != nil {
		s.OnWithdraw()
	}
}

// moved takes back what a pane's new status ends: a question once it no longer asks, a finish once it is no longer
// done, a watchdog note once the state it was about moved on, everything once the pane closed.
func (s *Service) moved(pane events.PaneRef, to string) {
	s.withdraw(pane, func(kind string) bool { return to == events.Gone || standsWhile[kind] != to })
}

// Seen takes back a pane's finish once someone looked at it through remuda: its chat, its terminal, a seen mark.
func (s *Service) Seen(machine, pane string) {
	s.withdraw(events.PaneRef{Machine: machine, Pane: pane}, func(kind string) bool { return kind == "done" })
}

// Withdrawn is the tags taken back within a day, newest first, for the roster: every page closes them in its browser.
func (s *Service) Withdrawn() []string { return s.withdrawals.withdrawn(time.Now()) }

// OwedTo is what a device is owed and has not taken: a socket device hears it as it connects.
func (s *Service) OwedTo(endpoint string) []string {
	return s.withdrawals.owedTo(endpoint, "", time.Now())
}

// Settle notes that a device took the tags it was owed.
func (s *Service) Settle(endpoint string, tags []string) { s.withdrawals.settle(endpoint, tags) }
