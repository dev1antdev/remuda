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
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/dev1antdev/herdr-ui/internal/auth"
	"github.com/dev1antdev/herdr-ui/internal/code"
	"github.com/dev1antdev/herdr-ui/internal/herdr"
	"github.com/dev1antdev/herdr-ui/internal/machines"
	"github.com/dev1antdev/herdr-ui/internal/push"
	"github.com/dev1antdev/herdr-ui/internal/quick"
	"github.com/dev1antdev/herdr-ui/internal/radar"
	"github.com/dev1antdev/herdr-ui/internal/review"
	"github.com/dev1antdev/herdr-ui/internal/schedule"
	"github.com/dev1antdev/herdr-ui/internal/search"
	"github.com/dev1antdev/herdr-ui/internal/server"
	"github.com/dev1antdev/herdr-ui/internal/state"
	"github.com/dev1antdev/herdr-ui/internal/tasks"
	"github.com/dev1antdev/herdr-ui/internal/watch"
	"github.com/dev1antdev/herdr-ui/internal/watchdog"
	"github.com/dev1antdev/herdr-ui/web"
)

// version is set at build time: -ldflags "-X main.version=…".
var version = "dev"

func main() {
	// `herdr-ui search`: the transcript scanner, run over ssh on a remote
	// machine by the herdr-ui that serves the search (see internal/search)
	search.Version = version
	if len(os.Args) > 1 && os.Args[1] == "search" {
		searchCmd(os.Args[2:])
		return
	}
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
	// a pane's session file (following a compaction's continuation) and
	// when it was last written — the radar recovers an old pane's last turn
	// from it, the watchdog reads an agent's silence off it
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
		if v.MTime > 1e12 { // nanoseconds from a local stat, seconds from a remote one
			return time.Unix(0, v.MTime), true
		}
		return time.Unix(v.MTime, 0), true
	}
	tracker := radar.New(dir, func(machineID string, p *herdr.PaneInfo) time.Time {
		at, _ := lastWrite(machineID, p)
		return at
	})
	defer tracker.Flush()
	reviews := review.New(dir)
	watcher := watch.New(mgr, pushSvc, tracker)
	watcher.Badges, watcher.Dirs = reviews.Badges, server.WorkspaceDirs
	watcher.Start()

	// the watchdog: a minute apart, over the radar's episodes
	paneOf := func(machineID, paneID string) *herdr.PaneInfo {
		snap, _ := watcher.Snapshot(machineID)
		if snap == nil {
			return nil
		}
		for i := range snap.Panes {
			if snap.Panes[i].PaneID == paneID {
				return &snap.Panes[i]
			}
		}
		return nil
	}
	wd := watchdog.New(watchdog.Inputs{
		Episodes: tracker.Episodes,
		Pane: func(machineID, paneID string) (watchdog.PaneInfo, bool) {
			p := paneOf(machineID, paneID)
			if p == nil {
				return watchdog.PaneInfo{}, false
			}
			label := ""
			if snap, _ := watcher.Snapshot(machineID); snap != nil {
				for _, ws := range snap.Workspaces {
					if ws.WorkspaceID == p.WorkspaceID {
						label = ws.Label
					}
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
			p := paneOf(machineID, paneID)
			if p == nil {
				return nil
			}
			mc, path, ok := sessionFile(machineID, p)
			if !ok {
				return nil
			}
			v, err := mc.TranscriptVersion(path)
			if err != nil {
				return nil
			}
			turns, err := mc.Transcript(path, v)
			if err != nil || len(turns) == 0 {
				return nil
			}
			last := turns[len(turns)-1]
			if last.Role != "assistant" {
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
	// agents started on tasks: their finishes come back as comments
	runs := &tasks.RunWatch{
		Store: taskStore,
		PaneState: func(machineID, paneID string) (string, time.Time, bool) {
			if snap, _ := watcher.Snapshot(machineID); snap == nil {
				return "unknown", time.Time{}, true // not seen yet (a restart, a reconnect): not gone
			}
			p := paneOf(machineID, paneID)
			if p == nil {
				return "", time.Time{}, false
			}
			d := tracker.Display(machineID, p)
			return d.State, time.UnixMilli(d.Since), true
		},
		// what the agent's latest turn ended with: the text after its last
		// tool call (a turn ending in a tool call is a stop, not an answer),
		// and the record it ended with
		LastAnswer: func(machineID, paneID string) (string, string) {
			p := paneOf(machineID, paneID)
			if p == nil {
				return "", ""
			}
			mc, path, ok := sessionFile(machineID, p)
			if !ok {
				return "", ""
			}
			v, err := mc.TranscriptVersion(path)
			if err != nil {
				return "", ""
			}
			turns, err := mc.Transcript(path, v)
			if err != nil || len(turns) == 0 {
				return "", ""
			}
			last := turns[len(turns)-1]
			if last.Role != "assistant" || len(last.IDs) == 0 {
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
	s := server.New(mgr, watcher, pushSvc, taskStore, code.New(*codePort), reviews, tracker, auth.NewGuard(token), ui)
	// schedules: prompts on a timetable, delivered through the server's own
	// submit path to the agent in a folder
	s.Schedules = schedule.New(dir, s.SubmitToFolder, pushSvc.Notify)
	s.Quick = quick.New(dir)
	go s.Schedules.Loop()

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

func searchCmd(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	q := fs.String("q", "", "text to find")
	dir := fs.String("dir", "", "only conversations in this folder")
	sinceS := fs.String("since", "", "only records after this time (RFC 3339)")
	limit := fs.Int("limit", 400, "stop after this many hits, newest first")
	_ = fs.Parse(args)
	if strings.TrimSpace(*q) == "" {
		fmt.Fprintln(os.Stderr, "herdr-ui search -q <text> [-dir <folder>] [-since <RFC3339>] [-limit N]")
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
