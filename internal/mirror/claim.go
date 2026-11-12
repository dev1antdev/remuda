package mirror

import (
	"maps"
	"strings"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// One herdr, one writer: a machine that runs its own remuda beside a cockpit reaching it over ssh would get two
// mirrors writing under the same source — the tasks, the reviews and the watchdog's notes each instance's own, the
// values flipping between them, every toast popped twice. So every remuda that mirrors anything claims the machine's
// herdr with the token remuda_by on each workspace, "remote <host:port>" from a cockpit reaching it, "local <host:port>"
// from the machine's own, and reads the claims back before every sync: a remote claim outranks a local one (the cockpit
// knows the person's tasks), two of one kind go by name. The one outranked yields there — no write, no toast — until
// the other's claim lapses with its TTL.

// TokenBy is the claim: which remuda writes into this herdr. No snippet row names it, so herdr never draws it.
const TokenBy = "remuda_by"

// Yield is a machine where another remuda writes into herdr, so this one stays out.
type Yield struct {
	Machine string `json:"machine"`
	Label   string `json:"label"`
	Writer  string `json:"writer"` // the other remuda's host and port
}

// claimOf is this remuda's claim on a machine, remote when it reaches the machine over ssh, as herdr stores it: read
// back the same, it is told from another remuda's.
func claimOf(self string, local bool) string {
	role := "remote"
	if local {
		role = "local"
	}
	return herdr.DisplayText(role+" "+self, herdr.MaxValueRunes)
}

// outranks says whether another remuda's claim takes the machine from ours.
func outranks(other, ours string) bool {
	otherRole, otherName, _ := strings.Cut(other, " ")
	ourRole, ourName, _ := strings.Cut(ours, " ")
	if otherRole != ourRole {
		return otherRole == "remote"
	}
	return otherName < ourName
}

// claimant is the claim on the snapshot's workspaces that outranks ours, "" when none does.
func claimant(snap *herdr.Snapshot, ours string) string {
	for i := range snap.Workspaces {
		if by := snap.Workspaces[i].Tokens[TokenBy]; by != "" && by != ours && outranks(by, ours) {
			return by
		}
	}
	return ""
}

// writerName is a claim's remuda in words: its host and port.
func writerName(claim string) string {
	_, name, _ := strings.Cut(claim, " ")
	return name
}

// with is the figures and the claim beside them.
func (f Figures) with(name, value string) Figures {
	tokens := maps.Clone(f.Tokens)
	if tokens == nil {
		tokens = map[string]string{}
	}
	tokens[name] = value
	return Figures{Tokens: tokens, Labels: f.Labels}
}

// yieldTo notes the remuda this writer stays out for, "" for none; the toasts and the Settings section read it.
func (w *writer) yieldTo(claim string) {
	w.mu.Lock()
	w.yielded = claim
	w.mu.Unlock()
	if claim != "" {
		w.panes, w.workspaces = map[string]written{}, map[string]written{} // what it wrote is the other's to overwrite
	}
}

func (w *writer) yielding() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.yielded
}

// outranked says whether another remuda's claim stands in a machine's herdr as its latest snapshot shows it: that
// one pops the toasts there, from the moment its claim is read, before this one's writer has synced at all.
func (s *Service) outranked(machineID string) bool {
	if s.in.Snapshot == nil {
		return false
	}
	snap := s.in.Snapshot(machineID)
	return snap != nil && claimant(snap, claimOf(s.in.Self, s.local(machineID))) != ""
}
