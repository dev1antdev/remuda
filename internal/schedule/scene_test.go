package schedule

import (
	"errors"
	"testing"

	"github.com/dev1antdev/remuda/internal/push"
)

func TestASceneScheduleRunsItsSceneAndNothingOfItsOwn(t *testing.T) {
	var pushed []push.Payload
	st := New(t.TempDir(), nil, func(p push.Payload) { pushed = append(pushed, p) }, nil)
	var ran []string
	result, skip, fail := "3 started", "", error(nil)
	st.Scenes = Scenes{
		Name: func(id string) (string, bool) { return "Morning", id == "s1" },
		Run: func(id, by string) (string, string, error) {
			ran = append(ran, id+" by "+by)
			return result, skip, fail
		},
	}
	for name, s := range map[string]Schedule{
		"no scene":             {Kind: KindScene, Cron: "0 9 * * 1-5"},
		"a scene that is gone": {Kind: KindScene, Scene: "s2", Cron: "0 9 * * 1-5"},
		"a folder of its own":  {Kind: KindScene, Scene: "s1", Machine: "local", Dir: "/x", Cron: "0 9 * * 1-5"},
		"a prompt of its own":  {Kind: KindScene, Scene: "s1", Prompt: "hi", Cron: "0 9 * * 1-5"},
	} {
		if _, err := st.Put(s); err == nil {
			t.Errorf("%s: saved", name)
		}
	}
	saved, err := st.Put(Schedule{Kind: KindScene, Scene: " s1 ", Cron: "0 9 * * 1-5", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Label != "Morning" || saved.Scene != "s1" {
		t.Fatalf("named after its scene: %+v", saved)
	}
	got, _ := st.Run(saved.ID)
	if len(ran) != 1 || ran[0] != "s1 by the schedule “Morning”" || !got.LastOK || got.LastResult != "3 started" {
		t.Fatalf("ran its scene: %v %+v", ran, got)
	}

	result, skip = "an agent works or asks on GOG", SkipAtWork
	got, _ = st.Run(saved.ID)
	if got.Skipped != SkipAtWork || got.LastResult != "skipped: an agent works or asks on GOG" || len(pushed) != 0 {
		t.Fatalf("a tidy waits for a quiet evening, quietly: %+v", got)
	}

	result, skip, fail = "", "", errors.New("1 refused: the folder is gone")
	got, _ = st.Run(saved.ID)
	if got.LastOK || got.LastResult != "1 refused: the folder is gone" || len(pushed) != 1 || pushed[0].Kind != "schedule" {
		t.Fatalf("a failed run is pushed: %+v %v", got, pushed)
	}

	other, err := st.Put(Schedule{Kind: KindDigest, Scene: "s1", Cron: "0 9 * * *"})
	if err != nil || other.Scene != "" {
		t.Fatalf("another kind keeps no scene: %+v %v", other, err)
	}
}
