// Package setup is `remuda setup`, `remuda uninstall` and `remuda code`:
// embedded POSIX scripts run here or over ssh, so the binary alone is
// enough and nothing needs a checkout.
package setup

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

//go:embed scripts/setup.sh
var setupScript string

//go:embed scripts/install-service.sh
var serviceScript string

// installScript is the installer people pipe to sh; here it puts the binary on a remote that has none.
//
//go:embed scripts/install.sh
var installScript string

//go:embed scripts/uninstall.sh
var uninstallScript string

//go:embed scripts/install-code.sh
var codeScript string

// Options is what the scripts are told.
type Options struct {
	Host      string // "" is this machine; else an ssh destination
	Tailscale bool
	TSName    string
	AuthKey   string
	Addr      string
	NoService bool   // herdr and its server only
	Version   string // this build's: a remote without remuda gets the same release when it is one
}

// Run does the setup, writing what happens to out.
func Run(o Options, out io.Writer) error {
	env := []string{}
	if v := os.Getenv("HERDR_INSTALL"); v != "" {
		env = append(env, "HERDR_INSTALL="+v)
	}
	if o.Addr != "" {
		env = append(env, "REMUDA_ADDR="+o.Addr)
	}
	if o.Tailscale {
		env = append(env, "REMUDA_TAILSCALE=1")
		if o.TSName != "" {
			env = append(env, "REMUDA_TAILSCALE_HOSTNAME="+o.TSName)
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
		if err := installSelf(out); err != nil {
			return fmt.Errorf("install remuda: %w", err)
		}
	} else {
		fmt.Fprintf(out, "— remuda%s\n", where(o.Host))
		if err := run(o.Host, installEnv(o.Version), installScript, nil, out); err != nil {
			return fmt.Errorf("install remuda there: %w (for a machine of this one's OS and CPU, \"Deploy this remuda there\" in the page's Updates copies this binary over ssh instead)", err)
		}
	}
	fmt.Fprintf(out, "— the remuda service%s\n", where(o.Host))
	if err := run(o.Host, env, serviceScript, []string{"install"}, out); err != nil {
		return fmt.Errorf("service: %w", err)
	}
	return nil
}

var releaseTag = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// installEnv tells the installer to fetch the binary alone, only when none is there,
// and this build's own release when it is one (a build between tags gets the newest).
func installEnv(version string) []string {
	env := []string{"REMUDA_INSTALL_ONLY=1", "REMUDA_INSTALL_MISSING=1"}
	if releaseTag.MatchString(version) {
		env = append(env, "REMUDA_INSTALL_VERSION="+version)
	}
	if v := os.Getenv("REMUDA_INSTALL_BASE"); v != "" {
		env = append(env, "REMUDA_INSTALL_BASE="+v)
	}
	return env
}

// Removal is `remuda uninstall`: remuda itself always, and what the person named besides.
type Removal struct {
	Host  string // "" is this machine; else an ssh destination
	Data  bool   // the state dir: token, tasks, schedules, push keys
	Code  bool   // code-server, with the editor's settings and extensions
	Herdr bool   // herdr: its hooks, its server and the agents in it, the binary
}

func (r Removal) env() []string {
	env := []string{}
	for k, on := range map[string]bool{"REMUDA_RM_DATA": r.Data, "REMUDA_RM_CODE": r.Code, "REMUDA_RM_HERDR": r.Herdr} {
		if on {
			env = append(env, k+"=1")
		}
	}
	return env
}

// nothingToRemove is the plan's exit status when it found nothing that would go.
const nothingToRemove = 10

// Plan prints what Remove would take and what it would leave, and whether there is anything to remove.
func (r Removal) Plan(out io.Writer) (bool, error) {
	err := run(r.Host, r.env(), uninstallScript, []string{"plan"}, out)
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == nothingToRemove {
		return false, nil
	}
	return err == nil, err
}

// Remove does what Plan said, the local script in a session of its own: typed into
// a herdr pane, -herdr stops that pane's server and herdr ends the pane's process group.
func (r Removal) Remove(out io.Writer) error {
	cmd := command(r.Host, r.env(), uninstallScript, []string{"run"}, out)
	if r.Host == "" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	}
	return cmd.Run()
}

// Code runs code-server's installer (install, uninstall or status) here or on a host.
func Code(host, action string, port int, out io.Writer) error {
	env := []string{}
	if port > 0 {
		env = append(env, "CODE_PORT="+strconv.Itoa(port))
	}
	return run(host, env, codeScript, []string{action}, out)
}

func where(host string) string {
	if host == "" {
		return " on this machine"
	}
	return " on " + host
}

// installSelf puts this binary at ~/.local/bin/remuda when what is there is missing or different.
func installSelf(out io.Writer) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dst := filepath.Join(home, ".local", "bin", "remuda")
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
	fmt.Fprintf(out, "— remuda → %s\n", dst)
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

// run hands a script to `sh -c`, here or on the host over ssh, by argument and
// not stdin: a child that reads stdin (an installer's prompt) must not eat the rest.
func run(host string, env []string, script string, args []string, out io.Writer) error {
	return command(host, env, script, args, out).Run()
}

// command is run's process, not started yet.
func command(host string, env []string, script string, args []string, out io.Writer) *exec.Cmd {
	// a secret never goes on a command line (ps shows it, on both ends): the
	// Tailscale auth key travels on stdin, the one line the script reads first
	var stdin io.Reader
	kept := make([]string, 0, len(env))
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "TS_AUTHKEY="); ok && v != "" {
			script = "IFS= read -r TS_AUTHKEY; export TS_AUTHKEY; " + script
			stdin = strings.NewReader(v + "\n")
			continue
		}
		kept = append(kept, kv)
	}
	env = kept
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
	cmd.Stdin = stdin // nil: /dev/null
	cmd.Stdout, cmd.Stderr = out, out
	return cmd
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
