package usage

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

// MachineReport is one machine's report in the fleet's answer, or why it has none.
type MachineReport struct {
	ID     string  `json:"id"`
	Label  string  `json:"label"`
	Report *Report `json:"report,omitempty"`
	Error  string  `json:"error,omitempty"`
}

// Collect runs the report on every host at once: in process here, through the machine's own remuda on a remote.
func Collect(hosts []shell.Host, since time.Time, loc *time.Location, tz string) []MachineReport {
	out := []MachineReport{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, host := range hosts {
		wg.Add(1)
		go func(host shell.Host) {
			defer wg.Done()
			report := MachineReport{ID: host.ID(), Label: host.Label()}
			if rep, err := ReadHost(host, since, loc, tz, false, 90*time.Second); err != nil {
				report.Error = err.Error()
			} else {
				report.Report = &rep
			}
			mu.Lock()
			out = append(out, report)
			mu.Unlock()
		}(host)
	}
	wg.Wait()
	return out
}

// errNoReport is a remote remuda that answered something other than a report.
var errNoReport = errors.New("the machine's remuda gave no report")

// ReadHost is one machine's report since a moment, its days by folder when daily: in process here, through the
// machine's own remuda on a remote.
func ReadHost(host shell.Host, since time.Time, loc *time.Location, tz string, daily bool, timeout time.Duration) (Report, error) {
	if host.Local() {
		if daily {
			return ScanHomeDays(host.Home(), since, loc)
		}
		return ScanHome(host.Home(), since, loc)
	}
	run := Script
	if daily {
		run = DaysScript
	}
	raw, err := shell.Text(host, run(host.Home(), since, tz), timeout)
	if err != nil {
		return Report{}, errors.New(strings.TrimSpace(err.Error()))
	}
	var rep Report
	if json.Unmarshal([]byte(raw), &rep) != nil {
		return Report{}, errNoReport
	}
	return rep, nil
}
