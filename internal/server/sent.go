package server

import (
	"context"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/sent"
)

// What a caller typed into a pane through remuda — a text, keys, a chosen answer — is a quiet row of the activity,
// naming the device it came from.

// noteSent keeps the row for one sending; keys says the text names keys rather than quoting what was typed.
func (s *Server) noteSent(ctx context.Context, machineID, paneID, text string, keys bool) {
	if s.Feed == nil {
		return
	}
	s.Feed.Add(sent.Entry(s.senderName(ctx), text, keys, machineID, paneID, s.workspaceOf(machineID, paneID)))
}

// senderName is the caller as a person would name it: the device's name, an agent key's name, else an API client.
func (s *Server) senderName(ctx context.Context) string {
	if id := auth.DeviceID(ctx); id != "" && s.Guard != nil && s.Guard.Devices != nil {
		if name, ok := s.Guard.Devices.Name(id); ok && name != "" {
			return name
		}
	}
	if name := auth.CallerName(ctx); name != "" {
		return name
	}
	return sent.APIClient
}

// workspaceOf is the pane's workspace as the sidebar names it, from the watcher's snapshot; "" when unknown.
func (s *Server) workspaceOf(machineID, paneID string) string {
	if s.Watch == nil {
		return ""
	}
	snap, err := s.Watch.Snapshot(machineID)
	if err != nil || snap == nil {
		return ""
	}
	pane := snap.Pane(paneID)
	if pane == nil {
		return ""
	}
	return snap.WorkspaceLabel(pane.WorkspaceID)
}
