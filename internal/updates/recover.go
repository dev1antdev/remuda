package updates

import (
	"fmt"
	"strings"

	"github.com/dev1antdev/remuda/internal/jobs"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/restart"
)

// RecoverChats starts a job resuming the chats a reboot stopped on the machine in the panes picked, every one for
// none; nothing stopped there, or none of the panes picked, is refused before any job.
func (s *Service) RecoverChats(mc *machines.Machine, panes []string) (jobs.Job, error) {
	return s.recoverChats(mc, panes, false)
}

// AutoRecoverChats is RecoverChats of every stopped chat, started by itself: the person switched on "Resume
// automatically next time".
func (s *Service) AutoRecoverChats(mc *machines.Machine) (jobs.Job, error) {
	return s.recoverChats(mc, nil, true)
}

func (s *Service) recoverChats(mc *machines.Machine, panes []string, auto bool) (jobs.Job, error) {
	stopped := s.Restart.Picked(mc.ID(), panes)
	if len(stopped) == 0 {
		return jobs.Job{}, fmt.Errorf("no chat stopped by a reboot on %s", mc.Label())
	}
	if s.Restart.Recovering(mc.ID()) {
		return jobs.Job{}, fmt.Errorf("the chats stopped on %s are being resumed already", mc.Label())
	}
	title := fmt.Sprintf("Resume %d chat(s) stopped with the reboot", len(stopped))
	if len(stopped) == 1 {
		title = "Resume the chat stopped with the reboot"
	}
	if auto {
		title += ", by itself"
	}
	var lines []string
	for _, c := range stopped {
		lines = append(lines, fmt.Sprintf("%s in %s   (%s)", restart.Title(c.Agent), c.Pane, c.Title))
	}
	command := "# on " + mc.Label() + ", through herdr, each in its own pane with its session resumed:\n" + strings.Join(lines, "\n")
	picked := make([]string, 0, len(stopped))
	for _, c := range stopped {
		picked = append(picked, c.Pane)
	}
	return s.Jobs.Run(mc, title, command, func(say func(string)) error {
		if auto {
			return s.Restart.AutoRecover(mc, say)
		}
		return s.Restart.Recover(mc, picked, say)
	}), nil
}
