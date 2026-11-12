package health

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAMacOnItsChargerChargesItsBattery(t *testing.T) {
	r, _ := Parse("batt=Now drawing from 'AC Power'\nbatt= -InternalBattery-0 (id=1234567)\t92%; charging; 0:28 remaining present: true\n")
	if r.Battery == nil || r.Battery.Percent != 92 || r.Battery.Discharging || !r.Battery.Charging {
		t.Fatalf("%+v", r.Battery)
	}
}

func TestAMacDrawingFromItsBatteryIsOnBattery(t *testing.T) {
	r, _ := Parse("batt=Now drawing from 'Battery Power'\nbatt= -InternalBattery-0 (id=1234567)\t15%; discharging; 0:41 remaining present: true\n")
	if r.Battery == nil || r.Battery.Percent != 15 || !r.Battery.Discharging || r.Battery.Charging {
		t.Fatalf("%+v", r.Battery)
	}
	r, _ = Parse("batt=Now drawing from 'Battery Power'\nbatt= -InternalBattery-0 (id=1234567)\t100%; charged; 0:00 remaining present: true\n")
	if r.Battery == nil || !r.Battery.Discharging {
		t.Fatalf("a full battery the Mac draws from: %+v", r.Battery)
	}
}

func TestAMacMiniHasNoBattery(t *testing.T) {
	if r, _ := Parse("batt=Now drawing from 'AC Power'\n"); r.Battery != nil {
		t.Fatalf("%+v", r.Battery)
	}
}

func TestALinuxLaptopReadsItsOwnBatteryNotItsMouses(t *testing.T) {
	r, _ := Parse("psu=Mains System 0 - -\npsu=Battery Device - 40 Discharging\npsu=Battery System - 63 Discharging\n")
	if r.Battery == nil || r.Battery.Percent != 63 || !r.Battery.Discharging {
		t.Fatalf("%+v", r.Battery)
	}
	if r, _ := Parse("psu=Mains System 1 - -\npsu=Battery System - 80 Not_charging\n"); r.Battery == nil || r.Battery.Discharging || r.Battery.Charging {
		t.Fatalf("a battery held on the charger: %+v", r.Battery)
	}
	if r, _ := Parse("psu=USB System 1 - -\n"); r.Battery != nil {
		t.Fatalf("a Pi's supply read as a battery: %+v", r.Battery)
	}
}

// TestTheBatteryScriptReadsAPowerSupplyFolder runs the Linux half on a stand-in of /sys/class/power_supply
func TestTheBatteryScriptReadsAPowerSupplyFolder(t *testing.T) {
	root := t.TempDir()
	for name, files := range map[string]map[string]string{
		"AC":     {"type": "Mains", "online": "0"},
		"BAT0":   {"type": "Battery", "capacity": "57", "status": "Discharging"},
		"hid-kb": {"type": "Battery", "scope": "Device", "capacity": "10", "status": "Not charging"},
	} {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for file, text := range files {
			if err := os.WriteFile(filepath.Join(dir, file), []byte(text+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	linux := BatteryScript[strings.Index(BatteryScript, "for p in"):]
	out, err := exec.Command("sh", "-c", strings.ReplaceAll(linux, "/sys/class/power_supply", root)).Output()
	if err != nil {
		t.Fatal(err)
	}
	r, _ := Parse(string(out))
	if r.Battery == nil || r.Battery.Percent != 57 || !r.Battery.Discharging {
		t.Fatalf("%q read as %+v", out, r.Battery)
	}
	if !strings.Contains(string(out), "psu=Battery Device - 10 Not_charging") {
		t.Fatalf("the device's battery line: %q", out)
	}
}
