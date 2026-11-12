package collisions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/shell"
)

// radarOn is a radar over two machines that are both this one — the main checkout's worktrees web on "local" and api
// on "gog" — with its pushes, mutes and roster wakes noted.
type radarOn struct {
	*Service
	pushed  []push.Payload
	changed int
	muted   bool
	away    bool
}

func newRadar(t *testing.T, dir string, workspaces []Workspace) *radarOn {
	t.Helper()
	r := &radarOn{}
	r.Service = New(dir, Inputs{
		Workspaces: func() []Workspace { return workspaces },
		Host: func(id string) (shell.Host, bool) {
			if r.away && id == "gog" {
				return nil, false
			}
			return shell.Local{Name: id}, true
		},
		Changed: func() { r.changed++ },
		Notify:  func(p push.Payload) { r.pushed = append(r.pushed, p) },
		Muted:   func(string, string) bool { return r.muted },
	})
	return r
}

func TestTheRadarTellsEachNewOverlapOfTheSameLinesOnce(t *testing.T) {
	repo := newRepository(t)
	web, api := repo.worktree("web"), repo.worktree("api")
	key := RepoKey("local", repo.main, repo.origin)
	workspaces := []Workspace{
		{Machine: "local", ID: "w1", Label: "web", Dir: web, Repo: key, Pane: "p1", Agents: true, Working: true},
		{Machine: "gog", ID: "w2", Label: "api", Dir: api, Repo: key, Pane: "p2", Agents: true, PR: true},
	}
	state := t.TempDir()
	radar := newRadar(t, state, workspaces)
	radar.Sweep()
	if len(radar.For("local", web)) != 0 || radar.changed != 0 {
		t.Fatalf("no work yet: %+v", radar.For("local", web))
	}

	repo.edit(web, "src/auth.ts", 40, 44, "web\n")
	repo.edit(api, "src/auth.ts", 42, 42, "api\n")
	radar.Sweep()
	webSide, apiSide := radar.For("local", web), radar.For("gog", api)
	if len(webSide) != 1 || webSide[0].With.Name != "api" || !webSide[0].Lines || webSide[0].First || webSide[0].Why != WhyPR || webSide[0].Main != "main" {
		t.Fatalf("web's side: %+v", webSide)
	}
	if len(apiSide) != 1 || apiSide[0].With.Workspace != "w1" || !apiSide[0].First {
		t.Fatalf("api's side: %+v", apiSide)
	}
	if len(radar.pushed) != 1 || radar.pushed[0].Kind != "collision" || radar.pushed[0].Machine != "local" || radar.pushed[0].Pane != "p1" || radar.pushed[0].Title != "api and web change the same lines" {
		t.Fatalf("the push goes to the one that lands second: %+v", radar.pushed)
	}
	if radar.changed != 1 {
		t.Fatalf("the roster is woken once: %d", radar.changed)
	}
	radar.Sweep()
	if len(radar.pushed) != 1 || radar.changed != 1 {
		t.Fatalf("a standing overlap is no news: %d pushes, %d wakes", len(radar.pushed), radar.changed)
	}
	again := newRadar(t, state, workspaces)
	if again.Sweep(); len(again.pushed) != 0 {
		t.Fatalf("a restart tells a standing overlap again: %+v", again.pushed)
	}

	radar.away = true
	repo.edit(api, "src/auth.ts", 42, 42, "line \n")
	radar.Sweep()
	if len(radar.For("local", web)) != 0 {
		t.Fatalf("a machine away takes its collisions with it: %+v", radar.For("local", web))
	}
	radar.away = false
	repo.edit(api, "src/auth.ts", 42, 42, "api\n")
	radar.Sweep()
	if len(radar.pushed) != 1 {
		t.Fatalf("an overlap that stood while its machine was away is no news when it answers again: %+v", radar.pushed)
	}

	repo.edit(api, "src/auth.ts", 42, 42, "line \n")
	repo.edit(api, "src/auth.ts", 2, 2, "far from web\n")
	radar.Sweep()
	if side := radar.For("local", web); len(side) != 1 || side[0].Lines {
		t.Fatalf("the same file, lines apart: %+v", side)
	}
	repo.edit(api, "src/auth.ts", 43, 43, "api again\n")
	radar.Sweep()
	if len(radar.pushed) != 2 {
		t.Fatalf("a new episode is told again: %+v", radar.pushed)
	}

	radar.muted = true
	repo.edit(api, "src/auth.ts", 43, 43, "line x\n")
	radar.Sweep()
	repo.edit(api, "src/auth.ts", 41, 41, "api, muted\n")
	radar.Sweep()
	if len(radar.pushed) != 2 {
		t.Fatalf("a muted folder is told nothing: %+v", radar.pushed)
	}
	if _, err := os.Stat(filepath.Join(state, episodesFile)); err != nil {
		t.Fatal(err)
	}
}

func TestAWorkspaceWithNoPeerIsNotRead(t *testing.T) {
	repo := newRepository(t)
	web := repo.worktree("web")
	key := RepoKey("local", repo.main, repo.origin)
	read := 0
	radar := New(t.TempDir(), Inputs{
		Workspaces: func() []Workspace {
			return []Workspace{{Machine: "local", ID: "w1", Label: "web", Dir: web, Repo: key, Agents: true}, {Machine: "local", ID: "w2", Label: "app", Dir: repo.main, Repo: key}}
		},
		Host: func(id string) (shell.Host, bool) { read++; return shell.Local{Name: id}, true },
	})
	radar.Sweep()
	if read != 0 {
		t.Fatalf("one active workspace on a repository is no collision to read: %d reads", read)
	}
}
