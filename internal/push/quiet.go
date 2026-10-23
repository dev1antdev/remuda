package push

import (
	"time"
	_ "time/tzdata" // a device's zone loads on any host, with or without a zoneinfo folder
)

// Quiet is a daily window, in the device's own zone, during which nothing is sent to it.
type Quiet struct {
	From string `json:"from"` // "HH:MM", 24 h
	To   string `json:"to"`
	TZ   string `json:"tz"` // an IANA zone name; empty reads as UTC
}

const clock = "15:04"

// Valid says whether both times parse and the zone loads.
func (q Quiet) Valid() bool {
	_, errFrom := time.Parse(clock, q.From)
	_, errTo := time.Parse(clock, q.To)
	_, errZone := q.location()
	return errFrom == nil && errTo == nil && errZone == nil
}

func (q Quiet) location() (*time.Location, error) {
	if q.TZ == "" {
		return time.UTC, nil
	}
	return time.LoadLocation(q.TZ)
}

// Covers says whether now falls inside the window: From == To is no window, a window past midnight
// (22:00 → 07:00) wraps, and an unknown zone reads as UTC rather than silencing nothing by surprise.
func (q Quiet) Covers(now time.Time) bool {
	from, errFrom := time.Parse(clock, q.From)
	to, errTo := time.Parse(clock, q.To)
	if errFrom != nil || errTo != nil || q.From == q.To {
		return false
	}
	loc, err := q.location()
	if err != nil {
		loc = time.UTC
	}
	local := now.In(loc)
	minute := local.Hour()*60 + local.Minute()
	start := from.Hour()*60 + from.Minute()
	end := to.Hour()*60 + to.Minute()
	if start < end {
		return minute >= start && minute < end
	}
	return minute >= start || minute < end
}

// muted says whether a device is inside its quiet hours at this moment.
func muted(sub *Subscription, now time.Time) bool {
	return sub.Prefs.Quiet != nil && sub.Prefs.Quiet.Covers(now)
}
