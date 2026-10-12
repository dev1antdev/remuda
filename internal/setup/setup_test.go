package setup

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Every embedded script parses: a typo in one is a machine half set up.
func TestScriptsParse(t *testing.T) {
	for name, script := range map[string]string{"setup": setupScript, "install-service": serviceScript, "install": installScript, "uninstall": uninstallScript, "install-code": codeScript} {
		cmd := exec.Command("sh", "-n")
		cmd.Stdin = strings.NewReader(script)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s.sh: %v\n%s", name, err, out)
		}
		// a backtick outside a comment is a command substitution — in an
		// unquoted heredoc too, where it once sat in a config file's text
		// and would have run `remuda code` from inside its own installer
		for i, line := range strings.Split(script, "\n") {
			if strings.Contains(line, "`") && !strings.HasPrefix(strings.TrimSpace(line), "#") {
				t.Errorf("%s.sh:%d: a backtick outside a comment: %s", name, i+1, line)
			}
		}
	}
}

func TestInstallEnvNamesThisBuildsReleaseOnly(t *testing.T) {
	t.Setenv("REMUDA_INSTALL_BASE", "")
	for version, want := range map[string]string{
		"v0.5.1":             "REMUDA_INSTALL_VERSION=v0.5.1",
		"v0.4.0-25-gae6199f": "", // between tags: no release of that name, the newest it is
		"v0.5.1-dirty":       "",
		"dev":                "",
		"":                   "",
	} {
		env := strings.Join(installEnv(version), " ")
		if !strings.Contains(env, "REMUDA_INSTALL_ONLY=1") || !strings.Contains(env, "REMUDA_INSTALL_MISSING=1") {
			t.Errorf("%q: the remote would be set up or overwritten: %s", version, env)
		}
		if got := strings.Contains(env, "REMUDA_INSTALL_VERSION="); got != (want != "") || (want != "" && !strings.Contains(env, want)) {
			t.Errorf("%q: env %q, want %q in it", version, env, want)
		}
	}
}

// home is a machine with nothing on it: a temp HOME, and nothing of the
// person's own named by the environment. Were a real herdr ever reached
// from here, it would find no server at the socket it is pointed at.
func home(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("the scripts are for macOS and Linux")
	}
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	for _, k := range []string{"XDG_CONFIG_HOME", "REMUDA_STATE_DIR", "REMUDA_INSTALL_VERSION", "REMUDA_INSTALL_BASE", "REMUDA_INSTALL_ONLY", "REMUDA_INSTALL_MISSING", "HERDR_PANE_ID"} {
		t.Setenv(k, "")
	}
	t.Setenv("HERDR_SOCKET_PATH", filepath.Join(dir, "no-herdr.sock"))
	t.Setenv("HERDR_SOCKET", filepath.Join(dir, "no-herdr.sock"))
	return dir
}

func write(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// --- the installer ---

const fakeBinary = "#!/bin/sh\necho remuda v9.9.9\n"

// release serves what a GitHub release does: the binary under every
// asset name, and checksums.txt saying sum for each.
func release(t *testing.T, sum string) string {
	t.Helper()
	if _, err := exec.LookPath("curl"); err != nil {
		if _, err := exec.LookPath("wget"); err != nil {
			t.Skip("neither curl nor wget here")
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/checksums.txt":
			for _, a := range []string{"remuda-darwin-arm64", "remuda-darwin-amd64", "remuda-linux-amd64", "remuda-linux-arm64"} {
				w.Write([]byte(sum + "  " + a + "\n"))
			}
		case strings.HasPrefix(r.URL.Path, "/remuda-"):
			w.Write([]byte(fakeBinary))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func sumOf(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// install runs the installer as `curl … | sh` does, for the binary alone:
// setup would reach for the machine's real herdr and service.
func install(t *testing.T, base string, env ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("sh", "-s")
	cmd.Stdin = strings.NewReader(installScript)
	cmd.Env = append(os.Environ(), append([]string{"REMUDA_INSTALL_ONLY=1", "REMUDA_INSTALL_BASE=" + base}, env...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestInstallerPutsTheCheckedBinaryInPlace(t *testing.T) {
	dir := home(t)
	out, err := install(t, release(t, sumOf(fakeBinary)))
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	bin := filepath.Join(dir, ".local", "bin", "remuda")
	got, err := os.ReadFile(bin)
	if err != nil || string(got) != fakeBinary {
		t.Fatalf("the binary is not in place: %v\n%s", err, out)
	}
	if fi, _ := os.Stat(bin); fi.Mode()&0o100 == 0 {
		t.Error("the binary cannot be run")
	}
	if !strings.Contains(out, "checksum ok") || !strings.Contains(out, "remuda v9.9.9") {
		t.Errorf("the installer did not say what it checked and installed:\n%s", out)
	}
	for _, left := range []string{"remuda.new", "remuda.sums"} {
		if exists(filepath.Join(dir, ".local", "bin", left)) {
			t.Errorf("%s was left behind", left)
		}
	}
}

func TestInstallerRefusesADownloadTheReleaseDoesNotVouchFor(t *testing.T) {
	dir := home(t)
	bin := filepath.Join(dir, ".local", "bin", "remuda")
	write(t, bin, "the one before", 0o755)
	out, err := install(t, release(t, sumOf("something else")))
	if err == nil {
		t.Fatalf("a wrong checksum went through:\n%s", out)
	}
	if !strings.Contains(out, "checksum mismatch") {
		t.Errorf("the refusal does not say why:\n%s", out)
	}
	if got, _ := os.ReadFile(bin); string(got) != "the one before" {
		t.Errorf("the installed binary was replaced by an unchecked one: %q", got)
	}
	for _, left := range []string{"remuda.new", "remuda.sums"} {
		if exists(filepath.Join(dir, ".local", "bin", left)) {
			t.Errorf("%s was left behind", left)
		}
	}
}

func TestInstallerLeavesARemudaThatIsThereWhenToldTo(t *testing.T) {
	dir := home(t)
	bin := filepath.Join(dir, ".local", "bin", "remuda")
	write(t, bin, "#!/bin/sh\necho remuda v0.0.1\n", 0o755)
	out, err := install(t, release(t, sumOf(fakeBinary)), "REMUDA_INSTALL_MISSING=1")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if got, _ := os.ReadFile(bin); !strings.Contains(string(got), "v0.0.1") {
		t.Errorf("a remote's own remuda was overwritten:\n%s", out)
	}
}

// --- the way back ---

// The programs the uninstall script calls that act on the machine rather
// than on files under HOME. Each stand-in writes down how it was called
// and answers as a machine with everything installed would.
var standIns = map[string]string{
	// loaded exactly when the service's file is there
	"launchctl": `case "$1" in print) [ -f "$HOME/Library/LaunchAgents/${2##*/}.plist" ]; exit $? ;; esac`,
	"systemctl": `case "$2" in is-active) [ -f "$HOME/.config/systemd/user/$3" ]; exit $? ;; esac`,
	"herdr": `case "$1 $2" in
"status server") if [ -f "$HOME/herdr-stopped" ]; then echo "status: not running"; else echo "status: running"; fi ;;
"integration status") printf 'pi: not installed (/x)\nclaude: current (v10) (/y)\ncodex: current (v8) (/z)\n' ;;
"server stop") : > "$HOME/herdr-stopped" ;;
esac`,
	"brew":  `exit 1`, // herdr did not come from Homebrew
	"ssh":   ``,
	"pgrep": `exit 1`, // no remuda started by hand
}

// machine lays a home out as the installer, `remuda setup` and `remuda
// code` leave it, and returns it with a reader of the stand-ins' calls.
func machine(t *testing.T) (string, func() string) {
	t.Helper()
	dir := home(t)
	bin := filepath.Join(dir, ".local", "bin")
	for name, body := range standIns {
		write(t, filepath.Join(bin, name), "#!/bin/sh\necho \""+name+" $*\" >> \"$HOME/calls.log\"\n"+body+"\n", 0o755)
	}
	// The stand-ins must be what the script finds. A real launchctl or
	// herdr reached from a test would act on the person's own machine, so
	// nothing runs unless every one of them resolves into this home.
	const pathLine = `PATH="$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:$PATH"`
	if !strings.Contains(uninstallScript, pathLine) {
		t.Fatal("the uninstall script no longer puts ~/.local/bin first on PATH: the stand-ins would not be found")
	}
	for name := range standIns {
		out, err := exec.Command("sh", "-c", pathLine+`; command -v "$1"`, "sh", name).Output()
		if got := strings.TrimSpace(string(out)); err != nil || got != filepath.Join(bin, name) {
			t.Fatalf("%s resolves to %q, not to its stand-in", name, got)
		}
	}

	write(t, filepath.Join(bin, "remuda"), fakeBinary, 0o755)
	write(t, filepath.Join(bin, "remuda.new"), "half a download", 0o644)
	state := stateDir(dir)
	write(t, filepath.Join(state, "token"), "t", 0o600)
	write(t, filepath.Join(state, "tasks.db"), "db", 0o600)
	write(t, filepath.Join(state, "tailscale", "node"), "n", 0o600)
	write(t, serviceFile(dir, "remuda"), "the service", 0o644)
	write(t, serviceFile(dir, "code-server"), "code-server — VS Code in the browser, for remuda", 0o644)
	if runtime.GOOS == "darwin" {
		write(t, filepath.Join(dir, "Library", "Logs", "remuda.log"), "log", 0o644)
	}
	write(t, filepath.Join(dir, ".local", "opt", "code-server-4.140.0", "bin", "code-server"), "", 0o755)
	write(t, filepath.Join(dir, ".local", "share", "code-server", "User", "settings.json"), "{}", 0o644)
	write(t, filepath.Join(dir, ".config", "code-server", "config.yaml"), "# written by remuda (`remuda code`)\n", 0o644)
	write(t, filepath.Join(dir, ".config", "herdr", "config.toml"), "", 0o644)
	write(t, filepath.Join(dir, ".local", "state", "herdr", "x"), "", 0o644)

	return dir, func() string {
		b, _ := os.ReadFile(filepath.Join(dir, "calls.log"))
		return string(b)
	}
}

func stateDir(home string) string {
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "remuda")
	}
	return filepath.Join(home, ".config", "remuda")
}

func serviceFile(home, name string) string {
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "LaunchAgents", "dev.mardeus."+name+".plist")
	}
	return filepath.Join(home, ".config", "systemd", "user", name+".service")
}

// stopsTheService is the call that takes the remuda service down here.
func stopsTheService() string {
	if runtime.GOOS == "darwin" {
		return "launchctl bootout gui/" // …/<uid>/dev.mardeus.remuda
	}
	return "systemctl --user disable --now remuda.service"
}

// The calls that change a machine beyond its files.
var acts = []string{"bootout", "unload", "disable", "integration uninstall", "server stop", "brew uninstall", "ssh -S"}

func TestPlanSaysWhatWouldGoAndTouchesNothing(t *testing.T) {
	dir, calls := machine(t)
	var out strings.Builder
	some, err := Removal{}.Plan(&out)
	if err != nil || !some {
		t.Fatalf("some=%v err=%v\n%s", some, err, out.String())
	}
	for _, want := range []string{"remove  the service", "remove  ~/.local/bin/remuda", "keep    your data", "(-data removes it)", "keep    code-server", "keep    herdr"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the plan lacks %q:\n%s", want, out.String())
		}
	}
	for _, f := range []string{filepath.Join(dir, ".local", "bin", "remuda"), serviceFile(dir, "remuda"), filepath.Join(stateDir(dir), "token")} {
		if !exists(f) {
			t.Errorf("a plan removed %s", f)
		}
	}
	for _, act := range acts {
		if strings.Contains(calls(), act) {
			t.Errorf("a plan called %q:\n%s", act, calls())
		}
	}

	out.Reset()
	if _, err := (Removal{Data: true, Code: true, Herdr: true}).Plan(&out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"remove  your data", "remove  code-server", "remove  herdr", "its hooks for: claude codex", "every agent and shell in its panes ends"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the plan for everything lacks %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "keep ") {
		t.Errorf("the plan for everything keeps something:\n%s", out.String())
	}
}

func TestRemovalTakesRemudaAndLeavesWhatIsNotItsOwn(t *testing.T) {
	dir, calls := machine(t)
	// an ssh master remuda left for a remote machine: a socket where it keeps them
	socks, err := os.MkdirTemp("/tmp", "rs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socks) })
	t.Setenv("TMPDIR", socks)
	ctl := filepath.Join(socks, "remuda-ssh-0123456789abcdef.ctl")
	l, err := net.Listen("unix", ctl)
	if err != nil {
		t.Fatal(err)
	}
	l.(*net.UnixListener).SetUnlinkOnClose(false)
	l.Close()

	var out strings.Builder
	if err := (Removal{}).Remove(&out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	gone := []string{filepath.Join(dir, ".local", "bin", "remuda"), filepath.Join(dir, ".local", "bin", "remuda.new"), serviceFile(dir, "remuda"), ctl}
	if runtime.GOOS == "darwin" {
		gone = append(gone, filepath.Join(dir, "Library", "Logs", "remuda.log"))
	}
	for _, f := range gone {
		if exists(f) {
			t.Errorf("%s is still there\n%s", f, out.String())
		}
	}
	for _, f := range []string{filepath.Join(stateDir(dir), "tasks.db"), serviceFile(dir, "code-server"), filepath.Join(dir, ".local", "opt", "code-server-4.140.0"), filepath.Join(dir, ".local", "bin", "herdr"), filepath.Join(dir, ".config", "herdr", "config.toml")} {
		if !exists(f) {
			t.Errorf("%s went without being asked for\n%s", f, out.String())
		}
	}
	log := calls()
	if !strings.Contains(log, stopsTheService()) || !strings.Contains(log, "ssh -S "+ctl+" -O exit") {
		t.Errorf("the service was not stopped or the ssh connection not closed:\n%s", log)
	}
	for _, line := range strings.Split(log, "\n") {
		herdrs := strings.Contains(line, "integration uninstall") || strings.Contains(line, "server stop")
		codes := strings.Contains(line, "code-server") && (strings.Contains(line, "bootout") || strings.Contains(line, "disable"))
		if herdrs || codes {
			t.Errorf("removing remuda alone called %q", line)
		}
	}
	if !strings.Contains(out.String(), "kept     your data") || !strings.Contains(out.String(), "kept     herdr") {
		t.Errorf("what stays is not said:\n%s", out.String())
	}
}

func TestRemovalOfEverythingLeavesNothing(t *testing.T) {
	dir, calls := machine(t)
	all := Removal{Data: true, Code: true, Herdr: true}
	var out strings.Builder
	if err := all.Remove(&out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	for _, f := range []string{
		filepath.Join(dir, ".local", "bin", "remuda"), serviceFile(dir, "remuda"), stateDir(dir),
		serviceFile(dir, "code-server"), filepath.Join(dir, ".local", "opt"), filepath.Join(dir, ".local", "share", "code-server"), filepath.Join(dir, ".config", "code-server"),
		filepath.Join(dir, ".local", "bin", "herdr"), filepath.Join(dir, ".config", "herdr"), filepath.Join(dir, ".local", "state", "herdr"),
	} {
		if exists(f) {
			t.Errorf("%s is still there\n%s", f, out.String())
		}
	}
	log := calls()
	hooks, stop := strings.Index(log, "herdr integration uninstall codex"), strings.Index(log, "herdr server stop")
	if !strings.Contains(log, "herdr integration uninstall claude") || hooks < 0 || stop < 0 {
		t.Fatalf("herdr's hooks or its server were left:\n%s", log)
	}
	if hooks > stop {
		t.Errorf("the server was stopped before the hooks were removed: a pane running this would end half way\n%s", log)
	}
	if strings.Contains(log, "integration uninstall pi") {
		t.Errorf("a hook that was never installed was removed:\n%s", log)
	}
	if !strings.Contains(out.String(), "Tailscale admin console") {
		t.Errorf("the tailnet node that stays listed is not mentioned:\n%s", out.String())
	}

	// the stand-in for herdr went with the rest, so the look afterwards asks
	// for remuda alone: with herdr's flag it would question whatever herdr
	// the machine running this test really has
	out.Reset()
	if some, err := (Removal{}).Plan(&out); err != nil || some {
		t.Errorf("after everything went there is still something to remove (some=%v err=%v):\n%s", some, err, out.String())
	}
}

// REMUDA_STATE_DIR can name any folder; only one holding remuda's own
// files is ever deleted.
func TestRemovalNeverDeletesAFolderThatIsNotItsState(t *testing.T) {
	dir, _ := machine(t)
	other := filepath.Join(dir, "Documents")
	write(t, filepath.Join(other, "thesis.txt"), "years", 0o644)
	t.Setenv("REMUDA_STATE_DIR", other)
	var out strings.Builder
	if err := (Removal{Data: true}).Remove(&out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if !exists(filepath.Join(other, "thesis.txt")) {
		t.Fatalf("a folder that is not remuda's state was deleted\n%s", out.String())
	}
	if !strings.Contains(out.String(), "none of remuda's files") {
		t.Errorf("why it stayed is not said:\n%s", out.String())
	}
}

func TestPlanFindsNothingOnAMachineWithoutRemuda(t *testing.T) {
	dir, _ := machine(t)
	for _, f := range []string{filepath.Join(dir, ".local", "bin", "remuda"), filepath.Join(dir, ".local", "bin", "remuda.new"), serviceFile(dir, "remuda")} {
		os.Remove(f)
	}
	if runtime.GOOS == "darwin" {
		os.Remove(filepath.Join(dir, "Library", "Logs", "remuda.log"))
	}
	var out strings.Builder
	some, err := Removal{}.Plan(&out)
	if err != nil || some {
		t.Fatalf("some=%v err=%v\n%s", some, err, out.String())
	}
	if !strings.Contains(out.String(), "what is kept goes with its flag") {
		t.Errorf("the plan does not say how the rest goes:\n%s", out.String())
	}
}

// byHand starts a process and has the pgrep stand-in report it for as
// long as it lives: a remuda the person started in a terminal. The
// returned channel closes when it has ended.
func byHand(t *testing.T, home string) <-chan struct{} {
	t.Helper()
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := strconv.Itoa(cmd.Process.Pid)
	write(t, filepath.Join(home, ".local", "bin", "pgrep"), "#!/bin/sh\nkill -0 "+pid+" 2>/dev/null && echo "+pid+"\n", 0o755)
	ended := make(chan struct{})
	go func() {
		cmd.Wait() // reaped at once: a zombie would still answer kill -0
		close(ended)
	}()
	t.Cleanup(func() { cmd.Process.Kill() })
	return ended
}

func running(ended <-chan struct{}) bool {
	select {
	case <-ended:
		return false
	case <-time.After(300 * time.Millisecond):
		return true
	}
}

func TestARemudaStartedByHandIsThePersonsToStop(t *testing.T) {
	dir, _ := machine(t)
	ended := byHand(t, dir)
	var out strings.Builder
	if _, err := (Removal{Data: true, Code: true}).Plan(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "note    a remuda started by hand runs too") {
		t.Errorf("the plan does not mention it:\n%s", out.String())
	}
	out.Reset()
	if err := (Removal{Data: true, Code: true}).Remove(&out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if !running(ended) {
		t.Fatalf("a process the person started was stopped without herdr going:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "it is yours to stop") {
		t.Errorf("that it still runs is not said:\n%s", out.String())
	}
}

// With herdr going it cannot stay: remuda starts herdr's server whenever
// none answers, and would bring back the one just stopped.
func TestARemudaStartedByHandStopsWhenHerdrGoes(t *testing.T) {
	dir, calls := machine(t)
	ended := byHand(t, dir)
	all := Removal{Data: true, Code: true, Herdr: true}
	var out strings.Builder
	if _, err := all.Plan(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "stop    a remuda started by hand") {
		t.Errorf("the plan does not say it will be stopped:\n%s", out.String())
	}
	if !running(ended) {
		t.Fatal("a plan stopped it")
	}
	out.Reset()
	if err := all.Remove(&out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if running(ended) {
		t.Fatalf("it still runs and will start herdr's server again:\n%s", out.String())
	}
	if !strings.Contains(calls(), "herdr server"+" stop") {
		t.Errorf("herdr's server was not stopped:\n%s", calls())
	}
}
