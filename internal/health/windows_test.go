//go:build windows

package health

import (
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/wintest"
)

// TestOnWindowsTheVitalsAreReadAsACockpitReadsThem runs the vitals probe through the runner's OpenSSH, Git Bash its
// shell, and reads it with the parser a cockpit uses.
func TestOnWindowsTheVitalsAreReadAsACockpitReadsThem(t *testing.T) {
	wintest.Live(t)
	answer := wintest.Text(t, wintest.Remote(t), Script())
	wintest.Record(t, "vitals.txt", answer)
	r, peer := Parse(answer)
	t.Logf("%+v", r)
	if !strings.HasPrefix(r.OS, "Windows 10.0.") || !strings.Contains(r.Model, "Windows") || !strings.Contains(r.Model, "(build ") {
		t.Errorf("the system and its edition: %q, %q", r.OS, r.Model)
	}
	if r.MemTotalMB <= 0 || r.MemAvailMB <= 0 || r.MemAvailMB > r.MemTotalMB {
		t.Errorf("the memory: %d MB of %d MB available", r.MemAvailMB, r.MemTotalMB)
	}
	if r.DiskFreeGB <= 0 || r.DiskUsedPct <= 0 || r.Cores <= 0 || r.Arch != "amd64" || r.Uptime == "" {
		t.Errorf("disk %.1f GB free (%d%% used), %d cores, arch %q, up %q", r.DiskFreeGB, r.DiskUsedPct, r.Cores, r.Arch, r.Uptime)
	}
	if r.Herdr == "" || r.HerdrServer == "" || r.Remuda == "" {
		t.Errorf("herdr %q, its server %q, remuda %q", r.Herdr, r.HerdrServer, r.Remuda)
	}
	if peer != nil || r.Running {
		t.Error("remuda.exe serves nothing, so there is no peer to relay through")
	}
}
