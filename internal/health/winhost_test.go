package health

import "testing"

func TestAWindowsMachineIsNamedInWindowsWords(t *testing.T) {
	if name, ok := windowsName("MINGW64_NT-10.0-26100 3.6.10-5a1665c8.x86_64"); !ok || name != "Windows 10.0.26100" {
		t.Errorf("windowsName = %q, %v", name, ok)
	}
	if _, ok := windowsName("Linux 6.8.0-1017-raspi"); ok {
		t.Error("Linux is no Windows")
	}
	for in, want := range map[string]string{
		"Windows Server 2025 Datacenter|26100": "Windows Server 2025 Datacenter (build 26100)",
		"Windows 10 Pro|26100":                 "Windows 11 Pro (build 26100)",
		"Windows 10 Home|19045":                "Windows 10 Home (build 19045)",
		"|":                                    "",
		"Windows 10 Pro|":                      "Windows 10 Pro",
	} {
		if got := windowsEdition(in); got != want {
			t.Errorf("windowsEdition(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAWindowsUptimeIsSaidAsBSDsUptimeSaysIt(t *testing.T) {
	var r Report
	parseWindowsUp(&r, "277443.5 1.26")
	if r.Uptime != "3 days, 5:04" || r.Load1 != 1.26 {
		t.Errorf("read %q, load %v", r.Uptime, r.Load1)
	}
	for seconds, want := range map[int]string{90000: "1 day, 1:00", 720: "12 mins", 18180: "5:03"} {
		if got := uptimeWords(seconds); got != want {
			t.Errorf("uptimeWords(%d) = %q, want %q", seconds, got, want)
		}
	}
}

func TestWindowsFreeMemoryIsItsAvailableMemory(t *testing.T) {
	r, _ := Parse("meminfo=MemTotal:       16771836 kB\nmeminfo=MemFree:        14157204 kB\n")
	if r.MemTotalMB != 16378 || r.MemAvailMB != 13825 {
		t.Errorf("read %d MB available of %d MB", r.MemAvailMB, r.MemTotalMB)
	}
	r, _ = Parse("meminfo=MemTotal: 4000000 kB\nmeminfo=MemFree: 100000 kB\nmeminfo=MemAvailable: 2000000 kB\n")
	if r.MemAvailMB != 1953 {
		t.Errorf("Linux's own available memory wins: %d", r.MemAvailMB)
	}
}
