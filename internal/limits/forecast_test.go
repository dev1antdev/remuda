package limits

import (
	"testing"
	"time"
)

var forecastNow = time.Date(2026, 11, 12, 14, 0, 0, 0, time.UTC)

// rising is n readings a few minutes apart up to now, from start by step percent each
func rising(n int, every time.Duration, start, step float64) []Sample {
	var out []Sample
	for i := 0; i < n; i++ {
		out = append(out, Sample{At: forecastNow.Add(-time.Duration(n-1-i) * every), Used: start + float64(i)*step})
	}
	return out
}

func fiveHours(used float64, resetsIn time.Duration) Window {
	return Window{Agent: "claude", Name: "5h", Used: used, ResetsAt: forecastNow.Add(resetsIn), At: forecastNow}
}

func TestAWindowRisingFastRunsOutBeforeItsReset(t *testing.T) {
	samples := rising(6, 5*time.Minute, 40, 2) // 40 → 50 % in 25 minutes: 24 % an hour
	out, ok := Forecast(samples, fiveHours(50, 3*time.Hour), forecastNow)
	if !ok || out.RunsOutAt == nil || out.Lasts {
		t.Fatalf("no run-out: %+v %v", out, ok)
	}
	if want := forecastNow.Add(125 * time.Minute); !out.RunsOutAt.Equal(want) {
		t.Fatalf("runs out at %v, want %v", out.RunsOutAt, want)
	}
	if out.Pace != 24 {
		t.Fatalf("pace %v", out.Pace)
	}
}

func TestAWindowRisingSlowlyLastsToItsReset(t *testing.T) {
	samples := rising(6, 5*time.Minute, 40, 0.5) // 6 % an hour: 50 points take over eight hours
	out, ok := Forecast(samples, fiveHours(42.5, 2*time.Hour), forecastNow)
	if !ok || !out.Lasts || out.RunsOutAt != nil {
		t.Fatalf("%+v %v", out, ok)
	}
}

func TestNoForecastFromTooLittle(t *testing.T) {
	w := fiveHours(50, 3*time.Hour)
	cases := map[string][]Sample{
		"three readings":            rising(3, 5*time.Minute, 44, 3),
		"readings over ten minutes": rising(6, 2*time.Minute, 40, 2),
		"a flat line":               rising(8, 5*time.Minute, 50, 0.1),
		"a falling line":            rising(6, 5*time.Minute, 60, -2),
		"none":                      nil,
	}
	for name, samples := range cases {
		if out, ok := Forecast(samples, w, forecastNow); ok {
			t.Errorf("%s: a forecast %+v", name, out)
		}
	}
}

func TestNoForecastOnceTheReadingsWentQuiet(t *testing.T) {
	samples := rising(6, 5*time.Minute, 40, 2)
	later := forecastNow.Add(40 * time.Minute) // the 5 hours' tenth is half an hour
	if out, ok := Forecast(samples, fiveHours(50, 3*time.Hour), later); ok {
		t.Fatalf("a forecast from readings 40 minutes old: %+v", out)
	}
}

func TestReadingsBeforeAResetSayNothing(t *testing.T) {
	old := rising(6, 5*time.Minute, 80, 3)
	fresh := []Sample{{At: forecastNow.Add(-4 * time.Minute), Used: 1}, {At: forecastNow.Add(-2 * time.Minute), Used: 2}, {At: forecastNow, Used: 3}}
	samples := append(old[:3], fresh...)
	if out, ok := Forecast(samples, fiveHours(3, 4*time.Hour), forecastNow); ok {
		t.Fatalf("the window before the reset made a pace: %+v", out)
	}
}

func TestASpentWindowHasNoForecast(t *testing.T) {
	if _, ok := Forecast(rising(6, 5*time.Minute, 90, 2), fiveHours(100, time.Hour), forecastNow); ok {
		t.Fatal("a spent window was forecast")
	}
	if _, ok := Forecast(rising(6, 5*time.Minute, 40, 2), Window{Agent: "claude", Name: "odd", Used: 50, ResetsAt: forecastNow.Add(time.Hour)}, forecastNow); ok {
		t.Fatal("a window of no known length was forecast")
	}
}

func TestTheWeekNeedsHoursOfReadings(t *testing.T) {
	week := Window{Agent: "codex", Name: "week", Used: 40, ResetsAt: forecastNow.Add(4 * 24 * time.Hour), At: forecastNow}
	if _, ok := Forecast(rising(8, 20*time.Minute, 33, 1), week, forecastNow); ok {
		t.Fatal("two hours of readings made a weekly pace")
	}
	out, ok := Forecast(rising(10, time.Hour, 31, 1), week, forecastNow) // a point an hour, 60 % left: 2.5 days, before the reset in 4
	if !ok || out.RunsOutAt == nil {
		t.Fatalf("%+v %v", out, ok)
	}
}

func TestSamplesAreKeptOnceAndBounded(t *testing.T) {
	var samples []Sample
	length := Length("5h")
	at := forecastNow.Add(-2 * time.Hour)
	for i := 0; i < 400; i++ {
		samples = addSample(samples, Sample{At: at, Used: float64(i) / 10}, length, at)
		samples = addSample(samples, Sample{At: at, Used: float64(i) / 10}, length, at) // the same reading twice
		at = at.Add(20 * time.Second)
	}
	if len(samples) > maxSamples {
		t.Fatalf("%d samples kept", len(samples))
	}
	for i := 1; i < len(samples); i++ {
		if !samples[i].At.After(samples[i-1].At) {
			t.Fatalf("samples out of order or twice at %d", i)
		}
		if gap := samples[i].At.Sub(samples[i-1].At); gap < length/600 {
			t.Fatalf("two readings %v apart kept", gap)
		}
	}
	if first := samples[0].At; first.Before(at.Add(-lookback(length) - time.Minute)) {
		t.Fatalf("a reading past the look back kept: %v", first)
	}
}

func TestLengthReadsTheWindowsNames(t *testing.T) {
	for name, want := range map[string]time.Duration{"5h": 5 * time.Hour, "week": 168 * time.Hour, "90m": 90 * time.Minute, "x": 0, "m": 0} {
		if got := Length(name); got != want {
			t.Errorf("%s: %v", name, got)
		}
	}
}
