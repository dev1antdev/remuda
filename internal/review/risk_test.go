package review

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

func TestTestsAreToldByEveryLanguagesHabit(t *testing.T) {
	for p, want := range map[string]bool{
		"internal/review/risk_test.go": true, "web/src/a.test.ts": true, "web/src/a.spec.tsx": true, "tests/test_auth.py": true,
		"app/auth_test.py": true, "spec/models/user_spec.rb": true, "src/__tests__/x.ts": true, "App/LoginTests.swift": true,
		"src/main/FooTest.java": true, "internal/review/testdata/a.txt": true,
		"internal/review/risk.go": false, "web/src/latest.ts": false, "contest/a.go": false, "src/attest.py": false,
	} {
		if got := isTest(p); got != want {
			t.Errorf("isTest(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestSensitivePathsByKind(t *testing.T) {
	for p, want := range map[string]string{
		"package-lock.json": KindLockfile, "web/pnpm-lock.yaml": KindLockfile, "go.sum": KindLockfile, "Cargo.lock": KindLockfile,
		".github/workflows/ci.yml": KindCI, ".gitlab-ci.yml": KindCI, "Jenkinsfile": KindCI, "ops/.circleci/config.yml": KindCI,
		"db/migrations/0042_users.sql": KindMigration, "prisma/schema.prisma": KindMigration, "alembic/versions/1a2b.py": KindMigration,
		"internal/auth/devices.go": KindAuth, "src/auth.ts": KindAuth, "src/oauthClient.ts": KindAuth, "app/signIn.tsx": KindAuth,
		"lib/password_reset.rb": KindAuth, "internal/secrets/rules.go": KindAuth, "api/permissions.py": KindAuth,
		"README.md": "", "src/tokenizer.ts": "", "internal/sessions/input.go": "", "src/author.ts": "", "go.mod": "",
	} {
		if got := sensitiveKind(p); got != want {
			t.Errorf("sensitiveKind(%q) = %q, want %q", p, got, want)
		}
	}
}

func TestAProfileCountsTheSourceWithNoTestBesideIt(t *testing.T) {
	paths := []string{
		"internal/review/risk.go", "internal/review/risk_test.go", // tested beside it
		"web/src/auth.ts", "web/test/auth.test.ts", // tested by name, elsewhere
		"internal/server/roster.go", "internal/app/app.go", "cmd/remuda/main.go", "cmd/remuda/old.go", // untested; old.go is gone
		"README.md", "db/migrations/0001.sql", "db/migrations/0002.sql", "db/migrations/0003.sql", "db/migrations/0004.sql", "go.sum",
	}
	profile := profileOf(paths, map[string]bool{"cmd/remuda/old.go": true}, 120, 30, 1, 4000)
	if profile.Files != len(paths) || profile.Lines() != 120+30+100 {
		t.Fatalf("size: %+v, lines %d", profile, profile.Lines())
	}
	if profile.UntestedCount != 3 || !reflect.DeepEqual(profile.Untested, []string{"cmd/remuda/main.go", "internal/app/app.go", "internal/server/roster.go"}) {
		t.Fatalf("untested: %d %v", profile.UntestedCount, profile.Untested)
	}
	kinds := map[string]Sensitive{}
	for _, s := range profile.Sensitive {
		kinds[s.Kind] = s
	}
	if m := kinds[KindMigration]; m.Count != 4 || len(m.Paths) != MaxNamed || m.Paths[0] != "db/migrations/0001.sql" {
		t.Fatalf("migrations: %+v", m)
	}
	if kinds[KindAuth].Count != 2 || kinds[KindLockfile].Count != 1 || len(profile.Sensitive) != 3 {
		t.Fatalf("sensitive: %+v", profile.Sensitive)
	}
}

func TestTheRiskSumsItsReasonsWeightiestFirst(t *testing.T) {
	if risk := RiskOf(Profile{Files: 2, Added: 10}, Signals{}); risk.Score != 0 || risk.Level != "low" || len(risk.Reasons) != 0 {
		t.Fatalf("a small change: %+v", risk)
	}
	medium := RiskOf(Profile{Files: 4, Added: 100, Removed: 60}, Signals{Collision: "file", CollidesWith: "web"})
	if medium.Score != pointsMedium+pointsCollisionFile || medium.Level != "low" || medium.Reasons[0].Kind != "size" || medium.Reasons[1].With != "web" || medium.Reasons[1].Same {
		t.Fatalf("a medium change sharing a file: %+v", medium)
	}
	risky := RiskOf(Profile{Files: 30, Added: 900, UntestedCount: 6, Untested: []string{"a.go"}, Sensitive: []Sensitive{{Kind: KindMigration, Count: 1}, {Kind: KindLockfile, Count: 1}}},
		Signals{Secrets: 2, ChecksFailed: 1, Conflicts: true, Changes: true, Collision: "lines", CollidesWith: "api"})
	if risky.Score != 100 || risky.Level != "high" {
		t.Fatalf("everything at once is capped at 100: %+v", risky)
	}
	var kinds []string
	for _, reason := range risky.Reasons {
		kinds = append(kinds, reason.Kind)
	}
	if want := []string{"secrets", "size", "untested", "checks", "collision", "migration", "conflicts", "changes", "lockfile"}; !reflect.DeepEqual(kinds, want) {
		t.Fatalf("order: %v, want %v", kinds, want)
	}
	if risky.Reasons[2].Points != pointsUntestedMost {
		t.Fatalf("six untested files are capped: %+v", risky.Reasons[2])
	}
	if risk := RiskOf(Profile{Sensitive: []Sensitive{{Kind: KindAuth, Count: 1}}}, Signals{Collision: "lines"}); risk.Score != pointsAuth+pointsCollisionLines || risk.Level != "medium" {
		t.Fatalf("auth and the same lines: %+v", risk)
	}
}

func TestTheSweepWeighsTheWorkAndKeepsAScanUntilItMoves(t *testing.T) {
	dir, git, write := scratchCheckout(t)
	write("a.go", "package a\n")
	git("add", "-A")
	git("commit", "-qm", "init")
	git("remote", "add", "origin", "git@github.com:example/app.git")
	write("a.go", "package a\n\nfunc A() {}\n")
	if err := os.MkdirAll(filepath.Join(dir, "db", "migrations"), 0o755); err != nil {
		t.Fatal(err)
	}
	write("db/migrations/0001.sql", strings.Repeat("create table t (id int);\n", 40))
	write("-dash.txt", "a name starting with a dash\n")

	machine := shell.Local{Name: "local"}
	store := New(t.TempDir())
	store.NoteScan("local", dir, SecretScan{Findings: nil, More: 3, At: time.Now()})
	store.Badges(machine, []string{dir})
	if remote := store.Remote("local", dir); remote != "git@github.com:example/app.git" {
		t.Fatalf("remote = %q", remote)
	}
	profile, ok := store.Profile("local", dir)
	if !ok || profile.Files != 3 || profile.Added != 2 || profile.NewFiles != 2 || profile.NewBytes < 960 || profile.UntestedCount != 1 || len(profile.Sensitive) != 1 {
		t.Fatalf("profile: %+v", profile)
	}
	if n := store.Secrets("local", dir); n != 3 {
		t.Fatalf("a check of the work the sweep reads next is still the answer: %d", n)
	}
	store.Badges(machine, []string{dir})
	if n := store.Secrets("local", dir); n != 3 {
		t.Fatalf("an unchanged work keeps its check: %d", n)
	}
	write("a.go", "package a\n\nfunc B() {}\n")
	store.Badges(machine, []string{dir})
	if n := store.Secrets("local", dir); n != 0 {
		t.Fatalf("a check of work that moved since is no answer: %d", n)
	}
	store.NoteScan("local", dir, SecretScan{More: 1, At: time.Now()})
	if n := store.Secrets("local", dir); n != 1 {
		t.Fatalf("a check of the work as it is now: %d", n)
	}
	git("remote", "remove", "origin")
	store.Badges(machine, []string{dir})
	if remote := store.Remote("local", dir); remote != "" {
		t.Fatalf("a checkout with no remote keeps %q", remote)
	}
}
