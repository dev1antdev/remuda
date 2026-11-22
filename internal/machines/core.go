package machines

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"time"

	"github.com/dev1antdev/remuda/internal/core"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/sshx"
)

// How a remuda started with -core reaches remuda-core instead of herdr: one `remuda-core connect` a machine — here a
// process of this remuda's, there an exec over the ssh master — which starts the core when none runs. The flag lives
// while the move off herdr is built; the release that makes the move takes herdr away with it.

// coreDialWithin bounds a dial whose context sets no deadline.
const coreDialWithin = 15 * time.Second

// dialWithin is how long a dial may take: until the context's deadline, else coreDialWithin.
func dialWithin(ctx context.Context) time.Duration {
	if deadline, ok := ctx.Deadline(); ok {
		return time.Until(deadline)
	}
	return coreDialWithin
}

// dialLocalCore reaches this machine's core through `remuda-core connect` under this remuda's own environment, so
// RC_SESSION, RC_SOCKET and the XDG folders aim a scratch instance at a throwaway core; the binary is the one on PATH
// first, so a scratch instance may run a build of its own.
func dialLocalCore(ctx context.Context) (net.Conn, error) {
	bin, err := herdr.FindBinary("remuda-core")
	if err != nil {
		return nil, fmt.Errorf("%v. Run `remuda core install`", err)
	}
	return sshx.CommandConn(exec.Command(bin, "connect"), dialWithin(ctx))
}

// remoteCoreDialer reaches the core on the far side of the master through `remuda-core connect` there, the way a
// script travels; one connection stays, holding one of the master's places for RPC.
func remoteCoreDialer(conn *sshx.Conn, bin string) core.Dialer {
	command := sshx.RemoteCommand("exec " + script.Quote(bin) + " connect")
	return func(ctx context.Context) (net.Conn, error) {
		return conn.DialExec(command, dialWithin(ctx))
	}
}

// coreLink is a machine's remuda-core: its client, the adapter that reads it as herdr, and its live terminals.
type coreLink struct {
	client    *core.Client
	adapter   *core.Adapter
	terminals *core.Terminals
}

// newCoreLink follows the core the client reaches; the machine lends the adapter its shell, the git its worktrees are
// made with, and its agents' sessions read off their processes; a failure to reach the core goes to transportError
// (nil: none hears it).
func newCoreLink(mc *Machine, client *core.Client, transportError func(error)) *coreLink {
	sessions := newProcessSessions(mc.Label(), mc.Transcripts.SessionsOf)
	return &coreLink{
		client:    client,
		adapter:   core.NewAdapter(client, mc, core.AdapterHooks{TransportError: transportError, SessionOf: sessions.of, Reread: sessions.readNow, Canonical: mc.Canonical}),
		terminals: core.NewTerminals(client),
	}
}

// close stops following the core and ends the connection. The adapter tells its transport errors from its own
// goroutine, and a machine failing on one closes this: so it never waits here.
func (l *coreLink) close() {
	go func() {
		l.adapter.Close()
		l.client.Close()
	}()
}

// reachCore follows the core on the far side of the master, once the probe found its binary there and it answered a
// hello.
func (mc *Machine) reachCore(conn *sshx.Conn, found probe) (*coreLink, error) {
	if found.core == "" {
		return nil, errNoCore(mc.Cfg.Host)
	}
	client := core.New(remoteCoreDialer(conn, found.core))
	if _, err := client.Hello(context.Background()); err != nil { // the client bounds its own dial and hello
		client.Close()
		return nil, fmt.Errorf("remuda-core on %s not answering: %v", mc.Cfg.Host, err)
	}
	return newCoreLink(mc, client, mc.transportError), nil
}

// RetryCore looks for remuda-core again on a remote connected without one, over the master it holds, and follows it
// once it answers: a job there may have just installed it. A master gone meanwhile fails the machine, for Ready to dial
// it again; of two looks at once the second to find the core lets its own go.
func (mc *Machine) RetryCore() {
	mc.mu.Lock()
	conn, waiting := mc.conn, mc.noCore != nil
	mc.mu.Unlock()
	if !waiting {
		return
	}
	out, err := shell.Text(conn, coreLookup, 10*time.Second)
	if err != nil {
		mc.transportError(err)
		return
	}
	link, err := mc.reachCore(conn, parseProbe(out))
	mc.mu.Lock()
	switch {
	case mc.conn != conn || mc.noCore == nil: // removed, failed, connected afresh, or the core found by another look
		mc.mu.Unlock()
		if link != nil {
			link.close()
		}
		return
	case err != nil:
		mc.noCore = err
		mc.mu.Unlock()
		return
	}
	mc.client, mc.attach, mc.link, mc.noCore = link.adapter, link.terminals, link, nil
	hook := mc.onState
	mc.mu.Unlock()
	if hook != nil {
		hook(mc.ID(), mc.Label(), StateConnected, "") // connected with its core now: whoever hears it reads the machine again
	}
}

// OnCore says whether the machine is reached through remuda-core rather than herdr.
func (mc *Machine) OnCore() bool { return mc.speaksCore }

// Core says whether every machine of the fleet is reached through its remuda-core (NewManager): set once, so a
// request's guard needs no machine of its own to ask.
func (m *Manager) Core() bool { return m.core }
