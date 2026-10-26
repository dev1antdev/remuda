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
	})
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
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
