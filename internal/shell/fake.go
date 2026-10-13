package shell

import (
	"sync"
	"time"
)

// Fake is a Host for tests: every script goes to Answer, whose text is the output.
type Fake struct {
	Name   string
	Dir    string // what Home answers
	Remote bool
	Answer func(script string) (string, error)

	mu      sync.Mutex
	scripts []string
}

func (f *Fake) ID() string { return f.Name }

func (f *Fake) Label() string { return f.Name }

func (f *Fake) Home() string { return f.Dir }

func (f *Fake) Local() bool { return !f.Remote }

// Scripts is every script run so far, in order.
func (f *Fake) Scripts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.scripts...)
}

func (f *Fake) Run(script string, timeout time.Duration) ([]byte, error) {
	out, err := f.answer(script)
	return []byte(out), err
}

func (f *Fake) Stream(script string, timeout time.Duration, sink func([]byte)) error {
	out, err := f.answer(script)
	if out != "" {
		sink([]byte(out))
	}
	return err
}

func (f *Fake) answer(script string) (string, error) {
	f.mu.Lock()
	f.scripts = append(f.scripts, script)
	f.mu.Unlock()
	if f.Answer == nil {
		return "", nil
	}
	return f.Answer(script)
}
