// Package watchdog notices agents that need a person without asking for
// one: working for a long time while the transcript stays silent (a hung
// tool, a wedged process), waiting on an answer for a long time (a prompt
// nobody saw), or looping — the same tool call again and again. It reads
// the radar's episodes and the transcripts a minute apart, leaves a note on
// the pane for the sidebar, and pushes once per episode, then as a
// reminder now and then while the condition holds.
package watchdog

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/herdr-ui/internal/herdr"
	"github.com/dev1antdev/herdr-ui/internal/push"
	"github.com/dev1antdev/herdr-ui/internal/radar"
)

// Inputs is everything the rules read and do, as functions, so the rules
// can be tested without herdr.
type Inputs struct {
	Episodes  func() []radar.Episode
	Pane      func(machineID, paneID string) (PaneInfo, bool)
	LastWrite func(machineID, paneID string) (time.Time, bool) // the transcript's last write
	LastTools func(machineID, paneID string, n int) []string   // fingerprints of the last n tool calls of the current turn
	Note      func(machineID, paneID, note string)
	Notify    func(push.Payload)
	Now       func() time.Time
}

type PaneInfo struct {
	Title, Workspace, Agent string
}

type Watchdog struct {
	in         Inputs
	QuietAfter time.Duration // working this long with no transcript write
	WaitAfter  time.Duration // blocked this long
	Remind     time.Duration // between pushes for one unchanged condition
	LoopN      int           // identical tool calls in a row that count as a loop

	mu    sync.Mutex
	fired map[string]time.Time // machine|pane|kind|since → last push
}

func New(in Inputs) *Watchdog {
	if in.Now == nil {
		in.Now = time.Now
	}
	return &Watchdog{in: in, QuietAfter: 20 * time.Minute, WaitAfter: 15 * time.Minute, Remind: 30 * time.Minute, LoopN: 6, fired: map[string]time.Time{}}
}

// Run sweeps forever, `every` apart.
func (w *Watchdog) Run(every time.Duration) {
	for {
		w.Sweep()
		time.Sleep(every)
	}
}

// Sweep applies the rules to every episode once.
func (w *Watchdog) Sweep() {
	now := w.in.Now()
	live := map[string]bool{}
	for _, ep := range w.in.Episodes() {
		age := now.Sub(ep.Since)
		var kind, note, body string
		var actions []push.Action
		switch ep.State {
		case herdr.StatusWorking:
			if age < w.QuietAfter {
				break
			}
			if last, ok := w.in.LastWrite(ep.Machine, ep.Pane); ok && now.Sub(last) >= w.QuietAfter {
				kind = "stuck"
				note = fmt.Sprintf("quiet for %s — working, but nothing written", minutes(now.Sub(last)))
				body = "working for " + minutes(age) + " with nothing written for " + minutes(now.Sub(last)) + " — a hung tool, or a wedged process?"
				actions = []push.Action{{Action: "open", Title: "Open"}, {Action: "esc", Title: "Send Esc"}}
			} else if tools := w.in.LastTools(ep.Machine, ep.Pane, w.LoopN); len(tools) == w.LoopN && allSame(tools) {
				kind = "loop"
				name, _, _ := strings.Cut(tools[0], "\x00")
				note = fmt.Sprintf("looping — the same %s call %d times in a row", name, w.LoopN)
				body = "the same " + name + " call " + fmt.Sprint(w.LoopN) + " times in a row"
				actions = []push.Action{{Action: "open", Title: "Open"}, {Action: "esc", Title: "Send Esc"}}
			}
		case herdr.StatusBlocked:
			if age >= w.WaitAfter {
				kind = "waiting"
				note = "waiting for your answer for " + minutes(age)
				body = "has been waiting for your answer for " + minutes(age)
				actions = []push.Action{{Action: "open", Title: "Open"}}
			}
		}
		if kind == "" {
			w.in.Note(ep.Machine, ep.Pane, "")
			continue
		}
		w.in.Note(ep.Machine, ep.Pane, note)
		key := fmt.Sprintf("%s|%s|%s|%d", ep.Machine, ep.Pane, kind, ep.Since.Unix())
		live[key] = true
		w.mu.Lock()
		last, seen := w.fired[key]
		due := !seen || now.Sub(last) >= w.Remind
		if due {
			w.fired[key] = now
		}
		w.mu.Unlock()
		if !due {
			continue
		}
		info, _ := w.in.Pane(ep.Machine, ep.Pane)
		label := info.Workspace
		if label == "" {
			label = ep.Pane
		}
		agent := info.Agent
		if agent == "" {
			agent = "agent"
		}
		title := label + " looks stuck"
		if kind == "waiting" {
			title = label + " is still waiting"
		}
		if info.Title != "" && info.Title != info.Agent {
			body += " · " + info.Title
		}
		w.in.Notify(push.Payload{
			Machine: ep.Machine, Pane: ep.Pane, Kind: kind,
			Title: title, Body: agent + " " + body, Tag: ep.Machine + "|" + ep.Pane + "|" + kind,
			Actions: actions,
		})
	}
	// an episode that ended takes its memory with it
	w.mu.Lock()
	for k := range w.fired {
		if !live[k] {
			delete(w.fired, k)
		}
	}
	w.mu.Unlock()
}

func allSame(xs []string) bool {
	for _, x := range xs[1:] {
		if x != xs[0] {
			return false
		}
	}
	return true
}

func minutes(d time.Duration) string {
	m := int(d.Round(time.Minute) / time.Minute)
	if m < 60 {
		return fmt.Sprintf("%d min", m)
	}
	return fmt.Sprintf("%dh %02dm", m/60, m%60)
}
