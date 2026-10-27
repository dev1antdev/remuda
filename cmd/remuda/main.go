// remuda is a self-hosted browser and phone cockpit for the coding agents
// herdr runs: one Go binary that bridges herdr and the agents' transcripts
// to the browser over localhost, a tailnet or an SSH tunnel.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	_ "time/tzdata" // the usage report cuts days in the viewer's zone, wherever the binary runs

	"github.com/dev1antdev/remuda/internal/app"
	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/code"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/state"
	buildversion "github.com/dev1antdev/remuda/internal/version"
)

// version is set at build time: -ldflags "-X main.version=…".
var version = "dev"

func main() {
	buildversion.Build = version
	if len(os.Args) > 1 {
		if command, ok := commands[os.Args[1]]; ok {
			command(os.Args[2:])
			return
		}
	}
	addr := flag.String("addr", "127.0.0.1:7317", "listen address (keep it loopback; use tailscale serve or an SSH tunnel for remote access)")
	herdrBin := flag.String("herdr-bin", "herdr", "herdr binary used for `terminal attach`")
	socket := flag.String("socket", herdr.SocketPath(), "herdr server socket")
	codePort := flag.Int("code-port", code.DefaultPort, "port each machine's code-server listens on (127.0.0.1, auth none; `remuda code` installs it)")
	printToken := flag.Bool("print-token", false, "print the access token and exit")
	showVersion := flag.Bool("version", false, "print the version and exit")
	tailscale := flag.Bool("tailscale", false, "join your tailnet as a node of its own and serve HTTPS there (no tailscale serve needed); first run prints a login URL unless TS_AUTHKEY is set")
	tsName := flag.String("tailscale-hostname", "remuda", "the node's name on the tailnet (https://<name>.<tailnet>.ts.net)")
	noCheck := flag.Bool("no-version-check", false, "never ask npm and GitHub what the newest Claude Code, herdr and remuda are")
	noDoor := flag.Bool("no-door", false, "keep the agents' door closed: no agent-key / agent-url files here or on a remote machine (a scratch instance beside the real one must not overwrite the real one's)")
	withDoor := flag.Bool("door", false, "open the agents' door although REMUDA_STATE_DIR is set, which closes it by itself")
	flag.Usage = help
	flag.Parse()

	if *showVersion {
		fmt.Println("remuda", version)
		return
	}

	dir, err := state.Dir()
	if err != nil {
		log.Fatalf("state dir: %v", err)
	}
	socketsUnder(dir)
	token, created, err := auth.LoadOrCreateToken(dir)
	if err != nil {
		log.Fatalf("token: %v", err)
	}
	if *printToken {
		fmt.Println(token)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err = app.Run(ctx, app.Options{
		Dir: dir, Token: token, ShowSignIn: created || interactive(),
		Addr: *addr, HerdrBin: *herdrBin, Socket: *socket, CodePort: *codePort,
		Tailscale: *tailscale, TailscaleName: *tsName, AuthKey: os.Getenv("TS_AUTHKEY"),
		VersionCheck: !*noCheck, Version: version,
		Door: doorOpen(*noDoor, *withDoor, os.Getenv("REMUDA_STATE_DIR") != ""),
	})
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

// doorOpen says whether this instance places the agents' door files: never with -no-door, and not for a scratch
// instance (REMUDA_STATE_DIR set) unless -door asks, so a second remuda beside the real one leaves its door alone.
func doorOpen(noDoor, withDoor, scratch bool) bool {
	if noDoor {
		return false
	}
	return !scratch || withDoor
}

// help is `remuda -h`: the server's flags, and the commands a person with nothing but the binary needs to know are there.
func help() {
	w := flag.CommandLine.Output()
	fmt.Fprint(w, `remuda — a browser and phone cockpit for the coding agents herdr runs

  remuda [flags]       the server (the background service runs this)
  remuda setup         herdr, its server, the agent hooks and the service, here or -host H
  remuda code          code-server for the Code mode, here or -host H
  remuda mcp           your task list for the coding agents, over MCP; mcp install registers it with each agent, here or -host H
  remuda tasks         your tasks from a shell: list, show, add, move, comment
  remuda uninstall     take remuda off this machine again (-all: herdr and your data too)

Each command lists its own flags with -h. The server's:

`)
	flag.PrintDefaults()
}
