package health

import (
	"reflect"
	"strings"
	"testing"
)

// piAnswer is the host part of the vitals as rp-orange answered it (Ubuntu 25.10 on a Raspberry Pi 4, vcgencmd there but
// /dev/vcio root's, so the firmware word unreadable and the under-voltage alarm the one signal)
const piAnswer = `meminfo=MemTotal:        7986112 kB
meminfo=MemAvailable:    6817052 kB
meminfo=SwapTotal:             0 kB
meminfo=SwapFree:              0 kB
thermal=cpu-thermal 62809 110000
cpufreq=1300000 1800000
model=Raspberry Pi 4 Model B Rev 1.5
undervolt=0
`

// macAnswer is the same part as an Apple-silicon Mac answered it: hw.memsize, vm_stat, vm.swapusage, no thermal zone
const macAnswer = `memsize=34359738368
vmstat=Mach Virtual Memory Statistics: (page size of 16384 bytes)
vmstat=Pages free:                                   101596.
vmstat=Pages active:                                 551838.
vmstat=Pages inactive:                               550170.
vmstat=Pages speculative:                               928.
vmstat=Pages throttled:                                   0.
vmstat=Pages wired down:                             224132.
vmstat=Pages purgeable:                                4026.
vmstat="Translation faults":                     7018406537.
vmstat=Pages stored in compressor:                  1441766.
swapusage=total = 1024.00M  used = 282.62M  free = 741.38M  (encrypted)
model=Mac16,1
`

func TestAPiReadsItsMemoryTemperatureAndUnderVoltageAlarm(t *testing.T) {
	r, _ := Parse(piAnswer)
	if r.MemTotalMB != 7798 || r.MemAvailMB != 6657 || r.SwapTotalMB != 0 || r.SwapUsedMB != 0 {
		t.Errorf("memory %d / %d MB, swap %d / %d", r.MemAvailMB, r.MemTotalMB, r.SwapUsedMB, r.SwapTotalMB)
	}
	if r.TempC != 62.8 || r.TempCritC != 110 {
		t.Errorf("temperature %v °C, critical %v", r.TempC, r.TempCritC)
	}
	if r.FreqMHz != 1300 || r.FreqMaxMHz != 1800 || r.Model != "Raspberry Pi 4 Model B Rev 1.5" {
		t.Errorf("clock %d of %d MHz, model %q", r.FreqMHz, r.FreqMaxMHz, r.Model)
	}
	if r.Throttle == nil || len(r.Throttle.Now) != 0 || r.Throttle.Word != "" {
		t.Errorf("throttle %+v: the alarm alone, nothing now", r.Throttle)
	}
	low, _ := Parse(strings.Replace(piAnswer, "undervolt=0", "undervolt=1", 1))
	if low.Throttle == nil || !reflect.DeepEqual(low.Throttle.Now, []string{"under-voltage"}) {
		t.Errorf("the alarm up reads %+v", low.Throttle)
	}
}

func TestAMacReadsItsMemoryFromVMStat(t *testing.T) {
	r, _ := Parse(macAnswer)
	if r.MemTotalMB != 32768 {
		t.Errorf("total %d MB", r.MemTotalMB)
	}
	// free, inactive, speculative and purgeable pages of 16 KB: 656720 pages
	if r.MemAvailMB != 10261 {
		t.Errorf("available %d MB", r.MemAvailMB)
	}
	if r.SwapTotalMB != 1024 || r.SwapUsedMB != 282 {
		t.Errorf("swap %d / %d MB", r.SwapUsedMB, r.SwapTotalMB)
	}
	if r.TempC != 0 || r.Throttle != nil || r.Model != "Mac16,1" {
		t.Errorf("a Mac has no zone or firmware word here: %v °C, %+v, %q", r.TempC, r.Throttle, r.Model)
	}
}

func TestTheFirmwareWordSaysWhatHoldsNowAndSinceBoot(t *testing.T) {
	for word, want := range map[string]Throttle{
		"0x0":     {Now: []string{}, Word: "0x0"},
		"0x50000": {Now: []string{}, Since: []string{"under-voltage", "throttled"}, Word: "0x50000"},
		"0x50005": {Now: []string{"under-voltage", "throttled"}, Since: []string{"under-voltage", "throttled"}, Word: "0x50005"},
		"0xe000e": {Now: []string{"frequency capped", "throttled", "soft temperature limit"}, Since: []string{"frequency capped", "throttled", "soft temperature limit"}, Word: "0xe000e"},
	} {
		got, ok := ParseThrottle(word)
		if !ok || !reflect.DeepEqual(*got, want) {
			t.Errorf("ParseThrottle(%q) = %+v, want %+v", word, got, want)
		}
	}
	if _, ok := ParseThrottle("throttled=?"); ok {
		t.Error("a word that is no number read as one")
	}
	r, _ := Parse("throttled=0x50005\nundervolt=0\n")
	if r.Throttle == nil || r.Throttle.Word != "0x50005" {
		t.Errorf("the alarm overrode the firmware word: %+v", r.Throttle)
	}
}

func TestTheCPUZoneIsPickedOverTheOthers(t *testing.T) {
	r, _ := Parse("thermal=acpitz 71000 98000\nthermal=x86_pkg_temp 55000 100000\nthermal=iwlwifi_1 80000\n")
	if r.TempC != 55 || r.TempCritC != 100 {
		t.Errorf("picked %v °C (critical %v), want the package's 55", r.TempC, r.TempCritC)
	}
	r, _ = Parse("thermal=acpitz 41000\nthermal=acpitz 47500\n")
	if r.TempC != 47.5 {
		t.Errorf("among zones of no kind the hottest: %v", r.TempC)
	}
}

func TestASampleKeepsTheMemoryAndTheTemperature(t *testing.T) {
	r, _ := Parse(piAnswer + "uptime= 10:00  up 28 days,  1:02,  0 users,  load average: 0.42, 0.27, 0.26\ndf=/dev/root 122344908 4557588 112763960 4% /\n")
	s, ok := sample(r)
	if !ok || s.MemUsed != 14.6 || s.Temp != 62.8 {
		t.Errorf("sample %+v", s)
	}
}
