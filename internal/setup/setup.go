// Package setup is `herdr-ui setup`: what a machine needs to serve
// herdr-ui, done for it — herdr installed and its headless server running,
// its agent integrations, the herdr-ui service — locally or over ssh. The
// two POSIX scripts are embedded, so the binary alone is enough.
package setup

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed scripts/setup.sh
var setupScript string

//go:embed scripts/install-service.sh
var serviceScript string

// Options is what the scripts are told.
type Options struct {
	Host      string // "" is this machine; else an ssh destination
	Tailscale bool
	TSName    string
	AuthKey   string
	Addr      string
	NoService bool // herdr and its server only
}

// Run does the setup, writing what happens to out.
func Run(o Options, out io.Writer) error {
	env := []string{}
	if v := os.Getenv("HERDR_INSTALL"); v != "" {
		env = append(env, "HERDR_INSTALL="+v)
	}
	if o.Addr != "" {
		env = append(env, "HERDR_UI_ADDR="+o.Addr)
	}
	if o.Tailscale {
		env = append(env, "HERDR_UI_TAILSCALE=1")
		if o.TSName != "" {
			env = append(env, "HERDR_UI_TAILSCALE_HOSTNAME="+o.TSName)
		}
		if o.AuthKey != "" {
			env = append(env, "TS_AUTHKEY="+o.AuthKey)
		}
	}
	fmt.Fprintf(out, "— herdr and its server%s\n", where(o.Host))
	if err := run(o.Host, env, setupScript, nil, out); err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	if o.NoService {
		return nil
	}
	if o.Host == "" {
		// the service runs ~/.local/bin/herdr-ui: this very binary, put there
		if err := installSelf(out); err != nil {
			return fmt.Errorf("install herdr-ui: %w", err)
		}
	}
	fmt.Fprintf(out, "— the herdr-ui service%s\n", where(o.Host))
	if err := run(o.Host, env, serviceScript, []string{"install"}, out); err != nil {
		return fmt.Errorf("service: %w", err)
	}
	return nil
}

func where(host string) string {
	if host == "" {
		return " on this machine"
	}
	return " on " + host
}

// installSelf puts this binary at ~/.local/bin/herdr-ui when what is there
// is missing or different, so the service runs the one that ran setup.
func installSelf(out io.Writer) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dst := filepath.Join(home, ".local", "bin", "herdr-ui")
	if same, _ := sameFile(exe, dst); same {
		return nil
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	if have, err := os.ReadFile(dst); err == nil && bytes.Equal(have, data) {
		return nil
	}
	fmt.Fprintf(out, "— herdr-ui → %s\n", dst)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dst+".new", data, 0o755); err != nil {
		return err
	}
	return os.Rename(dst+".new", dst) // a running old one keeps its inode
}

func sameFile(a, b string) (bool, error) {
	sa, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	sb, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	return os.SameFile(sa, sb), nil
}

// run hands a script to `sh -c` — here, or on the host over ssh — with the
// environment through env(1) (ssh passes none through). By argument, not
// stdin: a child that reads stdin (an installer's prompt) must not eat
// the rest of the script.
func run(host string, env []string, script string, args []string, out io.Writer) error {
	var cmd *exec.Cmd
	if host == "" {
		cmd = exec.Command("sh", append([]string{"-c", script, "sh"}, args...)...)
		cmd.Env = append(os.Environ(), env...)
	} else {
		words := append([]string{"env"}, quoteAll(env)...)
		words = append(words, "sh", "-c", quote(script), "sh")
		for _, a := range args {
			words = append(words, quote(a))
		}
		cmd = exec.Command("ssh", "-o", "BatchMode=yes", host, strings.Join(words, " "))
	}
	cmd.Stdin = nil // /dev/null
	cmd.Stdout, cmd.Stderr = out, out
	return cmd.Run()
}

func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func quoteAll(ss []string) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		k, v, _ := strings.Cut(s, "=")
		out = append(out, k+"="+quote(v))
	}
	return out
}
