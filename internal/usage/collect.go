package usage

import (
	"encoding/json"
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
			if host.Local() {
				rep, err := ScanHome(host.Home(), since, loc)
				if err != nil {
					report.Error = err.Error()
				} else {
					report.Report = &rep
				}
			} else {
				raw, err := shell.Text(host, Script(host.Home(), since, tz), 90*time.Second)
				var rep Report
				switch {
				case err != nil:
					report.Error = strings.TrimSpace(err.Error())
				case json.Unmarshal([]byte(raw), &rep) != nil:
					report.Error = "the machine's remuda gave no report"
				default:
					report.Report = &rep
				}
			}
			mu.Lock()
			out = append(out, report)
			mu.Unlock()
		}(host)
	}
	wg.Wait()
	return out
}
