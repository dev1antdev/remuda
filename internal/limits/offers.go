package limits

// The offers: when a project's queue or an agent schedule holds a start on a plan limit, the person is offered to run
// that very card or schedule now on the other vendor's agent — once per card or schedule and window episode, through a
// push whose button does it and the page's toast. Nothing runs by itself and nothing saved changes: a taken offer starts
// the agent once (Run, wired by the app), a declined one is let be until the window resets.

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/state"
)

// The kinds of thing an offer runs.
const (
	OfferTask     = "task"
	OfferSchedule = "schedule"
)

// Offer is the person's chance to run a held card or schedule on the other vendor's agent now.
type Offer struct {
	ID       string     `json:"id"`
	Kind     string     `json:"kind"` // task | schedule
	Project  int64      `json:"project,omitempty"`
	Task     int64      `json:"task,omitempty"`
	Ref      string     `json:"ref,omitempty"` // the card as the board names it, "#12"
	Schedule string     `json:"schedule,omitempty"`
	Title    string     `json:"title"`             // the card's title or the schedule's name
	Machine  string     `json:"machine,omitempty"` // where it would run; "" for a project whose queue picks the machine
	From     string     `json:"from"`              // the agent kind the limit holds
	To       string     `json:"to"`                // the one offered
	Model    string     `json:"model,omitempty"`   // a recipe's model for it, "" for its default
	Why      string     `json:"why"`               // the hold, in words
	Episode  string     `json:"-"`
	Window   string     `json:"window"`                // the held window: 5h | week
	Used     float64    `json:"used"`                  // its use when the offer was made
	RunsOut  *time.Time `json:"runs_out_at,omitempty"` // when it runs out at this pace, for a window held on its forecast
	ResetsAt time.Time  `json:"resets_at"`             // the held window's reset, when the offer goes
	At       time.Time  `json:"at"`
	Closed   string     `json:"closed,omitempty"` // taken | declined; an open offer has none
}

// Errors an offer's run answers.
var (
	ErrNoOffer      = errors.New("no such offer: it was taken, declined, or its window reset")
	ErrOfferRunning = errors.New("this offer is being run already")
)

// offerKey is the offer's place: one per card or schedule and episode.
func offerKey(o Offer) string {
	return fmt.Sprintf("%s|%d|%d|%s|%s", o.Kind, o.Project, o.Task, o.Schedule, o.Episode)
}

// Offer keeps an offer and pushes it, unless one stands already for the same card or schedule and episode; true when
// it was made now.
func (s *Service) Offer(o Offer) bool {
	now := s.now()
	key := offerKey(o)
	s.mu.Lock()
	if _, ok := s.kept.Offers[key]; ok || o.To == "" || !o.ResetsAt.After(now) {
		s.mu.Unlock()
		return false
	}
	o.ID, o.At, o.Closed = state.NewID(), now.UTC(), ""
	s.kept.Offers[key] = o
	s.saveLocked()
	s.mu.Unlock()
	s.notify(OfferPush(o))
	s.changed()
	return true
}

// OfferPush is the push of an offer: what waits and why, and a button that runs it on the other vendor.
func OfferPush(o Offer) push.Payload {
	what := "Schedule “" + o.Title + "”"
	if o.Kind == OfferTask {
		what = "Task " + o.Ref
	}
	return push.Payload{
		Machine: o.Machine, Kind: "limits", Panel: "usage",
		Title:   what + " waits for " + AgentName(o.From) + "'s plan limit",
		Body:    o.Why + " · run it on " + AgentName(o.To) + " now?",
		Tag:     "limits-offer|" + o.ID,
		Actions: []push.Action{{Action: push.ActionOffer, Title: "Run on " + AgentName(o.To)}},
		Offer:   o.ID,
	}
}

// Offers are the open ones, oldest first; the ones whose window reset are gone.
func (s *Service) Offers() []Offer {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Offer{}
	for _, o := range s.kept.Offers {
		if o.Closed == "" && o.ResetsAt.After(now) && !s.offering[o.ID] {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].At.Before(out[j].At) || out[i].At.Equal(out[j].At) && out[i].ID < out[j].ID
	})
	return out
}

// OfferIDs are the open offers' ids, what the roster carries so a page knows when to read them.
func (s *Service) OfferIDs() []string {
	var ids []string
	for _, o := range s.Offers() {
		ids = append(ids, o.ID)
	}
	return ids
}

// RunOffer runs an open offer through Run, one run of it at a time; what Run says is the answer. A run that fails
// leaves the offer open, for the person to try again.
func (s *Service) RunOffer(id string) (string, error) {
	key, o, err := s.claim(id)
	if err != nil {
		return "", err
	}
	if s.Run == nil {
		s.release(id)
		return "", errors.New("no way to run an offer")
	}
	words, err := s.Run(o)
	s.mu.Lock()
	delete(s.offering, id)
	if err == nil {
		if cur, ok := s.kept.Offers[key]; ok {
			cur.Closed = "taken"
			s.kept.Offers[key] = cur
			s.saveLocked()
		}
	}
	s.mu.Unlock()
	s.changed()
	return words, err
}

// DeclineOffer lets an offer be until its window resets: no device shows it again.
func (s *Service) DeclineOffer(id string) error {
	s.mu.Lock()
	err := ErrNoOffer
	for key, o := range s.kept.Offers {
		if o.ID == id && o.Closed == "" {
			o.Closed = "declined"
			s.kept.Offers[key] = o
			err = s.saveLocked()
			break
		}
	}
	s.mu.Unlock()
	if err != ErrNoOffer {
		s.changed()
	}
	return err
}

func (s *Service) claim(id string) (string, Offer, error) {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, o := range s.kept.Offers {
		if o.ID != id {
			continue
		}
		switch {
		case o.Closed != "" || !o.ResetsAt.After(now):
			return "", Offer{}, ErrNoOffer
		case s.offering[id]:
			return "", Offer{}, ErrOfferRunning
		}
		s.offering[id] = true
		return key, o, nil
	}
	return "", Offer{}, ErrNoOffer
}

func (s *Service) release(id string) {
	s.mu.Lock()
	delete(s.offering, id)
	s.mu.Unlock()
}

// expireOffersLocked drops the offers whose window reset; true when one went. The caller holds mu.
func (s *Service) expireOffersLocked(now time.Time) bool {
	gone := false
	for key, o := range s.kept.Offers {
		if !o.ResetsAt.After(now) {
			delete(s.kept.Offers, key)
			gone = true
		}
	}
	return gone
}
