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
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/dev1antdev/herdr-ui/internal/auth"
	"github.com/dev1antdev/herdr-ui/internal/herdr"
	"github.com/dev1antdev/herdr-ui/internal/machines"
	"github.com/dev1antdev/herdr-ui/internal/push"
	"github.com/dev1antdev/herdr-ui/internal/server"
	"github.com/dev1antdev/herdr-ui/internal/state"
	"github.com/dev1antdev/herdr-ui/internal/watch"
	"github.com/dev1antdev/herdr-ui/web"
)

// version is set at build time: -ldflags "-X main.version=…".
var version = "dev"

func main() {
	addr := flag.String("addr", "127.0.0.1:7317", "listen address (keep it loopback; use tailscale serve or an SSH tunnel for remote access)")
	herdrBin := flag.String("herdr-bin", "herdr", "herdr binary used for `terminal attach`")
	socket := flag.String("socket", herdr.SocketPath(), "herdr server socket")
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

	if _, err := exec.LookPath(*herdrBin); err != nil {
		log.Fatalf("herdr binary %q not found in PATH", *herdrBin)
	}
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
	watcher := watch.New(mgr, pushSvc)
	watcher.Start()

	s := server.New(mgr, watcher, pushSvc, auth.NewGuard(token), ui)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("herdr-ui %s listening on http://%s", version, *addr)
	log.Printf("sign in with the token from `herdr-ui -print-token`, or open:")
	log.Printf("  http://%s/?token=%s", *addr, token)

	if err := server.ListenAndServe(ctx, *addr, s.Handler()); err != nil {
		log.Fatal(err)
	}
}
