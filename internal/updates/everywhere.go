package updates

// Update everywhere: every update a connected machine has pending — herdr, its hooks, the agents, code-server, a newer
// release of remuda — listed as steps whose commands are shown before Run, then run one after another here, each
// waiting for the job before it, the page's own host last, since its remuda restarts once its job ends. A step is run
// only as it was shown: the plan is read again at Run and a step that moved is a StaleError, nothing started.

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/health"
	"github.com/dev1antdev/remuda/internal/machines"
)

// Step is one update of the run: the machine, what, its title and the command shown.
type Step struct {
	Machine string `json:"machine"`
	Label   string `json:"label"` // the machine's name, as the page shows it
	What    string `json:"what"`
	Title   string `json:"title"`
	Command string `json:"command"`
}

// StepRun is a step as the run stands: waiting, running, ok, failed, or skipped when its machine went away.
type StepRun struct {
	Step
	State string `json:"state"`
	Job   string `json:"job,omitempty"`
	Error string `json:"error,omitempty"`
}

// Batch is one run of Update everywhere.
type Batch struct {
	ID      string    `json:"id"`
	Started time.Time `json:"started"`
	Ended   time.Time `json:"ended,omitzero"`
	Done    bool      `json:"done"`
	Steps   []StepRun `json:"steps"`
}

// everywhereOrder is the order of a machine's steps: herdr first, then the hooks it writes, the agents, code-server,
// and remuda's own release last, since installing it restarts that machine's remuda
var everywhereOrder = []string{"herdr", "hooks", "claude", "codex", "gemini", "kimi", "code", "remuda-release"}

// StepTimeout is how long a step's job may run before the next one starts anyway.
var StepTimeout = 30 * time.Minute

// stepPoll is how often a running step's job is looked at.
var stepPoll = time.Second

// pendingUpdate says whether an offer installs something newer than the machine has: a tool's newer version known,
// hooks behind the installed herdr, a code-server older than the one remuda installs; a release is offered only when newer
func pendingUpdate(what string, r *health.Report, l health.Latest) bool {
	switch what {
	case "herdr":
		return r.Herdr != "" && l.Herdr != "" && health.Newer(r.Herdr, l.Herdr)
	case "claude", "codex", "gemini", "kimi":
		have, latest := agentVersion(r, what), agentVersion(&health.Report{Claude: l.Claude, Codex: l.Codex, Gemini: l.Gemini, Kimi: l.Kimi}, what)
		return have != "" && latest != "" && health.Newer(have, latest)
	case "hooks":
		return len(r.OutdatedHooks) > 0
	case "code":
		return r.CodeServer != "" && l.Code != "" && health.Newer(r.CodeServer, l.Code)
	case "remuda-release":
		return true
	}
	return false
}

// machineSteps are the machine's pending updates in everywhereOrder.
func (s *Service) machineSteps(mc *machines.Machine) []Step {
	report := s.Health.Report(mc.ID())
	if report == nil {
		return nil
	}
	latest := s.Health.Latest()
	offered := map[string]Update{}
	for _, u := range s.Available(mc) {
		offered[u.What] = u
	}
	var steps []Step
	for _, what := range everywhereOrder {
		if u, ok := offered[what]; ok && pendingUpdate(what, report, latest) {
			steps = append(steps, Step{Machine: mc.ID(), Label: mc.Label(), What: u.What, Title: u.Title, Command: u.Command})
		}
	}
	return steps
}

// EverywherePlan is every connected machine's pending updates, the page's own host last.
func (s *Service) EverywherePlan() []Step {
	var steps, host []Step
	for _, mc := range s.Machines.All() {
		if state, _ := mc.State(); state != machines.StateConnected {
			continue
		}
		if mc.Local() {
			host = s.machineSteps(mc)
			continue
		}
		steps = append(steps, s.machineSteps(mc)...)
	}
	return append(steps, host...)
}

// ErrBatchRunning refuses a second run while one goes.
var ErrBatchRunning = errors.New("an Update everywhere is running already")

// batches keeps the recent runs, the newest last.
type batches struct {
	mu   sync.Mutex
	list []*Batch
}

// StartEverywhere runs the steps the person was shown, each checked against the plan as it stands now.
func (s *Service) StartEverywhere(asked []Step) (Batch, error) {
	if len(asked) == 0 {
		return Batch{}, fmt.Errorf("no step to run")
	}
	if s.running() {
		return Batch{}, ErrBatchRunning
	}
	steps, err := inPlanOrder(s.EverywherePlan(), asked)
	if err != nil {
		return Batch{}, err
	}
	s.runs.mu.Lock()
	for _, b := range s.runs.list {
		if !b.Done {
			s.runs.mu.Unlock()
			return Batch{}, ErrBatchRunning
		}
	}
	batch := &Batch{ID: batchID(), Started: time.Now()}
	for _, step := range steps {
		batch.Steps = append(batch.Steps, StepRun{Step: step, State: "waiting"})
	}
	s.runs.list = append(s.runs.list, batch)
	if len(s.runs.list) > 5 {
		s.runs.list = s.runs.list[len(s.runs.list)-5:]
	}
	view := s.copyOf(batch)
	s.runs.mu.Unlock()
	go s.runBatch(batch)
	return view, nil
}

// inPlanOrder is the steps asked for, each as the plan holds it now and in the plan's order whatever the request's —
// the page's own host last, since its remuda restarts once its release is in; a step the plan no longer holds as it
// was shown is a StaleError
func inPlanOrder(plan, asked []Step) ([]Step, error) {
	now := map[string]Step{}
	for _, step := range plan {
		now[step.Machine+"|"+step.What] = step
	}
	wanted := map[string]bool{}
	for _, step := range asked {
		if current, ok := now[step.Machine+"|"+step.What]; !ok || current.Command != step.Command {
			return nil, &StaleError{fmt.Sprintf("%s on %s changed since it was shown — look again", step.Title, step.Label)}
		}
		wanted[step.Machine+"|"+step.What] = true
	}
	var steps []Step
	for _, step := range plan {
		if wanted[step.Machine+"|"+step.What] {
			steps = append(steps, step)
		}
	}
	return steps, nil
}

// running says whether a run goes now.
func (s *Service) running() bool {
	s.runs.mu.Lock()
	defer s.runs.mu.Unlock()
	for _, b := range s.runs.list {
		if !b.Done {
			return true
		}
	}
	return false
}

// Everywhere is the run as it stands.
func (s *Service) Everywhere(id string) (Batch, bool) {
	s.runs.mu.Lock()
	defer s.runs.mu.Unlock()
	for _, b := range s.runs.list {
		if b.ID == id {
			return s.copyOf(b), true
		}
	}
	return Batch{}, false
}

// copyOf is a batch the caller may keep; s.runs.mu is held.
func (s *Service) copyOf(b *Batch) Batch {
	view := *b
	view.Steps = append([]StepRun(nil), b.Steps...)
	return view
}

// runBatch runs the steps one after another, each waiting for its job; a step that fails does not stop the ones after it.
func (s *Service) runBatch(batch *Batch) {
	for i := range batch.Steps {
		s.setStep(batch, i, func(step *StepRun) { step.State = "running" })
		step := batch.Steps[i].Step
		mc, err := s.Machines.Get(step.Machine)
		if err == nil {
			if state, _ := mc.State(); state != machines.StateConnected {
				err = fmt.Errorf("%s is not connected now", step.Label)
			}
		}
		if err == nil {
			err = s.stillPending(mc, step)
		}
		if err != nil {
			s.setStep(batch, i, func(run *StepRun) { run.State, run.Error = "skipped", err.Error() })
			continue
		}
		job, err := s.Start(mc, step.What, step.Command)
		var stale *StaleError
		if errors.As(err, &stale) {
			s.setStep(batch, i, func(run *StepRun) { run.State, run.Error = "skipped", err.Error() })
			continue
		}
		if err != nil {
			s.setStep(batch, i, func(run *StepRun) { run.State, run.Error = "failed", err.Error() })
			continue
		}
		s.setStep(batch, i, func(run *StepRun) { run.Job = job.ID })
		done, ok, why := s.waitJob(job.ID)
		s.setStep(batch, i, func(run *StepRun) {
			switch {
			case !done:
				run.State, run.Error = "failed", "still running after "+StepTimeout.String()+"; the next step started"
			case ok:
				run.State = "ok"
			default:
				run.State, run.Error = "failed", why
			}
		})
	}
	s.runs.mu.Lock()
	batch.Done, batch.Ended = true, time.Now()
	s.runs.mu.Unlock()
}

// stillPending says why a step shown at Run is not to run now — an earlier step of the run already saw to it (the herdr
// update installs the hooks itself), or its command moved: a skip, not a failure; nil when it stands as it was shown
func (s *Service) stillPending(mc *machines.Machine, step Step) error {
	for _, now := range s.machineSteps(mc) {
		if now.What != step.What {
			continue
		}
		if now.Command != step.Command {
			return fmt.Errorf("its command changed since it was shown; look again")
		}
		return nil
	}
	return fmt.Errorf("nothing left to do: up to date now")
}

// waitJob waits for a job to end: whether it ended, went well, and why not.
func (s *Service) waitJob(id string) (done, ok bool, why string) {
	deadline := time.Now().Add(StepTimeout)
	for time.Now().Before(deadline) {
		if job, found := s.Jobs.Get(id); !found || job.Done {
			if !found {
				return true, false, "the job is gone"
			}
			return true, job.OK, job.Error
		}
		time.Sleep(stepPoll)
	}
	return false, false, ""
}

func (s *Service) setStep(batch *Batch, i int, change func(*StepRun)) {
	s.runs.mu.Lock()
	change(&batch.Steps[i])
	s.runs.mu.Unlock()
}

func batchID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
