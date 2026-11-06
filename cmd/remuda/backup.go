package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/backup"
	"github.com/dev1antdev/remuda/internal/state"
)

const backupUsage = `usage: remuda backup now [-addr 127.0.0.1:7317]    a backup now, through the running service
       remuda backup list [-addr 127.0.0.1:7317]   the backups on the machine they go to
       remuda restore [-yes] <file>                 a backup back into this machine's state dir, the service stopped
Where and when the backups go is set in Settings › Backups.
`

// backupCmd is `remuda backup now|list`: the running service is asked, with the token of its state dir.
func backupCmd(args []string) {
	if len(args) == 0 || (args[0] != "now" && args[0] != "list") {
		fmt.Fprint(os.Stderr, backupUsage)
		os.Exit(2)
	}
	fs := flag.NewFlagSet("backup "+args[0], flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:7317", "the running service's loopback address")
	_ = fs.Parse(args[1:])
	dir, err := state.Dir()
	if err != nil {
		fail(err)
	}
	token, err := os.ReadFile(filepath.Join(dir, "token"))
	if err != nil {
		fail(fmt.Errorf("no token in %s: %w", dir, err))
	}
	call := func(method, path string, out any) {
		req, _ := http.NewRequest(method, "http://"+*addr+path, nil)
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
		client := &http.Client{Timeout: backup.Timeout*2 + 30*time.Second}
		res, err := client.Do(req)
		if err != nil {
			fail(fmt.Errorf("no remuda answers at %s: %w", *addr, err))
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		if res.StatusCode >= 300 {
			fail(fmt.Errorf("%s", strings.TrimSpace(string(body))))
		}
		if err := json.Unmarshal(body, out); err != nil {
			fail(err)
		}
	}
	if args[0] == "now" {
		var result backup.Result
		call("POST", "/api/backups/run", &result)
		fmt.Printf("backed up %d files, %s, to %s:%s in %.1f s", result.Files, megabytes(result.Size), result.Label, result.Path, result.Took)
		if result.Pruned > 0 {
			fmt.Printf("; %d older removed", result.Pruned)
		}
		fmt.Println()
		return
	}
	var list struct {
		Folder string        `json:"folder"`
		Files  []backup.File `json:"files"`
	}
	call("GET", "/api/backups/files", &list)
	fmt.Println(list.Folder)
	for _, file := range list.Files {
		fmt.Printf("  %s  %8s  %s\n", file.At.Local().Format("2006-01-02 15:04"), megabytes(file.Size), file.Name)
	}
	if len(list.Files) == 0 {
		fmt.Println("  no backup there yet")
	}
}

// restoreCmd is `remuda restore <file>`: refused while a remuda serves the state dir; the dir as it is moved aside first.
func restoreCmd(args []string) {
	fs := flag.NewFlagSet("restore", flag.ExitOnError)
	sure := fs.Bool("yes", false, "restore without being asked")
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		fmt.Fprint(os.Stderr, backupUsage)
		os.Exit(2)
	}
	dir, err := state.Dir()
	if err != nil {
		fail(err)
	}
	if state.Held(dir) {
		fail(fmt.Errorf("a remuda serves %s now; stop it first: %s", dir, stopHint()))
	}
	archive, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		fail(err)
	}
	manifest, _, err := backup.Read(archive)
	if err != nil {
		fail(err)
	}
	fmt.Printf("%s: %d files from %s, taken %s by remuda %s\n", fs.Arg(0), manifest.Files, manifest.Host, manifest.At.Local().Format("2006-01-02 15:04"), manifest.Version)
	if !*sure && !confirm(fmt.Sprintf("Put it in place of %s? The dir as it is now is moved aside, not deleted. [y/N] ", dir)) {
		os.Exit(1)
	}
	_, aside, err := backup.Restore(archive, dir, time.Now())
	if err != nil {
		fail(err)
	}
	if aside != "" {
		fmt.Println("the state dir as it was is at", aside)
	}
	fmt.Println("restored into", dir, "— start the service again:", startHint())
	fmt.Println("the copy holds remuda's state alone: on a new machine, set up its ~/.ssh (its key let in on the other machines, the hosts in its config) before the machines connect")
}

// stopHint and startHint are the service's own commands on this system, as install-service.sh sets it up
func stopHint() string {
	if runtime.GOOS == "darwin" {
		return "launchctl bootout gui/$(id -u)/dev.mardeus.remuda"
	}
	return "systemctl --user stop remuda"
}

func startHint() string {
	if runtime.GOOS == "darwin" {
		return "launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/dev.mardeus.remuda.plist"
	}
	return "systemctl --user start remuda"
}

func megabytes(size int64) string {
	if size < 1<<20 {
		return fmt.Sprintf("%d KB", (size+1023)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(size)/(1<<20))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "remuda:", err)
	os.Exit(1)
}
