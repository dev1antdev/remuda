package restart

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
)

// Server restarts herdr's server onto the installed binary and starts every
// chat that came back as a bare shell again, its panes locked throughout.
func (r *Restarter) Server(mc *machines.Machine, say func(string)) error {
	client, _, err := mc.Ready()
	if err != nil {
		return err
	}
	snap, err := client.SessionSnapshot()
	if err != nil {
		return fmt.Errorf("read the session: %v", err)
	}
	type chat struct{ pane, kind, session, title string }
	var chats []chat
	for i := range snap.Panes {
		p := snap.Panes[i]
		if _, ok := agents[p.Agent]; !ok || sessionOf(&p) == "" {
			continue
		}
		chats = append(chats, chat{p.PaneID, p.Agent, sessionOf(&p), p.DisplayTitle()})
	}
	sort.Slice(chats, func(i, j int) bool { return chats[i].pane < chats[j].pane })
	if r.Lock != nil {
		var locks []sync.Locker
		for _, c := range chats {
			l := r.Lock(mc.Cfg.ID, c.pane)
			l.Lock()
			locks = append(locks, l)
		}
		defer func() {
			for _, l := range locks {
				l.Unlock()
			}
		}()
	}
	say(fmt.Sprintf("%d pane(s); %d chat(s) with a session to come back to", len(snap.Panes), len(chats)))
	for _, c := range chats {
		say(fmt.Sprintf("  · %s (%s): %s %s", c.title, c.pane, Title(c.kind), short(c.session)))
	}

	say("herdr server stop")
	out, err := mc.StopServer()
	if strings.TrimSpace(out) != "" {
		say("  " + strings.TrimSpace(out))
	}
	if err != nil {
		return fmt.Errorf("stop: %v", err)
	}
	time.Sleep(2 * time.Second) // the old process lets go of the socket
	say("herdr server — the installed binary")
	if err := mc.StartServer(); err != nil {
		return fmt.Errorf("start: %v (remuda keeps trying by itself; the chats are listed above)", err)
	}

	// herdr restores the shape at once and its own agents a moment later
	fresh, err := r.waitSession(mc, 60*time.Second)
	if err != nil {
		return fmt.Errorf("the server did not answer in time: %v (the chats are listed above: `claude --resume <id>`)", err)
	}
	installed, running, _ := mc.ServerVersions()
	if running != "" {
		say(fmt.Sprintf("server up: %s (installed %s), %d pane(s) restored", running, installed, len(fresh.Panes)))
	}
	if len(chats) == 0 {
		return nil
	}
	time.Sleep(8 * time.Second) // what herdr brings back itself shows up
	fresh, err = r.waitSession(mc, 20*time.Second)
	if err != nil {
		return err
	}
	byID := map[string]herdr.PaneInfo{}
	for i := range fresh.Panes {
		byID[fresh.Panes[i].PaneID] = fresh.Panes[i]
	}
	say("== chats back")
	var failed []string
	for _, c := range chats {
		p, ok := byID[c.pane]
		a := agents[c.kind]
		switch {
		case !ok:
			say(fmt.Sprintf("· %s (%s): the pane did not come back — `%s`", c.title, c.pane, Resume(c.kind, c.session)))
			failed = append(failed, c.title)
		case p.Agent == c.kind:
			say(fmt.Sprintf("· %s (%s): %s came back by itself", c.title, c.pane, a.Title))
		case p.Agent != "":
			say(fmt.Sprintf("· %s (%s): runs %q now — left as it is", c.title, c.pane, p.Agent))
		default:
			args := a.resume(c.session)
			say(fmt.Sprintf("· %s (%s): %s %s", c.title, c.pane, a.Kind, strings.Join(args, " ")))
			cl, _, err := mc.Ready()
			if err != nil {
				return err
			}
			if err := cl.AgentStartWith(c.pane, a.Kind, agentName(a.Kind, c.pane), args, 90*time.Second); err != nil {
				say(fmt.Sprintf("  ✗ %v — run `%s` in that pane", err, Resume(c.kind, c.session)))
				failed = append(failed, c.title)
				continue
			}
			say(fmt.Sprintf("  ✓ %s is back, session %s resumed", a.Title, short(c.session)))
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d chat(s) not back: %s", len(failed), strings.Join(failed, ", "))
	}
	return nil
}

// waitSession polls until the machine's herdr answers with a session.
func (r *Restarter) waitSession(mc *machines.Machine, timeout time.Duration) (*herdr.Snapshot, error) {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		client, _, err := mc.Ready()
		if err == nil {
			snap, err := client.SessionSnapshot()
			if err == nil {
				return snap, nil
			}
			last = err
		} else {
			last = err
		}
		time.Sleep(2 * time.Second)
	}
	return nil, last
}
