package triggers

import (
	"time"
)

// offer keeps a dev server the row names as an offer to preview once the previews find its port listening; a few
// tries, since a server may say where it serves a moment before it does.
func (s *Service) offer(machine, paneID, row, by string) {
	port, url, ok := ServedAt(row)
	if !ok || s.Listening == nil {
		return
	}
	for _, wait := range listenTries {
		time.Sleep(wait)
		label, listening := s.Listening(machine, port)
		if !listening {
			continue
		}
		s.mu.Lock()
		if s.machines[machine][paneID] == nil {
			s.mu.Unlock()
			return // the pane went meanwhile
		}
		s.offers[key(machine, paneID)] = Offer{Port: port, URL: url, Label: label, By: by, At: time.Now()}
		s.mu.Unlock()
		if s.Changed != nil {
			s.Changed()
		}
		return
	}
}

// OfferOf is the preview a pane's dev server is offered as, while it stands; nil for none.
func (s *Service) OfferOf(machine, pane string) *Offer {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.offers[key(machine, pane)]
	if !ok || time.Since(o.At) > OfferKeep {
		return nil
	}
	return &o
}

// Refused hears herdr refusing the person's watches on a machine, or taking them again (nil).
func (s *Service) Refused(machine string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		delete(s.refused, machine)
		return
	}
	s.refused[machine] = err.Error()
}

// Refusals is why herdr refused the person's watches, by machine.
func (s *Service) Refusals() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]string, len(s.refused))
	for machine, why := range s.refused {
		out[machine] = why
	}
	return out
}
