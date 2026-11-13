package updates

import (
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/health"
)

func TestAWindowsMachineGetsTheReleasesExeAndNoServiceRestart(t *testing.T) {
	tag := "v0.14.0"
	base := "https://github.com/" + health.ReleasesRepo + "/releases/download/" + tag + "/"
	rel := health.Release{Version: "0.14.0", Tag: tag, Checksums: base + "checksums.txt",
		Assets: map[string]string{"remuda-windows-amd64.exe": base + "remuda-windows-amd64.exe", "remuda-linux-amd64": base + "remuda-linux-amd64"}}
	u, ok := ReleaseUpdate("v0.13.0", "Windows 10.0.26100", "amd64", rel, false)
	if !ok {
		t.Fatal("no release offered to a Windows machine")
	}
	for _, want := range []string{"remuda-windows-amd64.exe", `"$HOME/.local/bin/remuda.new.exe"`, `mv -f "$b/remuda.exe" "$b/remuda.exe.old.$$"`, "checksum ok"} {
		if !strings.Contains(u.Command, want) {
			t.Errorf("the command lacks %q:\n%s", want, u.Command)
		}
	}
	if strings.Contains(u.Command, "launchctl") || strings.Contains(u.Command, "systemctl") || !strings.Contains(u.Note, "no service on Windows") {
		t.Errorf("a Windows machine has no service to restart:\n%s\n%s", u.Command, u.Note)
	}
	if u, _ := ReleaseUpdate("v0.13.0", "Linux 6.8", "amd64", rel, false); !strings.Contains(u.Command, "systemctl") || strings.Contains(u.Command, ".exe") {
		t.Errorf("a Linux machine's release is as before:\n%s", u.Command)
	}
}

func TestHerdrOnWindowsUpdatesWithoutAHandoff(t *testing.T) {
	u, err := Plan("herdr", &health.Report{OS: "Windows 10.0.26100", Herdr: "0.9.4", HerdrVia: "installer"})
	if err != nil || strings.Contains(u.Command, "--handoff") || !strings.Contains(u.Command, "herdr update &&") {
		t.Fatalf("%v: %s", err, u.Command)
	}
	if u, _ := Plan("herdr", &health.Report{OS: "Linux 6.8", Herdr: "0.9.4", HerdrVia: "installer"}); !strings.Contains(u.Command, "herdr update --handoff") {
		t.Fatalf("Linux keeps its handoff: %s", u.Command)
	}
}
