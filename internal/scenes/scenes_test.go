package scenes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func morning() Scene {
	return Scene{Name: " Morning ", Steps: []Step{
		{Machine: "local", Dir: "/home/me/api/", Agent: "claude", Model: "opus", Args: "--permission-mode plan", Prompt: " Read the overnight PRs ", Tab: " planner "},
		{Machine: "gog", Dir: "/home/me/web", Agent: "shell"},
	}}
}

func TestAStartSceneIsTrimmedAndChecked(t *testing.T) {
	scene, err := Check(morning(), nil)
	if err != nil {
		t.Fatal(err)
	}
	step := scene.Steps[0]
	if scene.Name != "Morning" || scene.Kind != KindStart || step.Dir != "/home/me/api" || step.Prompt != "Read the overnight PRs" || step.Tab != "planner" {
		t.Fatalf("trimmed: %+v", scene)
	}
}

func TestAStepIsRefusedWhereARecipeWouldBe(t *testing.T) {
	known := func(id string) bool { return id == "r1" }
	for name, step := range map[string]Step{
		"no machine":                {Dir: "/x", Agent: "claude"},
		"a relative folder":         {Machine: "local", Dir: "x", Agent: "claude"},
		"nothing to run":            {Machine: "local", Dir: "/x"},
		"an agent herdr lacks":      {Machine: "local", Dir: "/x", Agent: "nobody"},
		"a shell with a prompt":     {Machine: "local", Dir: "/x", Agent: "shell", Prompt: "hi"},
		"a model with no flag":      {Machine: "local", Dir: "/x", Agent: "pi", Model: "big"},
		"an unclosed quote":         {Machine: "local", Dir: "/x", Agent: "claude", Args: `--name "half`},
		"a recipe and an agent":     {Machine: "local", Dir: "/x", Agent: "claude", Recipe: "r1"},
		"a recipe with a prompt":    {Machine: "local", Dir: "/x", Recipe: "r1", Prompt: "hi"},
		"a recipe that is gone":     {Machine: "local", Dir: "/x", Recipe: "r2"},
		"a tab name too long":       {Machine: "local", Dir: "/x", Agent: "shell", Tab: strings.Repeat("a", MaxTab+1)},
		"a tab name with a newline": {Machine: "local", Dir: "/x", Agent: "shell", Tab: "a\nb"},
	} {
		if _, err := CheckStep(step, known); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := CheckStep(Step{Machine: "local", Dir: "/x", Recipe: "r1", Tab: "plan"}, known); err != nil {
		t.Fatalf("a recipe step: %v", err)
	}
}

func TestASceneIsRefusedForWhatCouldNotRun(t *testing.T) {
	many := Scene{Name: "Many"}
	for range MaxSteps + 1 {
		many.Steps = append(many.Steps, Step{Machine: "local", Dir: "/x", Agent: "shell"})
	}
	for name, scene := range map[string]Scene{
		"no name":                 {Steps: morning().Steps},
		"no steps":                {Name: "Empty"},
		"too many steps":          many,
		"an unknown kind":         {Name: "Odd", Kind: "dance", Steps: morning().Steps},
		"a start scene that tidy": {Name: "Both", Steps: morning().Steps, Tidy: &Tidy{Shells: true}},
		"a tidy with steps":       {Name: "Both", Kind: KindTidy, Steps: morning().Steps, Tidy: &Tidy{Shells: true}},
		"a tidy doing nothing":    {Name: "Evening", Kind: KindTidy, Tidy: &Tidy{}},
		"a tidy without choices":  {Name: "Evening", Kind: KindTidy},
		"idle past a week":        {Name: "Evening", Kind: KindTidy, Tidy: &Tidy{Idle: true, IdleHours: MaxIdleHours + 1}},
		"a share past 95 %":       {Name: "Evening", Kind: KindTidy, Tidy: &Tidy{Compact: true, CompactAt: 99}},
		"a share under 10 %":      {Name: "Evening", Kind: KindTidy, Tidy: &Tidy{Compact: true, CompactAt: 5}},
		"a step refused":          {Name: "Bad", Steps: []Step{{Machine: "local", Dir: "x", Agent: "shell"}}},
	} {
		if _, err := Check(scene, nil); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestATidyTakesItsDefaultsAndItsMachinesOnce(t *testing.T) {
	scene, err := Check(Scene{Name: "Evening", Kind: KindTidy, Tidy: &Tidy{Idle: true, Compact: true, CompactAt: 0, Shells: false, IdleHours: 0, Machines: []string{" gog ", "gog", "", "local"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tidy := *scene.Tidy
	if tidy.IdleHours != DefaultIdleHours || tidy.CompactAt != DefaultCompactAt || strings.Join(tidy.Machines, ",") != "gog,local" {
		t.Fatalf("the defaults: %+v", tidy)
	}
	if !tidy.Covers("gog") || tidy.Covers("roofr") || !(Tidy{}).Covers("roofr") {
		t.Fatal("covers the machines it names, every one when it names none")
	}
	off, _ := Check(Scene{Name: "Evening", Kind: KindTidy, Tidy: &Tidy{Shells: true, IdleHours: 4, CompactAt: 50}}, nil)
	if off.Tidy.IdleHours != 0 || off.Tidy.CompactAt != 0 {
		t.Fatalf("a number of a choice switched off is not kept: %+v", off.Tidy)
	}
}

func TestTheListIsReplacedWholeAndKeptPrivately(t *testing.T) {
	dir := t.TempDir()
	st := New(dir)
	evening := Scene{ID: "same", Name: "Evening", Kind: KindTidy, Tidy: &Tidy{Shells: true}}
	twice := morning()
	twice.ID = "same"
	saved, err := st.Replace([]Scene{evening, twice})
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 2 || saved[0].ID != "same" || saved[1].ID == "" || saved[1].ID == "same" {
		t.Fatalf("ids: %+v", saved)
	}
	if _, err := st.Replace([]Scene{{Name: "Bad"}}); err == nil {
		t.Fatal("a scene without steps saved")
	}
	again := New(dir)
	if list := again.List(); len(list) != 2 || list[1].Steps[0].Model != "opus" {
		t.Fatalf("read back: %+v", list)
	}
	if got, ok := again.Get(saved[1].ID); !ok || got.Name != "Morning" {
		t.Fatalf("one by its id: %+v %v", got, ok)
	}
	info, err := os.Stat(filepath.Join(dir, file))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("the file: %v %v", info.Mode(), err)
	}
	if list := New(t.TempDir()).List(); list == nil || len(list) != 0 {
		t.Fatal("no file is an empty list, never null")
	}
}
