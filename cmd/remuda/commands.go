package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/code"
	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/search"
	"github.com/dev1antdev/remuda/internal/setup"
	"github.com/dev1antdev/remuda/internal/state"
	"github.com/dev1antdev/remuda/internal/transcript"
	"github.com/dev1antdev/remuda/internal/usage"
)

// rootFlags are the session folders an agent named for itself, as a scan command receives them from the cockpit.
type rootFlags struct {
	roots transcript.Roots
}

func (r *rootFlags) String() string { return "" }

func (r *rootFlags) Set(value string) error {
	name, dir, ok := strings.Cut(value, "=")
	format := transcript.Format(name)
	if !ok || dir == "" || !rpath.Absolute(dir) || format != transcript.Claude && format != transcript.Codex {
		return fmt.Errorf("a root is claude=<absolute folder> or codex=<absolute folder>")
	}
	if r.roots == nil {
		r.roots = transcript.Roots{}
	}
	r.roots[format] = append(r.roots[format], dir)
	return nil
}

// forHome is the machine's own roots with the ones its agents named for themselves.
func (r *rootFlags) forHome(home string) transcript.Roots {
	roots := transcript.RootsOf(home)
	for format, dirs := range r.roots {
		roots[format] = append(roots[format], dirs...)
	}
	return roots
}

// commands are the subcommands beside the server: setup, uninstall and code for people, search and usage for a remuda on a
// remote machine, mcp and tasks for the coding agents (and a person at a shell) to reach the task list.
var commands = map[string]func(args []string){
	"setup":      setupCmd,
	"uninstall":  uninstallCmd,
	"code":       codeCmd,
	"core":       coreCmd,
	"search":     searchCmd,
	"usage":      usageCmd,
	"mcp":        mcpCmd,
	"tasks":      tasksCmd,
	"statusline": statuslineCmd,
	"backup":     backupCmd,
	"restore":    restoreCmd,
}

// searchCmd is the scanner a remote machine's remuda runs for the search lens.
func searchCmd(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	q := fs.String("q", "", "text to find")
	dir := fs.String("dir", "", "only conversations in this folder")
	sinceS := fs.String("since", "", "only records after this time (RFC 3339)")
	limit := fs.Int("limit", 400, "stop after this many hits, newest first")
	var roots rootFlags
	fs.Var(&roots, "root", "a session folder an agent named for itself, format=folder")
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
	if err := search.ScanRoots(roots.forHome(home), *q, *dir, since, *limit, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// usageCmd is the cost report a remote machine's remuda runs for the usage lens.
func usageCmd(args []string) {
	fs := flag.NewFlagSet("usage", flag.ExitOnError)
	sinceS := fs.String("since", "", "records from this time on (RFC 3339); default: 7 days")
	tz := fs.String("tz", "", "the zone days are cut in (IANA name); default: this machine's")
	daily := fs.Bool("daily", false, "every day broken down by folder too, for the year's heatmap")
	var roots rootFlags
	fs.Var(&roots, "root", "a session folder an agent named for itself, format=folder")
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
	scan := usage.ScanRoots
	if *daily {
		scan = usage.ScanRootsDays
	}
	rep, err := scan(roots.forHome(home), since, loc)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(rep)
}

// setupCmd is `remuda setup [-host H] [-tailscale] …`: remuda-core and the remuda service, here or over ssh.
func setupCmd(args []string) {
	fs := flag.NewFlagSet("setup", flag.ExitOnError)
	host := fs.String("host", "", "set up a remote machine over ssh instead of this one")
	ts := fs.Bool("tailscale", false, "the service joins your tailnet by itself (see -tailscale on the server)")
	tsName := fs.String("tailscale-hostname", "remuda", "the node's name on the tailnet")
	addr := fs.String("addr", "127.0.0.1:7317", "the service's loopback address")
	noService := fs.Bool("no-service", false, "remuda-core only; no remuda service")
	open := fs.Bool("open", false, "open the page in the browser once the service answers (a Mac at the desk only)")
	serverOnly := fs.Bool("server", false, "the service serves the page only (-server): no remuda-core installed there, the machine none of the cockpit's")
	fs.Parse(args)
	err := setup.Run(setup.Options{Host: *host, Tailscale: *ts, TSName: *tsName, AuthKey: os.Getenv("TS_AUTHKEY"), Addr: *addr, NoService: *noService, Server: *serverOnly, Version: version}, os.Stdout)
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

// uninstallCmd is `remuda uninstall [-host H] [-data] [-code] [-core] [-herdr] [-all] [-yes] [-n]`: what would go is shown first, then asked about.
func uninstallCmd(args []string) {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	host := fs.String("host", "", "a remote machine over ssh instead of this one")
	data := fs.Bool("data", false, "also your data: the token, tasks, schedules, push keys")
	codeToo := fs.Bool("code", false, "also code-server, with the editor's settings and extensions")
	coreToo := fs.Bool("core", false, "also remuda-core: its core and every terminal running in it, the binary, its folders")
	herdrToo := fs.Bool("herdr", false, "also herdr: its hooks, its server and every agent running in it, the binary")
	all := fs.Bool("all", false, "everything: -data -code -core -herdr")
	yes := fs.Bool("yes", false, "do not ask")
	dry := fs.Bool("n", false, "only say what would be removed")
	fs.Parse(args)
	r := setup.Removal{Host: *host, Data: *data || *all, Code: *codeToo || *all, Core: *coreToo || *all, Herdr: *herdrToo || *all}
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

// coreCmd is `remuda core [-host H] [install|status]`: remuda-core, the pinned release, by the script the binary carries;
// `remuda uninstall -core` takes it away.
func coreCmd(args []string) {
	fs := flag.NewFlagSet("core", flag.ExitOnError)
	host := fs.String("host", "", "a remote machine over ssh instead of this one")
	fs.Parse(args)
	action := "install"
	if fs.NArg() > 0 {
		action = fs.Arg(0)
	}
	if action != "install" && action != "status" {
		fmt.Fprintln(os.Stderr, "usage: remuda core [-host H] [install|status] (remuda uninstall -core takes it away)")
		os.Exit(2)
	}
	if err := setup.Core(*host, action, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "core:", err)
		os.Exit(1)
	}
}

// terminalSource is what a task an agent makes here is marked with: the core terminal it runs in, else the herdr
// pane, so a task list read while both run still names the right one.
func terminalSource() string {
	if id := os.Getenv("RC_TERMINAL_ID"); id != "" {
		return id
	}
	return os.Getenv("HERDR_PANE_ID")
}
