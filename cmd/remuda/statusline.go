package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/limits"
	"github.com/dev1antdev/remuda/internal/setup"
	"github.com/dev1antdev/remuda/internal/state"
)

// statuslineCmd is `remuda statusline`: as Claude Code's status line, it keeps the rate limits Claude Code hands it and
// runs the person's own status line; `remuda statusline install|status|uninstall [-host H]` puts it in front of that
// status line in Claude Code's settings, says how it stands, or takes it away again.
func statuslineCmd(args []string) {
	action := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		action, args = args[0], args[1:]
	}
	switch action {
	case "":
		dir, err := state.Dir()
		if err != nil {
			return // a status line never fails: without a state dir there is just nothing to keep
		}
		limits.RunStatusline(os.Stdin, os.Stdout, dir, time.Now())
	case "install", "status", "uninstall":
		fs := flag.NewFlagSet("statusline "+action, flag.ExitOnError)
		host := fs.String("host", "", "a remote machine over ssh instead of this one")
		fs.Parse(args)
		if *host != "" {
			if err := setup.Statusline(*host, action, os.Stdout); err != nil {
				fmt.Fprintln(os.Stderr, "statusline:", err)
				os.Exit(1)
			}
			return
		}
		words, err := statuslineHere(action)
		if err != nil {
			fmt.Fprintln(os.Stderr, "statusline:", err)
			os.Exit(1)
		}
		fmt.Println(words)
	default:
		fmt.Fprint(os.Stderr, `usage: remuda statusline                       what Claude Code runs as its status line once installed
       remuda statusline install [-host H]    keep Claude Code's rate limits for remuda, your status line still shown
       remuda statusline status|uninstall [-host H]
`)
		os.Exit(2)
	}
}

func statuslineHere(action string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir, err := state.Dir()
	if err != nil {
		return "", err
	}
	switch action {
	case "install":
		return limits.InstallStatusline(home, dir, selfPath(home))
	case "uninstall":
		return limits.UninstallStatusline(home, dir)
	}
	return limits.StatuslineStatus(home, dir)
}

// selfPath is the remuda the status line runs: the installed one, else this very binary.
// On Windows it is remuda.exe, spelled with forward slashes: Claude Code runs its status line through Git Bash there,
// which would read a backslash as an escape.
func selfPath(home string) string {
	name := "remuda"
	if runtime.GOOS == "windows" {
		name = "remuda.exe"
	}
	path := filepath.Join(home, ".local", "bin", name)
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		if exe, err := os.Executable(); err == nil {
			path = exe
		}
	}
	return filepath.ToSlash(path)
}
