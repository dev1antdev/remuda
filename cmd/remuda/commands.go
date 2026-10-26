package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/code"
	"github.com/dev1antdev/remuda/internal/search"
	"github.com/dev1antdev/remuda/internal/setup"
	"github.com/dev1antdev/remuda/internal/state"
	"github.com/dev1antdev/remuda/internal/usage"
)

// commands are the subcommands beside the server: setup, uninstall and code for people, search and usage for a remuda on a
// remote machine, mcp and tasks for the coding agents (and a person at a shell) to reach the task list.
var commands = map[string]func(args []string){
	"setup":     setupCmd,
	"uninstall": uninstallCmd,
	"code":      codeCmd,
	"search":    searchCmd,
	"usage":     usageCmd,
	"mcp":       mcpCmd,
	"tasks":     tasksCmd,
}

// searchCmd is the scanner a remote machine's remuda runs for the search lens.
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

// usageCmd is the cost report a remote machine's remuda runs for the usage lens.
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
