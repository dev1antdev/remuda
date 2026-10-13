// remuda is a self-hosted browser and phone cockpit for the coding agents
// herdr runs: one Go binary that bridges herdr and the agents' transcripts
// to the browser over localhost, a tailnet or an SSH tunnel.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	_ "time/tzdata" // the usage report cuts days in the viewer's zone, wherever the binary runs

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/code"
	"github.com/dev1antdev/remuda/internal/health"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/jobs"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/quick"
	"github.com/dev1antdev/remuda/internal/radar"
	"github.com/dev1antdev/remuda/internal/restart"
	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/schedule"
	"github.com/dev1antdev/remuda/internal/search"
	"github.com/dev1antdev/remuda/internal/server"
	"github.com/dev1antdev/remuda/internal/setup"
	"github.com/dev1antdev/remuda/internal/state"
	"github.com/dev1antdev/remuda/internal/tasks"
	"github.com/dev1antdev/remuda/internal/transcript"
	"github.com/dev1antdev/remuda/internal/tsnetx"
	"github.com/dev1antdev/remuda/internal/usage"
	buildversion "github.com/dev1antdev/remuda/internal/version"
	"github.com/dev1antdev/remuda/internal/watch"
	"github.com/dev1antdev/remuda/internal/watchdog"
	"github.com/dev1antdev/remuda/web"
)

// version is set at build time: -ldflags "-X main.version=…".
var version = "dev"

func main() {
	buildversion.Build = version
	if len(os.Args) > 1 && os.Args[1] == "search" {
		searchCmd(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "setup" {
		setupCmd(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "uninstall" {
		uninstallCmd(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "code" {
		codeCmd(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "usage" {
		usageCmd(os.Args[2:])
		return
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

	bin, err := herdr.FindBinary(*herdrBin)
	if err != nil {
		log.Fatalf("herdr: %v", err)
	}
	*herdrBin = bin
	if err := herdr.EnsureServer(bin, *socket); err != nil {
		log.Printf("WARNING: herdr is not answering at %s (%v); remuda will keep trying", *socket, err)
	} else if v, protocol, err := herdr.NewClient(*socket).Ping(); err == nil {
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
	sessionFile := func(machineID string, p *herdr.PaneInfo) (*machines.Machine, string, bool) {
		if p.AgentSession == nil {
			return nil, "", false
		}
		mc, err := mgr.Get(machineID)
		if err != nil {
			return nil, "", false
		}
		path, err := mc.ResolveSession(p.Dir(), p.AgentSession.Kind, p.AgentSession.Value)
		if err != nil {
			return nil, "", false
		}
		return mc, mc.FollowContinuation(path), true
	}
	lastWrite := func(machineID string, p *herdr.PaneInfo) (time.Time, bool) {
		mc, path, ok := sessionFile(machineID, p)
		if !ok {
			return time.Time{}, false
		}
		v, err := mc.TranscriptVersion(path)
		if err != nil {
			return time.Time{}, false
		}
		return v.Time(), true
	}
	tracker := radar.New(dir, func(machineID string, p *herdr.PaneInfo) time.Time {
		at, _ := lastWrite(machineID, p)
		return at
	})
	defer tracker.Flush()
	reviews := review.New(dir)
	watcher := watch.New(mgr, pushSvc, tracker)
	watcher.Badges, watcher.Dirs = reviews.Badges, server.WorkspaceDirs
	meter := usage.NewTracker()
	var meterBusy sync.Map
	watcher.Usage = func(mc *machines.Machine, snap *herdr.Snapshot) {
		if _, running := meterBusy.LoadOrStore(mc.Cfg.ID, true); running {
			return
		}
		panes := append([]herdr.PaneInfo(nil), snap.Panes...)
		go func() {
			defer meterBusy.Delete(mc.Cfg.ID)
			for i := range panes {
				p := &panes[i]
				if p.Agent == "" || p.AgentSession == nil {
					continue
				}
				orig, err := mc.ResolveSession(p.Dir(), p.AgentSession.Kind, p.AgentSession.Value)
				if err != nil {
					continue
				}
				chain := mc.ContinuationChain(orig)
				for n := 0; n < 8; n++ {
					pred := mc.Predecessor(chain[0])
					if pred == "" {
						break
					}
					chain = append([]string{pred}, chain...)
				}
				meter.Observe(mc, p.PaneID, chain)
			}
		}()
	}
	watcher.Start()

	paneAndSnap := func(machineID, paneID string) (*herdr.PaneInfo, *herdr.Snapshot) {
		snap, _ := watcher.Snapshot(machineID)
		if snap == nil {
			return nil, nil
		}
		for i := range snap.Panes {
			if snap.Panes[i].PaneID == paneID {
				return &snap.Panes[i], snap
			}
		}
		return nil, snap
	}
	paneOf := func(machineID, paneID string) *herdr.PaneInfo {
		p, _ := paneAndSnap(machineID, paneID)
		return p
	}
	lastTurn := func(machineID, paneID string) (transcript.Turn, bool) {
		p := paneOf(machineID, paneID)
		if p == nil {
			return transcript.Turn{}, false
		}
		mc, path, ok := sessionFile(machineID, p)
		if !ok {
			return transcript.Turn{}, false
		}
		v, err := mc.TranscriptVersion(path)
		if err != nil {
			return transcript.Turn{}, false
		}
		turns, err := mc.Transcript(path, v)
		if err != nil || len(turns) == 0 {
			return transcript.Turn{}, false
		}
		last := turns[len(turns)-1]
		return last, last.Role == "assistant"
	}
	wd := watchdog.New(watchdog.Inputs{
		Episodes: tracker.Episodes,
		Pane: func(machineID, paneID string) (watchdog.PaneInfo, bool) {
			p, snap := paneAndSnap(machineID, paneID)
			if p == nil {
				return watchdog.PaneInfo{}, false
			}
			label := ""
			for _, ws := range snap.Workspaces {
				if ws.WorkspaceID == p.WorkspaceID {
					label = ws.Label
				}
			}
			return watchdog.PaneInfo{Title: p.DisplayTitle(), Workspace: label, Agent: p.AgentName()}, true
		},
		LastWrite: func(machineID, paneID string) (time.Time, bool) {
			p := paneOf(machineID, paneID)
			if p == nil {
				return time.Time{}, false
			}
			return lastWrite(machineID, p)
		},
		LastTools: func(machineID, paneID string, n int) []string {
			last, ok := lastTurn(machineID, paneID)
			if !ok {
				return nil
			}
			var out []string
			for _, part := range last.Parts {
				if part.Type == "tool" {
					out = append(out, part.ToolName+"\x00"+string(part.Input))
				}
			}
			if len(out) > n {
				out = out[len(out)-n:]
			}
			return out
		},
		Note:   tracker.SetAlert,
		Notify: pushSvc.Notify,
	})
	go wd.Run(time.Minute)

	taskStore, err := tasks.Open(dir)
	if err != nil {
		log.Fatalf("tasks: %v", err)
	}
	defer taskStore.Close()
	runs := &tasks.RunWatch{
		Store: taskStore,
		PaneState: func(machineID, paneID string) (string, time.Time, bool) {
			p, snap := paneAndSnap(machineID, paneID)
			if snap == nil {
				return "unknown", time.Time{}, true // not seen yet (a restart, a reconnect): not gone
			}
			if p == nil {
				return "", time.Time{}, false
			}
			d := tracker.Display(machineID, p)
			return d.State, time.UnixMilli(d.Since), true
		},
		// the text after the turn's last tool call: a turn ending in a tool call is a stop, not an answer
		LastAnswer: func(machineID, paneID string) (string, string) {
			last, ok := lastTurn(machineID, paneID)
			if !ok || len(last.IDs) == 0 {
				return "", ""
			}
			var texts []string
			for _, part := range last.Parts {
				switch part.Type {
				case "tool":
					texts = texts[:0]
				case "text":
					if t := strings.TrimSpace(part.Text); t != "" {
						texts = append(texts, t)
					}
				}
			}
			return strings.Join(texts, "\n\n"), last.IDs[len(last.IDs)-1]
		},
		Notify: pushSvc.Notify,
	}
	go runs.Loop(15 * time.Second)
	s := server.New(mgr, watcher, pushSvc, taskStore, code.New(*codePort), reviews, tracker, quick.New(dir), meter, auth.NewGuard(token), ui)
	s.Schedules = schedule.New(dir, s.SubmitToFolder, pushSvc.Notify)
	go s.Schedules.Loop()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	vitals := health.New(mgr, version, !*noCheck)
	vitals.Start(15 * time.Minute)
	s.Health = vitals
	var downMu sync.Mutex
	var lastDown string
	watcher.LocalDown = func(err error) {
		e := herdr.EnsureServer(bin, *socket)
		msg := ""
		if e != nil {
			msg = e.Error()
		}
		downMu.Lock()
		defer downMu.Unlock()
		if msg != lastDown {
			lastDown = msg
			if e != nil {
				log.Printf("herdr server: %v", e)
			} else {
				log.Printf("herdr server: answering again")
			}
		}
	}
	runner := jobs.New()
	runner.OnDone = func(j jobs.Job) {
		if mc, err := mgr.Get(j.Machine); err == nil {
			vitals.Refresh(mc)
		}
	}
	s.Jobs = runner
	restarter := restart.New(mgr)
	restarter.Later = func(mc *machines.Machine, title string, fn func(say func(string)) error) {
		runner.Run(mc, title, "", fn)
	}
	restarter.Lock = s.PaneLock
	watcher.OnEvent = restarter.OnStatus
	s.Restart = restarter
	if *tailscale {
		var node *tsnetx.Node
		s.Tailscale = func() tsnetx.Status { return node.Status() } // set before the handler is handed out
		node = tsnetx.Start(dir, *tsName, os.Getenv("TS_AUTHKEY"), s.Handler())
		defer node.Close()
	}
	log.Printf("remuda %s listening on http://%s", version, *addr)
	// the sign-in link goes to a person at a terminal or into the first run's log, never the service's log at every restart
	if created || interactive() {
		log.Printf("sign in with the token from `remuda -print-token`, or open:")
		log.Printf("  http://%s/?token=%s", *addr, token)
	} else {
		log.Printf("sign in with the token from `remuda -print-token`")
	}

	// log.Fatal would skip the deferred flush and close
	if err := server.ListenAndServe(ctx, *addr, s.Handler()); err != nil {
		log.Print(err)
	}
}

func searchCmd(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	q := fs.String("q", "", "text to find")
	dir := fs.String("dir", "", "only conversations in this folder")
	sinceS := fs.String("since", "", "only records after this time (RFC 3339)")
	limit := fs.Int("limit", 400, "stop after this many hits, newest first")
	_ = fs.Parse(args)
	if strings.TrimSpace(*q) == "" {
		fmt.Fprintln(os.Stderr, "remuda search -q <text> [-dir <folder>] [-since <RFC3339>] [-limit N]")
		os.Exit(2)
	}
	var since time.Time
	if *sinceS != "" {
		var err error
		if since, err = time.Parse(time.RFC3339, *sinceS); err != nil {
			fmt.Fprintln(os.Stderr, "bad -since:", err)
			os.Exit(2)
		}
	}
	home, _ := os.UserHomeDir()
	if err := search.Scan(filepath.Join(home, ".claude", "projects"), *q, *dir, since, *limit, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func usageCmd(args []string) {
	fs := flag.NewFlagSet("usage", flag.ExitOnError)
	sinceS := fs.String("since", "", "records from this time on (RFC 3339); default: 7 days")
	tz := fs.String("tz", "", "the zone days are cut in (IANA name); default: this machine's")
	_ = fs.Parse(args)
	since := time.Now().AddDate(0, 0, -7)
	if *sinceS != "" {
		var err error
		if since, err = time.Parse(time.RFC3339, *sinceS); err != nil {
			fmt.Fprintln(os.Stderr, "bad -since:", err)
			os.Exit(2)
		}
	}
	loc := time.Local
	if *tz != "" {
		if l, err := time.LoadLocation(*tz); err == nil {
			loc = l
		}
	}
	home, _ := os.UserHomeDir()
	rep, err := usage.Scan(filepath.Join(home, ".claude", "projects"), since, loc)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(rep)
}

// help is `remuda -h`: the server's flags, and the commands a person with nothing but the binary needs to know are there.
func help() {
	w := flag.CommandLine.Output()
	fmt.Fprint(w, `remuda — a browser and phone cockpit for the coding agents herdr runs

  remuda [flags]       the server (the background service runs this)
  remuda setup         herdr, its server, the agent hooks and the service, here or -host H
  remuda code          code-server for the Code mode, here or -host H
  remuda uninstall     take remuda off this machine again (-all: herdr and your data too)

Each command lists its own flags with -h. The server's:

`)
	flag.PrintDefaults()
}

// setupCmd is `remuda setup [-host H] [-tailscale] …`: herdr and its server, the agent integrations and the remuda service, here or over ssh.
func setupCmd(args []string) {
	fs := flag.NewFlagSet("setup", flag.ExitOnError)
	host := fs.String("host", "", "set up a remote machine over ssh instead of this one")
	ts := fs.Bool("tailscale", false, "the service joins your tailnet by itself (see -tailscale on the server)")
	tsName := fs.String("tailscale-hostname", "remuda", "the node's name on the tailnet")
	addr := fs.String("addr", "127.0.0.1:7317", "the service's loopback address")
	noService := fs.Bool("no-service", false, "herdr and its server only; no remuda service")
	open := fs.Bool("open", false, "open the page in the browser once the service answers (a Mac at the desk only)")
	fs.Parse(args)
	err := setup.Run(setup.Options{Host: *host, Tailscale: *ts, TSName: *tsName, AuthKey: os.Getenv("TS_AUTHKEY"), Addr: *addr, NoService: *noService, Version: version}, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *host == "" && !*noService {
		if dir, err := state.Dir(); err == nil {
			if token, _, err := auth.LoadOrCreateToken(dir); err == nil {
				page := "http://" + *addr + "/?token=" + url.QueryEscape(token)
				fmt.Printf("\nopen %s\n", page)
				if *open && answers(*addr, 15*time.Second) && openPage(page) {
					fmt.Println("(it is open in your browser)")
				}
			}
		}
	}
}

// uninstallCmd is `remuda uninstall [-host H] [-data] [-code] [-herdr] [-all] [-yes] [-n]`: what would go is shown first, then asked about.
func uninstallCmd(args []string) {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	host := fs.String("host", "", "a remote machine over ssh instead of this one")
	data := fs.Bool("data", false, "also your data: the token, tasks, schedules, push keys")
	codeToo := fs.Bool("code", false, "also code-server, with the editor's settings and extensions")
	herdrToo := fs.Bool("herdr", false, "also herdr: its hooks, its server and every agent running in it, the binary")
	all := fs.Bool("all", false, "everything: -data -code -herdr")
	yes := fs.Bool("yes", false, "do not ask")
	dry := fs.Bool("n", false, "only say what would be removed")
	fs.Parse(args)
	r := setup.Removal{Host: *host, Data: *data || *all, Code: *codeToo || *all, Herdr: *herdrToo || *all}
	some, err := r.Plan(os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "uninstall:", err)
		os.Exit(1)
	}
	if !some || *dry {
		return
	}
	if !*yes && !confirm("Remove these? [y/N] ") {
		fmt.Println("nothing removed")
		return
	}
	if err := r.Remove(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "uninstall: not everything could be removed:", err)
		os.Exit(1)
	}
}

// codeCmd is `remuda code [-host H] [-port N] [install|uninstall|status]`: code-server for the Code mode, by the script the binary carries.
func codeCmd(args []string) {
	fs := flag.NewFlagSet("code", flag.ExitOnError)
	host := fs.String("host", "", "a remote machine over ssh instead of this one")
	port := fs.Int("port", code.DefaultPort, "the loopback port code-server listens on (the server's -code-port must say the same)")
	fs.Parse(args)
	action := "install"
	if fs.NArg() > 0 {
		action = fs.Arg(0)
	}
	if action != "install" && action != "uninstall" && action != "status" {
		fmt.Fprintln(os.Stderr, "usage: remuda code [-host H] [-port N] [install|uninstall|status]")
		os.Exit(2)
	}
	if err := setup.Code(*host, action, *port, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "code:", err)
		os.Exit(1)
	}
}
