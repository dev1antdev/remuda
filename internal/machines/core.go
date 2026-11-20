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
	"github.com/dev1antdev/remuda/internal/sshx"
)

// How a remuda started with -core reaches remuda-core instead of herdr: one `remuda-core connect` a machine — here a
// process of this remuda's, there an exec over the ssh master — which starts the core when none runs. The flag lives
// while the move off herdr is built; the release that makes the move takes herdr away with it.

// NoAttachOnCore is why a machine on remuda-core has no live terminal yet.
const NoAttachOnCore = "the live terminal on remuda-core is not in this remuda yet"

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

// coreLink is a machine's remuda-core: its client and the adapter that reads it as herdr.
type coreLink struct {
	client  *core.Client
	adapter *core.Adapter
}

// newCoreLink follows the core the client reaches; the machine lends the adapter its shell, the git its worktrees are
// made with, and a failure to reach the core goes to transportError (nil: none hears it).
func newCoreLink(mc *Machine, client *core.Client, transportError func(error)) *coreLink {
	return &coreLink{client: client, adapter: core.NewAdapter(client, mc, core.AdapterHooks{TransportError: transportError})}
}

// close stops following the core and ends the connection. The adapter tells its transport errors from its own
// goroutine, and a machine failing on one closes this: so it never waits here.
func (l *coreLink) close() {
	go func() {
		l.adapter.Close()
		l.client.Close()
	}()
}

// noAttach is the terminals of a machine on remuda-core until its live terminal is built: every attach refused.
func noAttach() herdr.Terminals {
	attach := herdr.NewAttachManager(nil)
	attach.Unavailable = NoAttachOnCore
	return attach
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

// OnCore says whether the machine is reached through remuda-core rather than herdr.
func (mc *Machine) OnCore() bool { return mc.speaksCore }

// Core says whether every machine of the fleet is reached through remuda-core (NewCoreManager): set once, so a
// request's guard needs no machine of its own to ask.
func (m *Manager) Core() bool { return m.core }
