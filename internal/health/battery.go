package health

// The battery, read without sudo: `pmset -g batt` on a Mac, the power supply class under /sys on Linux. A machine with
// none — a desktop, a Raspberry Pi, a Mac mini — reports none; a device's own battery (a mouse's, a headset's) is not
// the machine's.

import (
	"strconv"
	"strings"
)

// BatteryScript prints a Mac's pmset lines as batt= and each Linux power supply as psu=<type> <scope> <online> <capacity> <status>.
const BatteryScript = `command -v pmset >/dev/null 2>&1 && pmset -g batt 2>/dev/null | sed 's/^/batt=/'
for p in /sys/class/power_supply/*; do [ -r "$p/type" ] && echo "psu=$(cat "$p/type" 2>/dev/null) $(cat "$p/scope" 2>/dev/null || echo System) $(cat "$p/online" 2>/dev/null || echo -) $(cat "$p/capacity" 2>/dev/null || echo -) $(cat "$p/status" 2>/dev/null | tr ' ' _)"; done
`

// Battery is the machine's own battery: how full, and whether the machine runs on it now.
type Battery struct {
	Percent     int  `json:"percent"`
	Discharging bool `json:"discharging,omitempty"` // on battery: the charger is away
	Charging    bool `json:"charging,omitempty"`
}

// parseBatt reads one of pmset's lines: "Now drawing from 'Battery Power'", then " -InternalBattery-0 (id=…)	85%; discharging; …".
func (b *body) parseBatt(v string) {
	if from, ok := strings.CutPrefix(v, "Now drawing from "); ok {
		b.onBattery = strings.Contains(from, "Battery Power")
		return
	}
	if !strings.Contains(v, "InternalBattery") {
		return
	}
	i := strings.Index(v, "%")
	if i < 0 {
		return
	}
	start := i
	for start > 0 && v[start-1] >= '0' && v[start-1] <= '9' {
		start--
	}
	percent, err := strconv.Atoi(v[start:i])
	if err != nil {
		return
	}
	status := ""
	if parts := strings.Split(v[i+1:], ";"); len(parts) > 1 {
		status = strings.ToLower(strings.TrimSpace(parts[1]))
	}
	b.battery = &Battery{Percent: percent, Discharging: status == "discharging", Charging: status == "charging" || status == "finishing charge"}
}

// parsePSU reads one of Linux's power supplies: its type, scope, online, capacity and status.
func (b *body) parsePSU(v string) {
	f := strings.Fields(v)
	if len(f) < 5 || f[0] != "Battery" || f[1] == "Device" {
		return
	}
	percent, err := strconv.Atoi(f[3])
	if err != nil {
		return
	}
	b.battery = &Battery{Percent: percent, Discharging: f[4] == "Discharging", Charging: f[4] == "Charging"}
}

// finishBattery puts the battery into the report: a Mac drawing from it is on battery whatever its status says.
func (b *body) finishBattery(r *Report) {
	if b.battery == nil {
		return
	}
	battery := *b.battery
	if b.onBattery {
		battery.Discharging, battery.Charging = true, false
	}
	r.Battery = &battery
}
