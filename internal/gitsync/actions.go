package gitsync

import (
	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/shell"
)

// Fetch fetches the checkout's upstream's remote, else origin: the work and the branch stay as they are, so it runs
// while an agent works there.
func (s *Service) Fetch(machineID, dir string) Outcome {
	return s.act(ActFetch, machineID, dir, func(host shell.Host, style rpath.Style, p probe) Outcome {
		remote := p.fetchRemote()
		if remote == "" {
			return refused(ActFetch, ReasonNoRemote)
		}
		outcome := Outcome{Action: ActFetch, Remote: remote, Branch: p.branch, Upstream: p.upstream}
		return s.network(host, style, p, outcome, fetchScript(remote))
	})
}

// Pull fast-forwards the checkout's branch to its upstream after a fetch; refused while an agent works, asks or ends
// a turn there, since it changes the files under it, and a branch that diverged is said so and left alone.
func (s *Service) Pull(machineID, dir string) Outcome {
	return s.act(ActPull, machineID, dir, func(host shell.Host, style rpath.Style, p probe) Outcome {
		switch {
		case p.branch == "":
			return refused(ActPull, ReasonDetached)
		case !p.tracks():
			if len(p.remotes) == 0 {
				return refused(ActPull, ReasonNoRemote)
			}
			return Outcome{Action: ActPull, State: Refused, Reason: ReasonNoUpstream, Branch: p.branch}
		case p.remote == ".":
			return Outcome{Action: ActPull, State: Refused, Reason: ReasonLocal, Branch: p.branch}
		}
		if s.Busy != nil {
			if agent, doing := s.Busy(machineID, p.top); agent != "" {
				return Outcome{Action: ActPull, State: Refused, Reason: ReasonBusy, Agent: agent, Doing: doing, Branch: p.branch}
			}
		}
		outcome := Outcome{Action: ActPull, Remote: p.remote, Branch: p.branch, Upstream: p.upstream}
		return s.network(host, style, p, outcome, pullScript(p.remote))
	})
}

// Push sends the branch to its upstream, or to origin under its own name with the upstream set when it has none; never
// a force or a tag. The secret check over the commits it would carry (and the work beside them) stands first, its
// findings or its failure holding the push back until the person says anyway.
func (s *Service) Push(machineID, dir string, anyway bool) Outcome {
	return s.act(ActPush, machineID, dir, func(host shell.Host, style rpath.Style, p probe) Outcome {
		if p.branch == "" {
			return refused(ActPush, ReasonDetached)
		}
		remote, target, setUpstream := p.remote, p.merge, false
		switch {
		case p.remote == ".":
			return Outcome{Action: ActPush, State: Refused, Reason: ReasonLocal, Branch: p.branch}
		case !p.tracks() || !p.hasRemote(p.remote):
			remote, target, setUpstream = p.fetchRemote(), "refs/heads/"+p.branch, true
			if remote == "" && len(p.remotes) == 0 {
				return refused(ActPush, ReasonNoRemote)
			}
			if remote == "" {
				return Outcome{Action: ActPush, State: Refused, Reason: ReasonWhichRemote, Branch: p.branch}
			}
		}
		outcome := Outcome{Action: ActPush, Remote: remote, Branch: p.branch, Upstream: p.upstream, Ahead: p.ahead}
		if !anyway && s.Check != nil {
			scan, err := s.Check(host, p.top)
			if err != nil {
				outcome.State, outcome.Reason, outcome.Output = Held, ReasonCheckFailed, err.Error()
				return outcome
			}
			if len(scan.Findings) > 0 || scan.More > 0 {
				outcome.State, outcome.Reason, outcome.Scan = Held, ReasonSecretsFound, &scan
				return outcome
			}
		}
		result := s.network(host, style, p, outcome, pushScript(remote, p.branch, target, setUpstream))
		if result.State == Done {
			result.Count = p.ahead
			if setUpstream {
				result.Reason = ReasonNewBranch
			}
		}
		return result
	})
}

// act is an action's common way: the machine reached, the checkout probed, one action on it at a time.
func (s *Service) act(action, machineID, dir string, do func(shell.Host, rpath.Style, probe) Outcome) Outcome {
	host, err := s.Host(machineID)
	if err != nil {
		outcome := refused(action, ReasonAway)
		outcome.Output = err.Error()
		return outcome
	}
	style := rpath.For(host.Home())
	p, err := readProbe(host, style, dir)
	switch {
	case err != nil:
		return Outcome{Action: action, State: Failed, Output: err.Error()}
	case p.missing:
		return refused(action, ReasonMissing)
	case p.notRepo || p.top == "":
		return refused(action, ReasonNotRepo)
	}
	key := machineID + "|" + p.top
	s.lockOf(machineID)
	s.mu.Lock()
	if s.running[key] {
		s.mu.Unlock()
		return Outcome{Action: action, State: Refused, Reason: ReasonRunning, Branch: p.branch}
	}
	s.running[key] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.running, key)
		s.mu.Unlock()
		s.Forget(machineID, false)
	}()
	return do(host, style, p)
}

// network runs a fetch, pull or push, at most PerMachine at once on the machine, and reads what it came to.
func (s *Service) network(host shell.Host, style rpath.Style, p probe, outcome Outcome, action string) Outcome {
	slot := s.slotOf(host.ID())
	slot <- struct{}{}
	defer func() { <-slot }()
	output, after, err := run(host, style, p.top, action)
	outcome.Repo = after
	word, numbers := marker(output)
	switch {
	case err != nil:
		outcome.State, outcome.Output = Failed, gitWords(output)
		if outcome.Output == "" {
			outcome.Output = err.Error()
		}
	case word == "gone":
		outcome.State, outcome.Reason = Refused, ReasonGone
	case word == "diverged" && len(numbers) == 2:
		outcome.State, outcome.Reason, outcome.Ahead, outcome.Behind = Refused, ReasonDiverged, numbers[0], numbers[1]
	case word == "uptodate":
		outcome.State, outcome.Reason = Done, ReasonUpToDate
	case word == "pulled" && len(numbers) == 1:
		outcome.State, outcome.Count = Done, numbers[0]
	default:
		outcome.State = Done
	}
	if after != nil && outcome.Upstream == "" {
		outcome.Upstream = after.Upstream
	}
	return outcome
}

func (s *Service) slotOf(machineID string) chan struct{} {
	s.lockOf(machineID)
	s.mu.Lock()
	defer s.mu.Unlock()
	slot := s.slots[machineID]
	if slot == nil {
		slot = make(chan struct{}, PerMachine)
		s.slots[machineID] = slot
	}
	return slot
}
