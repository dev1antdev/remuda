package setup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/health"
)

func TestCoreVersionMatchesTheScript(t *testing.T) {
	if !strings.Contains(coreScript, `CORE_VERSION:-`+CoreVersion+`}`) || !strings.Contains(coreScript, `[ "$VERSION" = "`+CoreVersion+`" ]`) {
		t.Fatalf("install-core.sh does not default to remuda-core %s, or pins its sums to another version", CoreVersion)
	}
	ci, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ci), "remuda-v"+CoreVersion+"\n") {
		t.Fatalf("CI installs another remuda-core release than the %s this binary pins", CoreVersion)
	}
}

// coreRelease is a release archive holding a remuda-core at version that logs every call but --version.
func coreRelease(t *testing.T, binary, version string) []byte {
	t.Helper()
	script := "#!/bin/sh\ncase \"$1\" in --version) echo \"rc " + version + "\" ;; *) echo \"new remuda-core $*\" >> \"$HOME/calls.log\" ;; esac\n"
	if binary == "remuda core.app/Contents/MacOS/remuda-core" {
		script += "if [ \"$1\" = --rc-install-bundle ]; then cp \"$2\" \"$3\"; fi\n"
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range map[string]string{binary: script, "LICENSE": "Apache-2.0", "conpty/conpty.dll": "dll", "conpty/x64/OpenConsole.exe": "exe"} {
		mode := int64(0o644)
		if name == binary {
			mode = 0o755
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

// coreHome is a home on a machine that says it is system/arch through a uname stand-in, with stand-ins for curl
// (handing out archive), loginctl, cygpath and Windows' own tar.exe first on PATH; nothing reaches GitHub or systemd.
func coreHome(t *testing.T, system, arch string, archive []byte) (string, func() string) {
	t.Helper()
	dir := home(t)
	bin := filepath.Join(dir, ".local", "bin")
	tools := filepath.Join(dir, "tools")
	write(t, filepath.Join(dir, "fixture"), string(archive), 0o644)
	for name, body := range map[string]string{
		"uname":    `case "$1" in -m) echo ` + arch + ` ;; *) echo ` + system + ` ;; esac`,
		"curl":     `while [ $# -gt 0 ]; do case "$1" in -o) out="$2"; shift ;; esac; shift; done; cp "$HOME/fixture" "$out"`,
		"loginctl": ``,
		"cygpath":  `echo "$2"`,
	} {
		write(t, filepath.Join(tools, name), "#!/bin/sh\necho \""+name+" $*\" >> \"$HOME/calls.log\"\n"+body+"\n", 0o755)
	}
	win := filepath.Join(dir, "Windows")
	write(t, filepath.Join(win, "System32", "tar.exe"), "#!/bin/sh\necho \"tar.exe $*\" >> \"$HOME/calls.log\"\nexec tar -xzf \"$2\" -C \"$4\"\n", 0o755)
	t.Setenv("SYSTEMROOT", win)
	t.Setenv("PATH", tools+":"+bin+":/usr/bin:/bin")
	sum := sumOf(string(archive))
	t.Setenv("CORE_SHA256", sum)
	return dir, func() string {
		b, _ := os.ReadFile(filepath.Join(dir, "calls.log"))
		return string(b)
	}
}

func installCore(t *testing.T, action string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := Core("", action, &out)
	return out.String(), err
}

func leftovers(t *testing.T, dir string) []string {
	t.Helper()
	left, _ := filepath.Glob(filepath.Join(dir, ".local", "remuda-core.*"))
	return left
}

func TestCoreInstallsTheCheckedReleaseOnLinux(t *testing.T) {
	dir, calls := coreHome(t, "Linux", "x86_64", coreRelease(t, "remuda-core", "0.2.3"))
	out, err := installCore(t, "install")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	core := filepath.Join(dir, ".local", "bin", "remuda-core")
	if got, _ := exec.Command(core, "--version").Output(); strings.TrimSpace(string(got)) != "rc 0.2.3" {
		t.Fatalf("remuda-core is not in place (%q):\n%s", got, out)
	}
	log := calls()
	if !strings.Contains(log, "curl -fsSL -o") || !strings.Contains(log, "/releases/download/remuda-v0.2.3/remuda-core-linux-x86_64.tar.gz") {
		t.Errorf("the pinned release was not the one fetched:\n%s", log)
	}
	if !strings.Contains(log, "loginctl --no-ask-password enable-linger") {
		t.Errorf("linger was not asked for, so the core would end with the last login:\n%s", log)
	}
	if strings.Contains(log, "new remuda-core") {
		t.Errorf("a fresh install ran the core:\n%s", log)
	}
	if _, err := exec.LookPath("rc"); err != nil {
		t.Errorf("no rc beside remuda-core:\n%s", out)
	} else if target, _ := os.Readlink(filepath.Join(dir, ".local", "bin", "rc")); target != "remuda-core" {
		t.Logf("an rc of the machine's own stands first on PATH; none was linked")
	}
	if left := leftovers(t, dir); len(left) > 0 {
		t.Errorf("the download was left behind: %v", left)
	}
	if !strings.Contains(out, "checksum ok") || !strings.Contains(out, "remuda-core 0.2.3 installed") {
		t.Errorf("the installer did not say what it checked and installed:\n%s", out)
	}
}

func TestCoreRefusesADownloadThePinDoesNotVouchFor(t *testing.T) {
	dir, _ := coreHome(t, "Linux", "aarch64", coreRelease(t, "remuda-core", "0.2.3"))
	t.Setenv("CORE_SHA256", sumOf("something else"))
	out, err := installCore(t, "install")
	if err == nil || !strings.Contains(out, "checksum mismatch") {
		t.Fatalf("a wrong checksum went through (err=%v):\n%s", err, out)
	}
	if exists(filepath.Join(dir, ".local", "bin", "remuda-core")) {
		t.Error("an unchecked remuda-core was installed")
	}
	if left := leftovers(t, dir); len(left) > 0 {
		t.Errorf("the download was left behind: %v", left)
	}
}

// installedCore puts a remuda-core at version in place, logging what it is asked.
func installedCore(t *testing.T, dir, name, version string) {
	t.Helper()
	write(t, filepath.Join(dir, ".local", "bin", name), "#!/bin/sh\ncase \"$1\" in --version) echo \"rc "+version+"\" ;; *) echo \"old remuda-core $*\" >> \"$HOME/calls.log\" ;; esac\n", 0o755)
}

func TestCoreUpdatesAnOlderOneThroughItsOwnSwap(t *testing.T) {
	dir, calls := coreHome(t, "Linux", "x86_64", coreRelease(t, "remuda-core", "0.2.3"))
	installedCore(t, dir, "remuda-core", "0.1.9")
	out, err := installCore(t, "install")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(calls(), "old remuda-core update --from "+filepath.Join(dir, ".local", "remuda-core.")) {
		t.Errorf("the installed core was not swapped through its own update, which keeps its terminals running:\n%s", calls())
	}
	if !strings.Contains(out, "updated from 0.1.9 to 0.2.3") {
		t.Errorf("the update is not said:\n%s", out)
	}
}

func TestCoreLeavesOneAsNewOrNewer(t *testing.T) {
	for _, version := range []string{"0.2.3", "0.3.1"} {
		t.Run(version, func(t *testing.T) {
			dir, calls := coreHome(t, "Linux", "x86_64", coreRelease(t, "remuda-core", "0.2.3"))
			installedCore(t, dir, "remuda-core", version)
			out, err := installCore(t, "install")
			if err != nil || !strings.Contains(out, "remuda-core "+version+" already at") {
				t.Fatalf("err=%v\n%s", err, out)
			}
			if strings.Contains(calls(), "curl") || strings.Contains(calls(), "old remuda-core") {
				t.Errorf("a current remuda-core was fetched or touched again:\n%s", calls())
			}
		})
	}
}

func TestCoreInstallsTheAppBundleOnAMac(t *testing.T) {
	dir, calls := coreHome(t, "Darwin", "arm64", coreRelease(t, "remuda core.app/Contents/MacOS/remuda-core", "0.2.3"))
	out, err := installCore(t, "install")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	core := filepath.Join(dir, ".local", "bin", "remuda-core")
	if !strings.Contains(calls(), "new remuda-core --rc-install-bundle") || !exists(core) {
		t.Fatalf("remuda core.app was not installed by its own installer:\n%s\n%s", calls(), out)
	}
	if !strings.Contains(calls(), "remuda-core-macos-aarch64.tar.gz") || strings.Contains(calls(), "loginctl") {
		t.Errorf("a Mac fetched another build, or was asked for linger:\n%s", calls())
	}
}

func TestCoreOnWindowsGoesBesideRemudaExeWithConPTY(t *testing.T) {
	dir, calls := coreHome(t, "MINGW64_NT-10.0-26100", "x86_64", coreRelease(t, "remuda-core.exe", "0.2.3"))
	out, err := installCore(t, "install")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	bin := filepath.Join(dir, ".local", "bin")
	for _, f := range []string{"remuda-core.exe", filepath.Join("conpty", "conpty.dll")} {
		if !exists(filepath.Join(bin, f)) {
			t.Errorf("%s is not beside remuda.exe:\n%s", f, out)
		}
	}
	log := calls()
	if !strings.Contains(log, "remuda-core-windows-x86_64.zip") || !strings.Contains(log, "tar.exe -xf") {
		t.Errorf("the zip was not fetched or not opened by Windows' own tar:\n%s", log)
	}
	if exists(filepath.Join(bin, "rc")) || strings.Contains(log, "loginctl") {
		t.Errorf("a Windows machine got an rc link or linger:\n%s", log)
	}
}

func TestCoreStatusSaysWhatIsThere(t *testing.T) {
	dir, calls := coreHome(t, "Linux", "x86_64", coreRelease(t, "remuda-core", "0.2.3"))
	if out, err := installCore(t, "status"); err != nil || !strings.Contains(out, "remuda-core is not installed here") {
		t.Fatalf("err=%v\n%s", err, out)
	}
	installedCore(t, dir, "remuda-core", "0.2.3")
	if out, err := installCore(t, "status"); err != nil || !strings.Contains(out, "remuda-core 0.2.3 at") {
		t.Fatalf("err=%v\n%s", err, out)
	}
	if strings.Contains(calls(), "curl") || strings.Contains(calls(), "old remuda-core") {
		t.Errorf("status fetched or ran something:\n%s", calls())
	}
}

func TestNoCoreForAMachineWithoutABuild(t *testing.T) {
	_, calls := coreHome(t, "MINGW64_NT-10.0-26100", "aarch64", coreRelease(t, "remuda-core.exe", "0.2.3"))
	out, err := installCore(t, "install")
	if err == nil || !strings.Contains(out, "has no build for") || strings.Contains(calls(), "curl") {
		t.Fatalf("err=%v\n%s\n%s", err, out, calls())
	}
}

func TestCoreOnWindowsWithoutAllOfItsConPTYIsRepaired(t *testing.T) {
	dir, calls := coreHome(t, "MINGW64_NT-10.0-26100", "x86_64", coreRelease(t, "remuda-core.exe", "0.2.3"))
	installedCore(t, dir, "remuda-core.exe", "0.2.3")
	write(t, filepath.Join(dir, ".local", "bin", "conpty", "conpty.dll"), "half a copy", 0o644)
	out, err := installCore(t, "install")
	if err != nil || !strings.Contains(out, "its ConPTY put back") {
		t.Fatalf("err=%v\n%s", err, out)
	}
	conpty := filepath.Join(dir, ".local", "bin", "conpty")
	if got, _ := os.ReadFile(filepath.Join(conpty, "conpty.dll")); string(got) != "dll" || !exists(filepath.Join(conpty, "x64", "OpenConsole.exe")) || exists(conpty+".new") {
		t.Errorf("ConPTY was not put back whole (%q):\n%s", got, out)
	}
	if strings.Contains(calls(), "old remuda-core") {
		t.Errorf("the core was swapped for its own version:\n%s", calls())
	}
	if !strings.Contains(health.Script(), `[ -f "$HOME/.local/bin/conpty/conpty.dll" ] && [ -f "$HOME/.local/bin/conpty/x64/OpenConsole.exe" ]`) {
		t.Error("the vitals count a Windows remuda-core whole by other files than its installer does")
	}
}

// coreMachine is machine(t) with remuda-core's cores in two sessions, the stand-in refusing to stop the one named stuck.
func coreMachine(t *testing.T, sessions ...string) (string, func() string) {
	t.Helper()
	dir, calls := machine(t)
	bin := filepath.Join(dir, ".local", "bin")
	write(t, filepath.Join(bin, "remuda-core"), "#!/bin/sh\necho \"RC_SESSION=$RC_SESSION remuda-core $*\" >> \"$HOME/calls.log\"\n[ \"$RC_SESSION\" != stuck ]\n", 0o755)
	for _, s := range sessions {
		write(t, filepath.Join(dir, ".local", "state", "remuda-core", s, "docs.json"), "{}", 0o600)
	}
	share := filepath.Join(dir, ".local", "share", "remuda-core", "current", "Contents", "MacOS", "remuda-core")
	write(t, share, "", 0o755)
	os.Remove(filepath.Join(bin, "rc"))
	if err := os.Symlink(share, filepath.Join(bin, "rc")); err != nil {
		t.Fatal(err)
	}
	return dir, calls
}

func TestRemovalOfCoreStopsEverySessionFirst(t *testing.T) {
	dir, calls := coreMachine(t, "work")
	var out strings.Builder
	if _, err := (Removal{Core: true}).Plan(&out); err != nil || !strings.Contains(out.String(), "sessions: default, work") {
		t.Fatalf("the plan does not name the cores that stop (err=%v):\n%s", err, out.String())
	}
	out.Reset()
	if err := (Removal{Core: true}).Remove(&out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	for _, s := range []string{"default", "work"} {
		if !strings.Contains(calls(), "RC_SESSION="+s+" remuda-core server stop") {
			t.Errorf("the %s core was not stopped:\n%s", s, calls())
		}
	}
	for _, f := range []string{"bin/remuda-core", "bin/rc", "share/remuda-core", "state/remuda-core"} {
		if exists(filepath.Join(dir, ".local", f)) {
			t.Errorf("~/.local/%s is still there\n%s", f, out.String())
		}
	}
}

func TestRemovalOfCoreStopsAHiddenSessionAndKeepsEverythingWhileACoreStillRuns(t *testing.T) {
	dir, calls := coreMachine(t, ".work", "..work")
	write(t, filepath.Join(dir, ".local", "bin", "pgrep"), "#!/bin/sh\ncase \"$*\" in *remuda-core*) echo 4242 ;; *) exit 1 ;; esac\n", 0o755)
	var out strings.Builder
	err := (Removal{Core: true}).Remove(&out)
	for _, s := range []string{".work", "..work"} {
		if !strings.Contains(calls(), "RC_SESSION="+s+" remuda-core server stop") {
			t.Errorf("the hidden session %s's core was not stopped:\n%s", s, calls())
		}
	}
	if err == nil || !strings.Contains(out.String(), "a remuda-core still runs (pid 4242") {
		t.Fatalf("a core still running went unsaid (err=%v):\n%s", err, out.String())
	}
	for _, f := range []string{"bin/remuda-core", "state/remuda-core/.work"} {
		if !exists(filepath.Join(dir, ".local", f)) {
			t.Errorf("~/.local/%s went while a core still ran\n%s", f, out.String())
		}
	}
}

func TestRemovalOfCoreLeavesItsFilesWhileACoreRuns(t *testing.T) {
	dir, _ := coreMachine(t, "stuck")
	var out strings.Builder
	err := (Removal{Core: true}).Remove(&out)
	if err == nil || !strings.Contains(out.String(), "remuda-core's stuck core did not stop") {
		t.Fatalf("a core that did not stop went unsaid (err=%v):\n%s", err, out.String())
	}
	for _, f := range []string{"bin/remuda-core", "state/remuda-core/stuck", "state/remuda-core/default"} {
		if !exists(filepath.Join(dir, ".local", f)) {
			t.Errorf("~/.local/%s went while a core still ran\n%s", f, out.String())
		}
	}
}

func TestRemovalOfCoreKeepsEverythingWhenItCannotTellACoreRuns(t *testing.T) {
	dir, _ := coreMachine(t)
	write(t, filepath.Join(dir, ".local", "bin", "pgrep"), "#!/bin/sh\ncase \"$*\" in *remuda-core*) exit 3 ;; *) exit 1 ;; esac\n", 0o755)
	var out strings.Builder
	err := (Removal{Core: true}).Remove(&out)
	if err == nil || !strings.Contains(out.String(), "could not tell whether a remuda-core still runs") {
		t.Fatalf("a process check that failed went unsaid (err=%v):\n%s", err, out.String())
	}
	if !exists(filepath.Join(dir, ".local", "state", "remuda-core", "default")) {
		t.Errorf("remuda-core's state went though nobody could tell its core had stopped\n%s", out.String())
	}
}

func TestCoreInstallsOneAtATime(t *testing.T) {
	dir, calls := coreHome(t, "Linux", "x86_64", coreRelease(t, "remuda-core", "0.2.3"))
	lock := filepath.Join(dir, ".local", "bin", ".remuda-core-install")
	write(t, filepath.Join(lock, "pid"), "4242", 0o644)
	out, err := installCore(t, "install")
	if err == nil || !strings.Contains(out, "another remuda-core install runs here (pid 4242); if none does, remove "+lock) || strings.Contains(calls(), "curl") {
		t.Fatalf("an install went on beside another (err=%v):\n%s", err, out)
	}
	if !exists(lock) {
		t.Fatal("the other install's lock was taken away")
	}
	os.RemoveAll(lock)
	if out, err := installCore(t, "install"); err != nil || !strings.Contains(out, "remuda-core 0.2.3 installed") {
		t.Fatalf("err=%v\n%s", err, out)
	}
	if exists(lock) {
		t.Error("the install left its lock behind")
	}
}
