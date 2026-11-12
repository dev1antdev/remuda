package schedule

import (
	"errors"
	"log"
	"strings"

	"github.com/dev1antdev/remuda/internal/push"
)

// KindScene runs one of the person's scenes on the timetable: a start scene's steps as they saved them, a tidy's whole
// plan only while nobody's agent works or asks.
const KindScene = "scene"

// Scenes is the app's way to a scene schedule's scene: its name while it exists, and a run of it that answers what it
// came to, or that it waits for another run (skip: SkipAtWork or SkipRunning, the result its words), or why it failed.
type Scenes struct {
	Name func(id string) (string, bool)
	Run  func(id, by string) (result, skip string, err error)
}

// checkScene keeps a scene schedule to its scene: no machine, folder, prompt or agent of its own; the label defaults
// to the scene's name.
func (st *Store) checkScene(s *Schedule) error {
	s.Scene = strings.TrimSpace(s.Scene)
	if s.Machine != "" || s.Dir != "" || s.Prompt != "" || s.Agent != "" {
		return errors.New("a scene schedule runs its scene: no machine, folder, prompt or agent of its own")
	}
	if s.Scene == "" {
		return errors.New("a scene is required")
	}
	name, ok := "", false
	if st.Scenes.Name != nil {
		name, ok = st.Scenes.Name(s.Scene)
	}
	if !ok {
		return errors.New("that scene is gone")
	}
	s.Since, s.Model, s.Args, s.Close = "", "", "", false
	if s.Label == "" {
		s.Label = name
	}
	return nil
}

// fireScene runs the scene and notes what it came to; a run that waits (a tidy while an agent works) is skipped
// quietly, a failed one is pushed.
func (st *Store) fireScene(s *Schedule) {
	result, skip, err := "", "", errors.New("scenes cannot run here")
	if st.Scenes.Run != nil {
		result, skip, err = st.Scenes.Run(s.Scene, "the schedule “"+s.Label+"”")
	}
	if skip != "" {
		st.noteSkip(s.ID, skip, result)
		return
	}
	st.mu.Lock()
	cur, ok := st.items[s.ID]
	if !ok {
		st.mu.Unlock()
		return
	}
	cur.LastRun, cur.Skipped, cur.LastOK, cur.LastResult = st.now(), "", err == nil, result
	if err != nil {
		cur.LastResult = err.Error()
	}
	if serr := st.saveLocked(); serr != nil {
		log.Printf("schedules: %v", serr)
	}
	label := cur.Label
	st.mu.Unlock()
	if err != nil && st.notify != nil {
		st.notify(push.Payload{Kind: "schedule", Title: "Schedule “" + label + "” could not run its scene", Body: err.Error(), Tag: "schedule|" + s.ID})
	}
}
