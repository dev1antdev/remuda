// herdr-ui — self-hosted browser/phone client for herdr. One Go binary, no
// external services: herdr owns the agents and panes, Claude Code owns its
// transcripts, herdr-ui bridges them to your browser over localhost (or your
// own tailnet / SSH tunnel).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dev1antdev/herdr-ui/internal/auth"
	"github.com/dev1antdev/herdr-ui/internal/code"
	"github.com/dev1antdev/herdr-ui/internal/herdr"
	"github.com/dev1antdev/herdr-ui/internal/machines"
	"github.com/dev1antdev/herdr-ui/internal/push"
	"github.com/dev1antdev/herdr-ui/internal/radar"
	"github.com/dev1antdev/herdr-ui/internal/review"
	"github.com/dev1antdev/herdr-ui/internal/server"
	"github.com/dev1antdev/herdr-ui/internal/state"
	"github.com/dev1antdev/herdr-ui/internal/tasks"
	"github.com/dev1antdev/herdr-ui/internal/watch"
	"github.com/dev1antdev/herdr-ui/web"
)

// version is set at build time: -ldflags "-X main.version=…".
var version = "dev"

func main() {
	addr := flag.String("addr", "127.0.0.1:7317", "listen address (keep it loopback; use tailscale serve or an SSH tunnel for remote access)")
	herdrBin := flag.String("herdr-bin", "herdr", "herdr binary used for `terminal attach`")
	socket := flag.String("socket", herdr.SocketPath(), "herdr server socket")
	codePort := flag.Int("code-port", code.DefaultPort, "port each machine's code-server listens on (127.0.0.1, auth none; see deploy/install-code.sh)")
	printToken := flag.Bool("print-token", false, "print the access token and exit")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("herdr-ui", version)
		return
	}

	dir, err := state.Dir()
	if err != nil {
		log.Fatalf("state dir: %v", err)
	}
	token, err := auth.LoadOrCreateToken(dir)
	if err != nil {
		log.Fatalf("token: %v", err)
	}
	if *printToken {
		fmt.Println(token)
		return
	}

	bin, err := herdr.FindBinary(*herdrBin)
	if err != nil {
		log.Fatalf("herdr: %v", err)
	}
	*herdrBin = bin
	if v, protocol, err := herdr.NewClient(*socket).Ping(); err != nil {
		log.Printf("WARNING: herdr is not answering at %s (%v) — start herdr first; herdr-ui will keep retrying", *socket, err)
	} else {
		log.Printf("connected to herdr %s (protocol %d) at %s", v, protocol, *socket)
	}

	ui, err := web.Assets()
	if err != nil {
		log.Fatalf("ui assets: %v", err)
	}

	mgr := machines.NewManager(dir, *socket, *herdrBin)
	defer mgr.Close()
	pushSvc, err := push.New(dir)
	if err != nil {
		log.Fatalf("push: %v", err)
	}
	// a pane open since before herdr-ui started: its session file's mtime
	// says when the agent last wrote
	tracker := radar.New(dir, func(machineID string, p *herdr.PaneInfo) time.Time {
		if p.AgentSession == nil {
			return time.Time{}
		}
		mc, err := mgr.Get(machineID)
		if err != nil {
			return time.Time{}
		}
		path, err := mc.ResolveSession(p.Dir(), p.AgentSession.Kind, p.AgentSession.Value)
		if err != nil {
			return time.Time{}
		}
		v, err := mc.TranscriptVersion(path)
		if err != nil {
			return time.Time{}
		}
		if v.MTime > 1e12 { // nanoseconds from a local stat, seconds from a remote one
			return time.Unix(0, v.MTime)
		}
		return time.Unix(v.MTime, 0)
	})
	defer tracker.Flush()
	reviews := review.New(dir)
	watcher := watch.New(mgr, pushSvc, tracker)
	watcher.Badges, watcher.Dirs = reviews.Badges, server.WorkspaceDirs
	watcher.Start()

	s := server.New(mgr, watcher, pushSvc, tasks.New(dir), code.New(*codePort), reviews, tracker, auth.NewGuard(token), ui)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("herdr-ui %s listening on http://%s", version, *addr)
	log.Printf("sign in with the token from `herdr-ui -print-token`, or open:")
	log.Printf("  http://%s/?token=%s", *addr, token)

	// log.Fatal would skip the deferred flush and close
	if err := server.ListenAndServe(ctx, *addr, s.Handler()); err != nil {
		log.Print(err)
	}
}
