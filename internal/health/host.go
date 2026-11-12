package health

// A machine's body as the vitals read it without sudo: its memory, its CPU's temperature and frequency, its model, and
// on a Raspberry Pi what the firmware throttled. The script prints raw lines (meminfo=, vmstat=, thermal=, …) and the
// parsing is Go's, so every system's format is tested on its own recorded answer.

import (
	"strconv"
	"strings"
)

// HostScript prints the body's lines; every probe is silent where its file or tool is missing.
const HostScript = `grep -E '^(MemTotal|MemAvailable|SwapTotal|SwapFree):' /proc/meminfo 2>/dev/null | sed 's/^/meminfo=/'
if command -v vm_stat >/dev/null 2>&1; then echo "memsize=$(sysctl -n hw.memsize 2>/dev/null)"; vm_stat 2>/dev/null | sed 's/^/vmstat=/'; echo "swapusage=$(sysctl -n vm.swapusage 2>/dev/null)"; fi
for z in /sys/class/thermal/thermal_zone*; do [ -r "$z/temp" ] && echo "thermal=$(cat "$z/type" 2>/dev/null) $(cat "$z/temp" 2>/dev/null) $(cat "$z"/trip_point_*_temp 2>/dev/null | sort -n | tail -1)"; done
f=/sys/devices/system/cpu/cpu0/cpufreq; [ -r "$f/scaling_cur_freq" ] && echo "cpufreq=$(cat "$f/scaling_cur_freq") $(cat "$f/cpuinfo_max_freq" 2>/dev/null)"
m=$(tr -d '\000' 2>/dev/null < /proc/device-tree/model); [ -n "$m" ] || m=$(sysctl -n hw.model 2>/dev/null); [ -n "$m" ] && echo "model=$m"
t=$(vcgencmd get_throttled 2>/dev/null); case "$t" in throttled=*) echo "throttled=${t#throttled=}";; *) [ -r /sys/devices/platform/soc/soc:firmware/get_throttled ] && echo "throttled=0x$(cat /sys/devices/platform/soc/soc:firmware/get_throttled)";; esac
for h in /sys/class/hwmon/hwmon*; do [ "$(cat "$h/name" 2>/dev/null)" = rpi_volt ] && echo "undervolt=$(cat "$h/in0_lcrit_alarm" 2>/dev/null)"; done
` + BatteryScript

// Throttle is what a Raspberry Pi's firmware held the board back for: now, and since it booted when the firmware's
// word was readable (vcgencmd); else the under-voltage alarm alone, now.
type Throttle struct {
	Now   []string `json:"now"`
	Since []string `json:"since,omitempty"`
	Word  string   `json:"word,omitempty"` // the firmware's own, "0x50000", when it was read
}

// throttleBits are the firmware word's bits, the low four now and the same four from bit 16 since boot.
var throttleBits = []string{"under-voltage", "frequency capped", "throttled", "soft temperature limit"}

// ParseThrottle reads the firmware word ("0x50005", vcgencmd's or sysfs's) into what holds now and what held since boot.
func ParseThrottle(word string) (*Throttle, bool) {
	value, err := strconv.ParseUint(strings.TrimPrefix(strings.TrimSpace(word), "0x"), 16, 32)
	if err != nil {
		return nil, false
	}
	t := &Throttle{Now: []string{}, Word: "0x" + strconv.FormatUint(value, 16)}
	for bit, what := range throttleBits {
		if value&(1<<bit) != 0 {
			t.Now = append(t.Now, what)
		}
		if value&(1<<(bit+16)) != 0 {
			t.Since = append(t.Since, what)
		}
	}
	return t, true
}

// body gathers the lines a report is read from in more than one: the memory of a Mac, the thermal zones
type body struct {
	pageSize, pagesFree, memsize float64
	sawVM                        bool
	zone                         thermalZone
	battery                      *Battery
	onBattery                    bool // a Mac's pmset says it draws from the battery
}

type thermalZone struct {
	kind       string
	temp, crit float64
	cpu        bool
}

// cpuZone says whether a thermal zone is the CPU's rather than a battery's, a disk's or the wifi's
func cpuZone(kind string) bool {
	kind = strings.ToLower(kind)
	for _, word := range []string{"cpu", "x86_pkg", "soc", "package", "core"} {
		if strings.Contains(kind, word) {
			return true
		}
	}
	return false
}

// parseHost reads one of the body's lines into the report, false when the key is none of them.
func (b *body) parseHost(r *Report, k, v string) bool {
	switch k {
	case "meminfo":
		name, rest, _ := strings.Cut(v, ":")
		kb, _ := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(rest), "kB")))
		switch name {
		case "MemTotal":
			r.MemTotalMB = kb / 1024
		case "MemAvailable":
			r.MemAvailMB = kb / 1024
		case "SwapTotal":
			r.SwapTotalMB = kb / 1024
		case "SwapFree":
			r.SwapUsedMB = r.SwapTotalMB - kb/1024
		}
	case "memsize":
		b.memsize, _ = strconv.ParseFloat(v, 64)
	case "vmstat":
		b.sawVM = true
		if i := strings.Index(v, "page size of "); i >= 0 {
			b.pageSize, _ = strconv.ParseFloat(strings.Fields(v[i+len("page size of "):])[0], 64)
			return true
		}
		name, count, ok := strings.Cut(v, ":")
		if !ok {
			return true
		}
		switch strings.TrimSpace(name) {
		case "Pages free", "Pages inactive", "Pages speculative", "Pages purgeable":
			pages, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(count), "."), 64)
			b.pagesFree += pages
		}
	case "swapusage":
		total, used := swapField(v, "total"), swapField(v, "used")
		if total > 0 {
			r.SwapTotalMB, r.SwapUsedMB = int(total), int(used)
		}
	case "thermal":
		f := strings.Fields(v)
		if len(f) < 2 {
			return true
		}
		temp, err := strconv.ParseFloat(f[1], 64)
		if err != nil || temp <= 0 {
			return true
		}
		zone := thermalZone{kind: f[0], temp: temp / 1000, cpu: cpuZone(f[0])}
		if len(f) >= 3 {
			crit, _ := strconv.ParseFloat(f[2], 64)
			zone.crit = crit / 1000
		}
		// the CPU's zone over any other, the hottest among equals
		if b.zone.kind == "" || zone.cpu && !b.zone.cpu || zone.cpu == b.zone.cpu && zone.temp > b.zone.temp {
			b.zone = zone
		}
	case "cpufreq":
		f := strings.Fields(v)
		if len(f) >= 1 {
			khz, _ := strconv.Atoi(f[0])
			r.FreqMHz = khz / 1000
		}
		if len(f) >= 2 {
			khz, _ := strconv.Atoi(f[1])
			r.FreqMaxMHz = khz / 1000
		}
	case "model":
		r.Model = strings.TrimSpace(v)
	case "throttled":
		if t, ok := ParseThrottle(v); ok {
			r.Throttle = t
		}
	case "batt":
		b.parseBatt(v)
	case "psu":
		b.parsePSU(v)
	case "undervolt":
		if r.Throttle == nil && strings.TrimSpace(v) != "" {
			r.Throttle = &Throttle{Now: []string{}}
			if strings.TrimSpace(v) == "1" {
				r.Throttle.Now = append(r.Throttle.Now, throttleBits[0])
			}
		}
	default:
		return false
	}
	return true
}

// finish puts what took several lines into the report: a Mac's memory, the zone chosen
func (b *body) finish(r *Report) {
	if b.memsize > 0 {
		r.MemTotalMB = int(b.memsize / (1024 * 1024))
		if b.sawVM && b.pageSize > 0 {
			r.MemAvailMB = int(b.pagesFree * b.pageSize / (1024 * 1024))
		}
	}
	if b.zone.kind != "" {
		r.TempC = float64(int(b.zone.temp*10)) / 10
		r.TempCritC = b.zone.crit
	}
	b.finishBattery(r)
}

// swapField reads "used = 1024.00M" out of macOS's vm.swapusage, in MB
func swapField(text, name string) float64 {
	i := strings.Index(text, name+" = ")
	if i < 0 {
		return 0
	}
	value := strings.Fields(text[i+len(name)+3:])
	if len(value) == 0 {
		return 0
	}
	number := strings.TrimRight(value[0], "MGK")
	n, err := strconv.ParseFloat(number, 64)
	if err != nil {
		return 0
	}
	switch {
	case strings.HasSuffix(value[0], "G"):
		n *= 1024
	case strings.HasSuffix(value[0], "K"):
		n /= 1024
	}
	return n
}
