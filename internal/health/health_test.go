package health

import "testing"

func TestParse(t *testing.T) {
	out := `os=macOS 26.6.2
claude=2.1.285 (Claude Code)
claude_path=/opt/homebrew/bin/claude
herdr=herdr 0.9.3
herdr_ui=herdr-ui v0.4.0-8-g3262e05
df=/dev/disk3s5   482797652 188483048 271706504    41% 1511806 2717065040    0%   /System/Volumes/Data
uptime=23:21  up 33 mins, 2 users, load averages: 6.52 3.32 3.43
`
	r := Parse(out)
	if r.OS != "macOS 26.6.2" || r.Claude != "2.1.285" || r.ClaudeVia != "brew" || r.Herdr != "0.9.3" || r.HerdrUI != "v0.4.0-8-g3262e05" {
		t.Fatalf("versions: %+v", r)
	}
	if r.DiskUsedPct != 41 || r.DiskFreeGB < 259 || r.DiskFreeGB > 260 {
		t.Fatalf("disk: %+v", r)
	}
	if r.Load1 != 6.52 || r.Uptime != "33 mins" {
		t.Fatalf("load/uptime: %+v", r)
	}
	// Linux: "up 5 days, 3:02, 1 user, load average: 0.52, 0.58, 0.59"
	l := Parse("os=Linux 6.8.0\nuptime= 10:11:12 up 5 days,  3:02,  1 user,  load average: 0.52, 0.58, 0.59\ndf=/dev/sda1 100000000 50000000 50000000 50% /\n")
	if l.Load1 != 0.52 || l.Uptime != "5 days,  3:02" || l.DiskUsedPct != 50 {
		t.Fatalf("linux: %+v", l)
	}
	if Parse("").Claude != "" {
		t.Fatal("empty")
	}
}

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"2.1.285", "2.1.295", true},
		{"2.1.295", "2.1.285", false},
		{"0.9.3", "v0.9.3", false},
		{"0.9.1", "v0.9.3", true},
		{"", "1.0.0", false},
		{"1.9.0", "2.0.0", true},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func TestInstalledVia(t *testing.T) {
	if installedVia("../Cellar/claude-code/2.1.285/bin/claude") != "brew" || installedVia("/opt/homebrew/bin/claude") != "brew" || installedVia("../lib/node_modules/@anthropic-ai/claude-code/cli.js") != "npm" || installedVia("/Users/x/.local/bin/claude") != "native" || installedVia("/Users/x/.nvm/versions/node/v22/bin/claude") != "npm" || installedVia("/usr/bin/claude") != "" {
		t.Fatal("via")
	}
}
