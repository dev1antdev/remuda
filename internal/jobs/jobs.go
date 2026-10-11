// Package jobs runs the long commands a person watches from the UI — an
// update of herdr or an agent on a machine — and keeps their output for
// the page to poll. A job is one shell script on one machine, streamed.
package jobs

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/machines"
)

const (
	maxOutput = 256 * 1024 // what a job keeps of its output: the tail
	keepJobs  = 50
	timeout   = 20 * time.Minute
)

// Job is one command on one machine, as the page sees it.
type Job struct {
	ID      string    `json:"id"`
	Machine string    `json:"machine"`
	Title   string    `json:"title"`
	Command string    `json:"command"`
	Started time.Time `json:"started"`
	Ended   time.Time `json:"ended,omitzero"`
	Done    bool      `json:"done"`
	OK      bool      `json:"ok"`
	Error   string    `json:"error,omitempty"`
	Output  string    `json:"output"`
}

type job struct {
	mu   sync.Mutex
	view Job
	out  strings.Builder
}

// Runner keeps the jobs.
type Runner struct {
	mu    sync.Mutex
	jobs  map[string]*job
	order []string
	// OnDone, when set, is told of every finished job (the vitals re-read)
	OnDone func(j Job)
}

func New() *Runner { return &Runner{jobs: map[string]*job{}} }

// Start runs the script on the machine in the background.
func (r *Runner) Start(mc *machines.Machine, title, command string) Job {
	return r.StartThen(mc, title, command, nil, nil)
}

// StartThen is Start with Go work around the script, writing to the same
// log: first before it (a look at the versions), then after it whatever
// its exit status — an update of every agent that half failed still has
// the other half to bring the chats onto. The script's failure is the
// job's verdict.
func (r *Runner) StartThen(mc *machines.Machine, title, command string, first func(say func(string)), then func(say func(string)) error) Job {
	return r.launch(mc, title, command, func(j *job) error {
		say := func(line string) { j.write([]byte(line + "\n")) }
		if first != nil {
			first(say)
		}
		err := mc.Stream(command, timeout, j.write)
		if then != nil {
			if terr := then(say); terr != nil && err == nil {
				err = terr
			}
		}
		return err
	})
}

// List is the machine's jobs, newest first, without their output.
func (r *Runner) List(machineID string, n int) []Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Job
	for i := len(r.order) - 1; i >= 0 && len(out) < n; i-- {
		j := r.jobs[r.order[i]]
		if j == nil {
			continue
		}
		v := j.snapshot()
		if v.Machine != machineID {
			continue
		}
		v.Output = ""
		out = append(out, v)
	}
	return out
}

// Run is Start for a job written in Go rather than a script: fn streams
// its own lines and returns the verdict.
func (r *Runner) Run(mc *machines.Machine, title, command string, fn func(say func(string)) error) Job {
	return r.launch(mc, title, command, func(j *job) error {
		return fn(func(line string) { j.write([]byte(line + "\n")) })
	})
}

func (r *Runner) launch(mc *machines.Machine, title, command string, run func(j *job) error) Job {
	j := &job{view: Job{ID: newID(), Machine: mc.Cfg.ID, Title: title, Command: command, Started: time.Now()}}
	r.mu.Lock()
	r.jobs[j.view.ID] = j
	r.order = append(r.order, j.view.ID)
	for i := 0; len(r.order) > keepJobs && i < len(r.order); {
		if old := r.jobs[r.order[i]]; old != nil && !old.done() { // a running one stays
			i++
			continue
		}
		delete(r.jobs, r.order[i])
		r.order = append(r.order[:i], r.order[i+1:]...)
	}
	r.mu.Unlock()
	go func() {
		err := run(j)
		j.mu.Lock()
		j.view.Done, j.view.Ended = true, time.Now()
		if err != nil {
			j.view.Error = err.Error()
		} else {
			j.view.OK = true
		}
		j.mu.Unlock()
		if r.OnDone != nil {
			r.OnDone(j.snapshot())
		}
	}()
	return j.snapshot()
}

// Get is the job as it stands.
func (r *Runner) Get(id string) (Job, bool) {
	r.mu.Lock()
	j := r.jobs[id]
	r.mu.Unlock()
	if j == nil {
		return Job{}, false
	}
	return j.snapshot(), true
}

func (j *job) write(b []byte) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.out.Write(b)
	if j.out.Len() > maxOutput {
		s := j.out.String()
		j.out.Reset()
		j.out.WriteString("…\n")
		j.out.WriteString(s[len(s)-maxOutput/2:])
	}
}

func (j *job) done() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.view.Done
}

func (j *job) snapshot() Job {
	j.mu.Lock()
	defer j.mu.Unlock()
	v := j.view
	v.Output = j.out.String()
	return v
}

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
