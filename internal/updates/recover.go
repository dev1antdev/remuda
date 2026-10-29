package updates

import (
	"fmt"
	"strings"

	"github.com/dev1antdev/remuda/internal/jobs"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/restart"
)

// RecoverChats starts a job resuming every chat a reboot stopped on the machine; none stopped is refused before any job.
func (s *Service) RecoverChats(mc *machines.Machine) (jobs.Job, error) {
	stopped := s.Restart.Stopped(mc.ID())
	if len(stopped) == 0 {
		return jobs.Job{}, fmt.Errorf("no chat stopped by a reboot on %s", mc.Label())
	}
	title := fmt.Sprintf("Resume %d chat(s) stopped with the reboot", len(stopped))
	if len(stopped) == 1 {
		title = "Resume the chat stopped with the reboot"
	}
	var lines []string
	for _, c := range stopped {
		lines = append(lines, fmt.Sprintf("%s in %s   (%s)", restart.Title(c.Agent), c.Pane, c.Title))
	}
	command := "# on " + mc.Label() + ", through herdr, each in its own pane with its session resumed:\n" + strings.Join(lines, "\n")
	return s.Jobs.Run(mc, title, command, func(say func(string)) error { return s.Restart.Recover(mc, say) }), nil
}
