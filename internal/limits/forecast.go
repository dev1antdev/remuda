package limits

// The forecast: when each window runs out at the pace of its recent readings. The pace is the slope of a line through
// the readings of the last fifth of the window (an hour of the 5 hours, a day and a third of the week); there is none
// from fewer than MinSamples readings, from readings covering less than a twentieth of the window, from a line that rose
// less than a point (nobody spends it), or from readings gone quiet for a tenth of the window (nobody spends it now).

import (
	"log"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/state"
)

// Sample is a window's use as one reading said it, at the moment the agent reported it.
type Sample struct {
	At   time.Time `json:"at"`
	Used float64   `json:"used"`
}

// Outlook is when a window runs out at the pace of its recent readings, or that the pace leaves some of it at its reset.
type Outlook struct {
	RunsOutAt *time.Time `json:"runs_out_at,omitempty"` // when it reaches 100 %, before its reset
	Lasts     bool       `json:"lasts,omitempty"`       // at this pace it lasts to its reset
	Pace      float64    `json:"pace"`                  // percent of the window an hour
}

// MinSamples is the fewest readings a pace is drawn from.
const MinSamples = 4

// Length is a window's span: 5 hours, a week, or the minutes its name says ("90m"); 0 for a name it cannot read.
func Length(name string) time.Duration {
	switch name {
	case "5h":
		return 5 * time.Hour
	case "week":
		return 7 * 24 * time.Hour
	}
	if minutes, err := strconv.Atoi(strings.TrimSuffix(name, "m")); err == nil && minutes > 0 && strings.HasSuffix(name, "m") {
		return time.Duration(minutes) * time.Minute
	}
	return 0
}

// lookback is how far back the readings that make the pace reach.
func lookback(length time.Duration) time.Duration { return length / 5 }

// Forecast is when the window runs out at the pace of its recent readings; false when they say no pace (see above), or
// when the window is spent already or has reset.
func Forecast(samples []Sample, w Window, now time.Time) (Outlook, bool) {
	length := Length(w.Name)
	if length <= 0 || w.Used >= 100 || !w.ResetsAt.After(now) {
		return Outlook{}, false
	}
	recent := sinceReset(within(samples, now.Add(-lookback(length)), now))
	if len(recent) < MinSamples {
		return Outlook{}, false
	}
	first, last := recent[0], recent[len(recent)-1]
	if last.At.Sub(first.At) < length/20 || now.Sub(last.At) > length/10 || last.Used-first.Used < 1 || last.Used >= 100 {
		return Outlook{}, false
	}
	pace := slope(recent)
	if pace <= 0 {
		return Outlook{}, false
	}
	out := Outlook{Pace: math.Round(pace*10) / 10}
	runsOut := last.At.Add(time.Duration((100 - last.Used) / pace * float64(time.Hour)))
	if !runsOut.Before(w.ResetsAt) {
		out.Lasts = true
		return out, true
	}
	if runsOut.Before(now) {
		runsOut = now
	}
	at := runsOut.Round(time.Minute).UTC()
	out.RunsOutAt = &at
	return out, true
}

// within is the samples from since to now, oldest first.
func within(samples []Sample, since, now time.Time) []Sample {
	var out []Sample
	for _, s := range samples {
		if !s.At.Before(since) && !s.At.After(now) {
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out
}

// sinceReset is the samples after the last drop: a window that started afresh says nothing of the one before.
func sinceReset(samples []Sample) []Sample {
	start := 0
	for i := 1; i < len(samples); i++ {
		if samples[i].Used < samples[i-1].Used-0.5 {
			start = i
		}
	}
	return samples[start:]
}

// slope is the least-squares line's rise through the samples, in percent an hour.
func slope(samples []Sample) float64 {
	origin := samples[0].At
	var sumX, sumY, sumXY, sumXX float64
	for _, s := range samples {
		x := s.At.Sub(origin).Hours()
		sumX += x
		sumY += s.Used
		sumXY += x * s.Used
		sumXX += x * x
	}
	n := float64(len(samples))
	den := n*sumXX - sumX*sumX
	if den == 0 {
		return 0
	}
	return (n*sumXY - sumX*sumY) / den
}

// addSample puts a reading into a window's samples: once by the moment it was reported, a reading closer than a
// six-hundredth of the window to the one before taking its place, the ones past the lookback dropped, at most maxSamples.
func addSample(samples []Sample, s Sample, length time.Duration, now time.Time) []Sample {
	for _, have := range samples {
		if have.At.Equal(s.At) {
			return samples
		}
	}
	samples = append(samples, s)
	sort.SliceStable(samples, func(i, j int) bool { return samples[i].At.Before(samples[j].At) })
	kept := samples[:0]
	for i, have := range samples {
		if have.At.Before(now.Add(-lookback(length))) {
			continue
		}
		if i+1 < len(samples) && samples[i+1].At.Sub(have.At) < length/600 {
			continue
		}
		kept = append(kept, have)
	}
	if len(kept) > maxSamples {
		kept = kept[len(kept)-maxSamples:]
	}
	return kept
}

const maxSamples = 200

// notePaceLocked keeps a reading's windows among their recent readings and writes them at most every paceSaveEvery;
// the caller holds mu.
func (s *Service) notePaceLocked(r Reading, now time.Time) {
	for key := range s.paces {
		if resets, ok := keyResets(key); ok && !resets.After(now) {
			delete(s.paces, key)
			s.paceMoved = true
		}
	}
	for _, w := range r.Windows {
		length := Length(w.Name)
		if length <= 0 {
			continue
		}
		at := w.At
		if at.IsZero() {
			at = now
		}
		key := alertKey(w)
		was := s.paces[key]
		s.paces[key] = addSample(append([]Sample(nil), was...), Sample{At: at.UTC(), Used: w.Used}, length, now)
		s.paceMoved = s.paceMoved || !slices.Equal(was, s.paces[key])
	}
	if s.dir != "" && s.paceMoved && now.Sub(s.paceAt) >= paceSaveEvery {
		s.paceMoved = false
		s.paceAt = now
		if err := state.SaveJSON(s.dir, paceFile, s.paces); err != nil {
			log.Printf("%s: %v", paceFile, err)
		}
	}
}

// paceFile keeps the windows' recent readings, the forecast's, across a restart: the weekly window's pace takes hours to read.
const paceFile = "limits-pace.json"

// paceSaveEvery is how often the readings are written at most.
const paceSaveEvery = 10 * time.Minute

// ForecastAhead is how much earlier than its reset a window must be forecast to run out for the push that says so.
const ForecastAhead = 30 * time.Minute
