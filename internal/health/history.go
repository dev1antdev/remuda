package health

import (
	"log"
	"time"

	"github.com/dev1antdev/remuda/internal/state"
)

// Sample is one probe's load, disk, memory and temperature reading, kept so the ⓘ and the Server place can show the day.
type Sample struct {
	At       time.Time `json:"at"`
	Load     float64   `json:"load"`
	DiskFree float64   `json:"disk_free"`      // percent of the disk free
	MemUsed  float64   `json:"mem,omitempty"`  // percent of the memory in use, 0 where it was not read
	Temp     float64   `json:"temp,omitempty"` // the CPU's °C, 0 where there is no reading
}

const (
	// HistoryKeep is how many samples a machine keeps: a day of probes 15 minutes apart.
	HistoryKeep = 96
	// HistoryFile holds every machine's samples in the state dir.
	HistoryFile = "vitals.json"
	// sampleApart is how close to the last sample a read replaces it instead of adding one (a read on demand).
	sampleApart = time.Minute
)

// history is every machine's samples, by machine id, as kept on disk.
type history map[string][]Sample

// loadHistory reads the samples kept from before; a damaged file is an empty history, said once.
func loadHistory(dir string) history {
	h := history{}
	if dir == "" {
		return h
	}
	if err := state.LoadJSON(dir, HistoryFile, &h); err != nil {
		log.Printf("health: %s is not readable (%v); the vitals start afresh", HistoryFile, err)
		return history{}
	}
	for id, samples := range h {
		h[id] = trim(samples)
	}
	return h
}

// note adds a sample to a machine's samples: one within sampleApart of the last replaces it, the oldest past HistoryKeep go.
func note(samples []Sample, s Sample) []Sample {
	if n := len(samples); n > 0 && s.At.Sub(samples[n-1].At) < sampleApart {
		samples = samples[:n-1]
	}
	return trim(append(samples, s))
}

func trim(samples []Sample) []Sample {
	if len(samples) > HistoryKeep {
		samples = samples[len(samples)-HistoryKeep:]
	}
	return samples
}

// sample is what a report adds to the history, and whether it read the vitals at all (a probe that reached nothing adds no zeros).
func sample(r Report) (Sample, bool) {
	if r.Load1 == 0 && r.DiskFreeGB == 0 {
		return Sample{}, false
	}
	s := Sample{At: r.At, Load: r.Load1, DiskFree: float64(100 - r.DiskUsedPct), Temp: r.TempC}
	if r.MemTotalMB > 0 && r.MemAvailMB > 0 {
		s.MemUsed = float64(int(1000*float64(r.MemTotalMB-r.MemAvailMB)/float64(r.MemTotalMB))) / 10
	}
	return s, true
}

// remember keeps the report's sample for the machine and writes the history out; the file is written outside the lock,
// from a copy, since the next probe may replace a sample in place meanwhile.
func (c *Collector) remember(id string, r Report) {
	s, ok := sample(r)
	if !ok {
		return
	}
	c.mu.Lock()
	c.history[id] = note(c.history[id], s)
	c.historySeq++
	seq := c.historySeq
	snapshot := make(history, len(c.history))
	for k, v := range c.history {
		snapshot[k] = append([]Sample(nil), v...)
	}
	c.mu.Unlock()
	if c.dir == "" {
		return
	}
	c.historyWrite.Lock()
	defer c.historyWrite.Unlock()
	if seq < c.historyWritten { // two machines' probes at once: a newer copy is on disk already
		return
	}
	if err := state.SaveJSON(c.dir, HistoryFile, snapshot); err != nil {
		log.Printf("health: %s not written: %v", HistoryFile, err)
		return
	}
	c.historyWritten = seq
}
