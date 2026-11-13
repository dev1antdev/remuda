package ghissues

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
)

// The writes, each made only when the person presses it on the page: a comment, a close for a reason, a reopen, the
// signed-in account assigned or unassigned. A comment's text travels as a file put on the machine, never on a command
// line. What gh says when it refuses is the error, whole.

// WriteTimeout is how long one write may take.
var WriteTimeout = 45 * time.Second

// ErrEmpty is a comment with nothing in it.
var ErrEmpty = errors.New("the comment is empty")

// Refused is gh's own word when a write did not go through.
type Refused struct {
	Machine string
	Output  string
}

func (r *Refused) Error() string {
	text := strings.TrimSpace(r.Output)
	if text == "" {
		text = "it answered nothing"
	}
	return "gh on " + r.Machine + " did not do it: " + text
}

// Comment posts the text as a comment on the issue.
func (s *Service) Comment(machineID, repo string, number int, body string) error {
	if strings.TrimSpace(body) == "" {
		return ErrEmpty
	}
	return s.write(machineID, repo, number, body, func(file string) string { return commentScript(repo, number, file) })
}

// Close closes the issue for the reason ("completed" or "not planned"), the comment posted first when there is one.
func (s *Service) Close(machineID, repo string, number int, reason, comment string) error {
	why, ok := Reason(reason)
	if !ok {
		return ErrInvalid
	}
	return s.write(machineID, repo, number, comment, func(file string) string { return closeScript(repo, number, why, file) })
}

// Reopen reopens the issue, the comment posted first when there is one.
func (s *Service) Reopen(machineID, repo string, number int, comment string) error {
	return s.write(machineID, repo, number, comment, func(file string) string { return reopenScript(repo, number, file) })
}

// Assign adds the signed-in account to the issue's assignees, or takes it off.
func (s *Service) Assign(machineID, repo string, number int, me bool) error {
	return s.write(machineID, repo, number, "", func(string) string { return assignScript(repo, number, me) })
}

// write runs one write's script, the text put on the machine first when there is one, and marks what it changed.
func (s *Service) write(machineID, repo string, number int, text string, build func(file string) string) error {
	if !RepoOK(repo) || number <= 0 {
		return ErrInvalid
	}
	if len(text) > MaxBody {
		return fmt.Errorf("%w: a comment holds at most %d characters", ErrInvalid, MaxBody)
	}
	host, err := s.Hosts(machineID)
	if err != nil {
		return err
	}
	file := ""
	if strings.TrimSpace(text) != "" {
		tmp, err := host.Run(`mktemp "${TMPDIR:-/tmp}/remuda-issue.XXXXXX"`, 30*time.Second)
		if err != nil {
			return fmt.Errorf("no room on %s for the comment: %w", host.Label(), err)
		}
		file = strings.TrimSpace(string(tmp))
		if err := host.PutFile(file, []byte(text), WriteTimeout); err != nil {
			_, _ = host.Run("rm -f "+script.Quote(file), 30*time.Second)
			return fmt.Errorf("the comment could not be copied to %s: %w", host.Label(), err)
		}
	}
	out, err := host.Run(build(file), WriteTimeout)
	defer s.moved(machineID, repo, number)
	if err != nil {
		if trouble := troubleOf(host.Label(), repo, firstText(string(out), err)); trouble.Kind != "failed" {
			return trouble
		}
		return &Refused{Machine: host.Label(), Output: firstText(string(out), err)}
	}
	return nil
}
