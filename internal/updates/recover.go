package updates

import (
	"fmt"
	"strings"

	"github.com/dev1antdev/remuda/internal/jobs"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/restart"
)

// RecoverChats starts a job resuming the chats a reboot stopped on the machine in the panes picked, every one for
// none; nothing stopped there, none of the panes picked, or herdr's agents while its server still answers there
// (*restart.HerdrRunning), is refused before any job.
func (s *Service) RecoverChats(mc *machines.Machine, panes []string) (jobs.Job, error) {
	return s.recoverChats(mc, panes, false)
}

// AutoRecoverChats is RecoverChats of every chat a reboot stopped, started by itself: the person switched on "Resume
// automatically next time". herdr's agents are never among them.
func (s *Service) AutoRecoverChats(mc *machines.Machine) (jobs.Job, error) {
	return s.recoverChats(mc, nil, true)
}

func (s *Service) recoverChats(mc *machines.Machine, panes []string, auto bool) (jobs.Job, error) {
	stopped := s.Restart.Picked(mc.ID(), panes)
	if auto {
		stopped = restart.Rebooted(stopped)
	}
	if len(stopped) == 0 {
		return jobs.Job{}, fmt.Errorf("no chat stopped by a reboot on %s", mc.Label())
	}
	if s.Restart.Recovering(mc.ID()) {
		return jobs.Job{}, fmt.Errorf("the chats stopped on %s are being resumed already", mc.Label())
	}
	if err := s.Restart.MayCarryOn(mc, stopped); err != nil {
		return jobs.Job{}, err
	}
	fromHerdr := len(stopped) - len(restart.Rebooted(stopped))
	title := fmt.Sprintf("Resume %d chat(s) stopped with the reboot", len(stopped))
	switch {
	case fromHerdr == len(stopped):
		title = fmt.Sprintf("Carry on %d agent(s) from herdr", fromHerdr)
	case fromHerdr > 0:
		title = fmt.Sprintf("Resume %d chat(s): stopped with the reboot, and from herdr", len(stopped))
	case len(stopped) == 1:
		title = "Resume the chat stopped with the reboot"
	}
	if auto {
		title += ", by itself"
	}
	via, how := "herdr", "each in its own pane with its session resumed"
	if mc.OnCore() {
		via = "remuda-core"
	}
	if fromHerdr > 0 {
		how = "each with its session resumed, in its own pane, or from herdr in a tab of a workspace on its folder"
	}
	var lines []string
	for _, c := range stopped {
		line := fmt.Sprintf("%s in %s   (%s)", restart.Title(c.Agent), c.Pane, c.Title)
		if c.From == restart.FromHerdr {
			line += ", from herdr, on " + c.Dir
		}
		lines = append(lines, line)
	}
	command := "# on " + mc.Label() + ", through " + via + ", " + how + ":\n" + strings.Join(lines, "\n")
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
