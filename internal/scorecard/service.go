package scorecard

import (
	"time"

	"github.com/dev1antdev/remuda/internal/arena"
	"github.com/dev1antdev/remuda/internal/review"
	"github.com/dev1antdev/remuda/internal/tasks"
)

// Service reads the scorecard off the cockpit's own stores; a store left nil counts as empty.
type Service struct {
	Tasks  *tasks.Store
	Review *review.Store
	Arenas *arena.Store
	Now    func() time.Time
}

// Since is a period's first moment: local midnight days-1 days before now, as the usage report cuts its periods.
func Since(days int, now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day()-(days-1), 0, 0, 0, 0, now.Location())
}

// Card is the scorecard of the last days, the days cut in loc.
func (s *Service) Card(days int, loc *time.Location) (Card, error) {
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	since := Since(days, now.In(loc))
	var rec Records
	if s.Tasks != nil {
		runs, err := s.Tasks.RunsSince(since)
		if err != nil {
			return Card{}, err
		}
		rec.Runs = runs
	}
	if s.Review != nil {
		rec.Verdicts = s.Review.Verdicts(since)
	}
	if s.Arenas != nil {
		rec.Lanes = s.Arenas.Outcomes(since)
	}
	card := Build(rec, since)
	card.Days = days
	return card, nil
}

// WithoutTasks is the card with no task named in it: what a viewer link reads, since the tasks are not a viewer's.
func (c Card) WithoutTasks() Card {
	rows := make([]Row, len(c.Rows))
	for i, row := range c.Rows {
		row.Try = nil
		rows[i] = row
	}
	c.Rows = rows
	return c
}
