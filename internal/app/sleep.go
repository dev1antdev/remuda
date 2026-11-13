package app

import (
	"log"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/feed"
	"github.com/dev1antdev/remuda/internal/jobs"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/restart"
	"github.com/dev1antdev/remuda/internal/sent"
	"github.com/dev1antdev/remuda/internal/sessions"
	"github.com/dev1antdev/remuda/internal/sleep"
	"github.com/dev1antdev/remuda/internal/updates"
	"github.com/dev1antdev/remuda/internal/watch"
)

// sleeper is the workspaces put to sleep and woken: its record is the restart memory's word that a sleeping pane is
// neither stopped by a reboot nor closed, the chats a reboot stopped are resumed by themselves when the person said
// so, and a closed workspace or a removed machine takes its record with it.
func sleeper(dir string, bus *events.Bus, fleet *machines.Manager, watcher *watch.Watcher, runner *jobs.Runner, input *sessions.Input, restarter *restart.Restarter, updater *updates.Service, activity *feed.Service, skip func(machineID, workspaceID string) bool, changed func()) *sleep.Service {
	store := sleep.Open(dir)
	service := &sleep.Service{Store: store, Watch: watcher, Jobs: runner, Lock: input.Lock, Skip: skip}
	service.Typed = func(machineID, paneID, text string, keys bool) {
		workspace := ""
		if snap, _ := watcher.Snapshot(machineID); snap != nil {
			if pane := snap.Pane(paneID); pane != nil {
				workspace = snap.WorkspaceLabel(pane.WorkspaceID)
			}
		}
		activity.Add(sent.Entry(sent.Remuda, text, keys, machineID, paneID, workspace))
	}
	restarter.Memory.Asleep = store.Holds
	restarter.Memory.OnStopped = func(machineID string) {
		if !store.Settings().ResumeAutomatically {
			return
		}
		mc, err := fleet.Get(machineID)
		if err != nil {
			return
		}
		if _, err := updater.AutoRecoverChats(mc); err != nil {
			log.Printf("resume the chats stopped on %s by themselves: %v", mc.Label(), err)
		}
	}
	events.Subscribe(bus, func(e events.Snapshot) {
		if store.Observe(e) {
			changed()
		}
	})
	events.Subscribe(bus, func(e events.MachineRemoved) { store.Forget(e.Machine) })
	return service
}
