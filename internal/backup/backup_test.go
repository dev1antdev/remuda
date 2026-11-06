package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// stateDir is a cockpit's state dir with a live WAL database whose last write sits in the -wal file only
func stateDir(t *testing.T) (string, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "token"), "secret-token\n")
	write(t, filepath.Join(dir, "schedules.json"), "[]")
	write(t, filepath.Join(dir, "tailscale", "tailscaled.state"), "{node}")
	write(t, filepath.Join(dir, "tailscale", "tailscaled.log1.txt"), "noise")
	write(t, filepath.Join(dir, "ssh", "ctl-gog"), "socket")
	write(t, filepath.Join(dir, "backups", "rp", "remuda-20260101T000000Z.tar.gz"), "old")
	write(t, filepath.Join(dir, "review.json.123456789"), "half written")
	write(t, filepath.Join(dir, "serve.lock"), "")
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "tasks.db")+"?_pragma=journal_mode(WAL)&_pragma=wal_autocheckpoint(0)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{"create table tasks (title text)", "insert into tasks values ('kept through the WAL')"} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return dir, db
}

func names(entries []Entry) []string {
	var out []string
	for _, e := range entries {
		out = append(out, e.Name)
	}
	sort.Strings(out)
	return out
}

func TestASnapshotHoldsTheStateAndLeavesTheRestOut(t *testing.T) {
	dir, _ := stateDir(t)
	at := time.Date(2026, 11, 5, 2, 30, 0, 0, time.UTC)
	archive, manifest, err := Snapshot(dir, "rp-orange", "v0.12.5", at)
	if err != nil {
		t.Fatal(err)
	}
	got, entries, err := Read(archive)
	if err != nil {
		t.Fatal(err)
	}
	if got.Host != "rp-orange" || got.Version != "v0.12.5" || !got.At.Equal(at) || got.Files != manifest.Files || manifest.Files != 4 {
		t.Fatalf("manifest %+v, packed %+v", got, manifest)
	}
	want := "schedules.json tailscale/ tailscale/tailscaled.state tasks.db token"
	if strings.Join(names(entries), " ") != want {
		t.Fatalf("entries %v, want %s", names(entries), want)
	}
	for _, e := range entries {
		if e.Name != "tasks.db" {
			continue
		}
		copied := filepath.Join(t.TempDir(), "copy.db")
		write(t, copied, string(e.Data))
		db, err := sql.Open("sqlite", "file:"+copied)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		var title string
		if err := db.QueryRow("select title from tasks").Scan(&title); err != nil || title != "kept through the WAL" {
			t.Fatalf("the copied database reads %q, %v", title, err)
		}
	}
}

// copiedTitle is the title the tasks table of a snapshot's tasks.db holds, "" when the snapshot has no database
func copiedTitle(t *testing.T, archive []byte) string {
	t.Helper()
	_, entries, err := Read(archive)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name != "tasks.db" {
			continue
		}
		copied := filepath.Join(t.TempDir(), "copy.db")
		write(t, copied, string(e.Data))
		db, err := sql.Open("sqlite", copied)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		var title string
		if err := db.QueryRow("select title from tasks").Scan(&title); err != nil {
			t.Fatalf("the copied database: %v", err)
		}
		return title
	}
	return ""
}

func TestACopyOfAnEmptyOrAWrongFolderIsRefused(t *testing.T) {
	if _, _, err := Snapshot(t.TempDir(), "rp-orange", "v", time.Now()); err == nil || !strings.Contains(err.Error(), "no token") {
		t.Fatalf("an empty folder was packed as a good copy: %v", err)
	}
	if _, _, err := Snapshot(filepath.Join(t.TempDir(), "missing"), "rp-orange", "v", time.Now()); err == nil {
		t.Fatal("a folder that is not there was packed")
	}
}

func TestASnapshotLeavesOutEveryHalfWrittenFile(t *testing.T) {
	dir, _ := stateDir(t)
	write(t, filepath.Join(dir, "tailscale", "tailscaled.state.tmp2847561930"), "{half a node")
	write(t, filepath.Join(dir, "tailscale", "certs", "box.ts.net.crt.tmp93"), "half a cert")
	write(t, filepath.Join(dir, "prefs.json.4821"), "{half")
	archive, manifest, err := Snapshot(dir, "rp-orange", "v", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	_, entries, err := Read(archive)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names(entries) {
		if strings.Contains(name, ".tmp") || strings.HasPrefix(name, "prefs.json") {
			t.Fatalf("a file still being written went into the copy: %s (%d files)", name, manifest.Files)
		}
	}
}

func TestAStateDirBehindASymlinkOrWithAnOddNameIsCopiedWhole(t *testing.T) {
	dir, _ := stateDir(t)
	link := filepath.Join(t.TempDir(), "remuda")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	archive, manifest, err := Snapshot(link, "rp-orange", "v", time.Now())
	if err != nil || manifest.Files != 4 || copiedTitle(t, archive) != "kept through the WAL" {
		t.Fatalf("through a symlink: %d files, %v", manifest.Files, err)
	}

	parent := filepath.Join(t.TempDir(), "Application Support #2 ?x")
	odd := filepath.Join(parent, "remuda")
	write(t, filepath.Join(odd, "token"), "t")
	made, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: filepath.ToSlash(filepath.Join(odd, "tasks.db"))}).String())
	if err != nil {
		t.Fatal(err)
	}
	defer made.Close()
	for _, q := range []string{"create table tasks (title text)", "insert into tasks values ('under an odd name')"} {
		if _, err := made.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	archive, manifest, err = Snapshot(odd, "rp-orange", "v", time.Now())
	if err != nil || manifest.Files != 2 || copiedTitle(t, archive) != "under an odd name" {
		t.Fatalf("under %q: %d files, %v", odd, manifest.Files, err)
	}
	left, _ := os.ReadDir(parent)
	if len(left) != 1 {
		t.Fatalf("the copy left files beside the state dir: %v", left)
	}
}

func TestPuttingACopyInPlaceClearsTheHalfWrittenOnes(t *testing.T) {
	folder := t.TempDir()
	write(t, filepath.Join(folder, "remuda-20261104T033000Z.tar.gz.partial"), "cut off yesterday")
	write(t, filepath.Join(folder, "remuda-20261105T033000Z.tar.gz.partial"), "today's")
	write(t, filepath.Join(folder, "notes.txt"), "the person's")
	out, err := shell.Text(shell.Local{Name: "local"}, FinishScript(folder, "remuda-20261105T033000Z.tar.gz", 2), Timeout)
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	var left []string
	entries, _ := os.ReadDir(folder)
	for _, e := range entries {
		left = append(left, e.Name())
	}
	if strings.Join(left, " ") != "notes.txt remuda-20261105T033000Z.tar.gz" {
		t.Fatalf("left %v", left)
	}
}

// tarball is a gzipped tar of the given names and bodies, a manifest first unless told otherwise
func tarball(t *testing.T, manifest bool, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	if manifest {
		files[ManifestName] = `{"format":1,"host":"rp-orange"}`
	}
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(body))
	}
	tw.Close()
	zw.Close()
	return buf.Bytes()
}

func TestABackupThatLeadsOutOfTheStateDirIsRefused(t *testing.T) {
	for _, name := range []string{"../escape", "/etc/passwd", "a/../../b", "./token"} {
		if _, _, err := Read(tarball(t, true, map[string]string{name: "x"})); err == nil {
			t.Errorf("%q was read", name)
		}
	}
	if _, _, err := Read(tarball(t, false, map[string]string{"token": "x"})); err == nil {
		t.Error("an archive with no manifest was read as a backup")
	}
	if _, _, err := Read([]byte("not a gzip")); err == nil {
		t.Error("a file that is no archive was read")
	}
}

func TestARestoreSetsTheOldDirAsideAndKeepsTheBackupsHere(t *testing.T) {
	source, _ := stateDir(t)
	archive, _, err := Snapshot(source, "rp-orange", "v0.12.5", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "remuda")
	write(t, filepath.Join(dir, "token"), "another token\n")
	write(t, filepath.Join(dir, "backups", "mac", "remuda-20261101T000000Z.tar.gz"), "kept")
	at := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	_, aside, err := Restore(archive, dir, at)
	if err != nil {
		t.Fatal(err)
	}
	if aside != dir+".before-restore-20261105T090000Z" {
		t.Fatalf("aside at %s", aside)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "token")); string(b) != "secret-token\n" {
		t.Fatalf("token %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(aside, "token")); string(b) != "another token\n" {
		t.Fatalf("the old dir's token %q", b)
	}
	if _, err := os.Stat(filepath.Join(dir, "backups", "mac", "remuda-20261101T000000Z.tar.gz")); err != nil {
		t.Fatalf("the backups kept here did not come along: %v", err)
	}
	if info, _ := os.Stat(filepath.Join(dir, "tailscale", "tailscaled.state")); info == nil || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("a restored file is not private: %v", info)
	}
}

func TestTheNightlyRunIsDueOnceItsTimeCame(t *testing.T) {
	zone := time.UTC
	settings := Settings{Machine: "mac", At: "03:30", Keep: 3}
	at := func(day, hour, minute int) time.Time { return time.Date(2026, 11, day, hour, minute, 0, 0, zone) }
	cases := []struct {
		name string
		last *Result
		now  time.Time
		want bool
	}{
		{"never taken, after the time", nil, at(5, 4, 0), true},
		{"never taken, before the time: yesterday's is due", nil, at(5, 2, 0), true},
		{"taken tonight", &Result{At: at(5, 3, 30)}, at(5, 9, 0), false},
		{"taken yesterday, before tonight's time", &Result{At: at(4, 3, 30)}, at(5, 3, 0), false},
		{"taken yesterday, tonight's time came", &Result{At: at(4, 3, 30)}, at(5, 3, 31), true},
		{"failed half an hour ago", &Result{At: at(5, 3, 30), Error: "away"}, at(5, 4, 0), false},
		{"failed an hour ago", &Result{At: at(5, 3, 30), Error: "away"}, at(5, 4, 30), true},
	}
	for _, c := range cases {
		if due, _ := Due(settings, c.last, c.now, zone); due != c.want {
			t.Errorf("%s: due %v", c.name, due)
		}
	}
	if due, _ := Due(Settings{At: "03:30", Keep: 3}, nil, at(5, 4, 0), zone); due {
		t.Error("due with no machine picked")
	}
	if next := Next("03:30", at(5, 2, 0), zone); !next.Equal(at(5, 3, 30)) {
		t.Errorf("next %v", next)
	}
	if next := Next("03:30", at(5, 4, 0), zone); !next.Equal(at(6, 3, 30)) {
		t.Errorf("next %v", next)
	}
	for _, bad := range []Settings{{At: "3:30", Keep: 3}, {At: "24:00", Keep: 3}, {At: "03:30", Keep: 0}, {At: "03:30", Keep: MaxKeep + 1}} {
		if Check(bad) == nil {
			t.Errorf("%+v passed", bad)
		}
	}
}

// here is this machine as a backup target, its files written as the fleet's local machine writes them
type here struct{ machine shell.Local }

func (h here) ID() string    { return h.machine.ID() }
func (h here) Label() string { return "this machine" }
func (h here) Home() string  { return h.machine.Home() }
func (h here) Local() bool   { return true }
func (h here) Run(script string, timeout time.Duration) ([]byte, error) {
	return h.machine.Run(script, timeout)
}
func (h here) Stream(script string, timeout time.Duration, sink func([]byte)) error {
	return h.machine.Stream(script, timeout, sink)
}
func (here) PutFile(path string, data []byte, timeout time.Duration) error {
	return os.WriteFile(path, data, 0o600)
}

func TestRunsGoToTheTargetAndTheOldestArePruned(t *testing.T) {
	dir, _ := stateDir(t)
	now := time.Date(2026, 11, 5, 3, 30, 0, 0, time.UTC)
	s := Open(dir, "rp-orange", "v0.12.5")
	s.Now = func() time.Time { return now }
	s.Zone = time.UTC
	var told []string
	s.Notify = func(title, body string) { told = append(told, body) }
	s.Target = func(id string) (Target, error) { return here{shell.Local{Name: "local"}}, nil }
	if _, err := s.Run(true); !errors.Is(err, ErrNoTarget) {
		t.Fatalf("a run with no machine picked: %v", err)
	}
	if err := s.Put(Settings{Machine: "local", At: "03:30", Keep: 2}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		result, err := s.Run(false)
		if err != nil || result.Size == 0 || result.Files != 5 { // the four of the dir and backup.json
			t.Fatalf("run %d: %+v %v", i, result, err)
		}
		now = now.Add(24 * time.Hour)
	}
	folder, files, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if folder != filepath.Join(dir, "backups", "rp-orange") || len(files) != 2 || files[0].Name != "remuda-20261107T033000Z.tar.gz" || files[1].Name != "remuda-20261106T033000Z.tar.gz" {
		t.Fatalf("on the target %s: %+v", folder, files)
	}
	if status := s.Status(); status.Last == nil || status.Last.Pruned != 1 || status.LastGood == nil || !status.Next.Equal(time.Date(2026, 11, 9, 3, 30, 0, 0, time.UTC)) {
		t.Fatalf("status %+v", status)
	}
	if _, err := os.Stat(filepath.Join(dir, "backups", "rp", "remuda-20260101T000000Z.tar.gz")); err != nil {
		t.Fatal("another cockpit's backup kept here went")
	}

	s.Target = func(id string) (Target, error) { return nil, errors.New("GOG is away") }
	for i := 0; i < 2; i++ {
		if _, err := s.Run(false); err == nil {
			t.Fatal("a run to an away machine went through")
		}
	}
	if len(told) != 1 || !strings.Contains(told[0], "GOG is away") {
		t.Fatalf("told %v: a failure is said once per run of them", told)
	}
	reopened := Open(dir, "rp-orange", "v0.12.5")
	if status := reopened.Status(); status.Settings.Keep != 2 || status.Last.Error == "" || status.LastGood == nil {
		t.Fatalf("after a restart %+v", status)
	}
}

func TestARemoteTargetGetsTheFileBesideItsOwnState(t *testing.T) {
	dir, _ := stateDir(t)
	var put string
	fake := &shell.Fake{Name: "gog", Remote: true, Answer: func(script string) (string, error) {
		switch {
		case strings.Contains(script, `d="$d/backups/"'rp-orange'`):
			return "/Users/me/Library/Application Support/remuda/backups/rp-orange\n", nil
		case strings.Contains(script, "mv -f"):
			return "pruned remuda-20261001T033000Z.tar.gz\n", nil
		}
		return "", nil
	}}
	s := Open(dir, "rp-orange", "v0.12.5")
	s.Target = func(id string) (Target, error) { return remote{fake, &put}, nil }
	if err := s.Put(Settings{Machine: "gog", At: "03:30", Keep: 14}); err != nil {
		t.Fatal(err)
	}
	result, err := s.Run(true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(put, "/backups/rp-orange/"+NameAt(result.At)+".partial") || result.Pruned != 1 || !strings.HasSuffix(result.Path, NameAt(result.At)) {
		t.Fatalf("put %s, result %+v", put, result)
	}
	scripts := fake.Scripts()
	if len(scripts) != 2 || !strings.Contains(scripts[1], "tail -n +15") {
		t.Fatalf("scripts %q", scripts)
	}
}

// remote is a Fake that notes where a file was put
type remote struct {
	*shell.Fake
	put *string
}

func (r remote) PutFile(path string, data []byte, timeout time.Duration) error {
	*r.put = path
	return nil
}

func TestTheHostFolderIsAPlainName(t *testing.T) {
	for host, want := range map[string]string{"rp-orange": "rp-orange", "Sergiis-MacBook-Pro.local": "Sergiis-MacBook-Pro", "a b/c": "a-b-c", "": "remuda", "..": "remuda"} {
		if got := HostFolder(host); got != want {
			t.Errorf("HostFolder(%q) = %q, want %q", host, got, want)
		}
	}
	list := ParseList("remuda-20261105T033000Z.tar.gz\x1f2048\nnotes.txt\x1f4\nremuda-20261106T033000Z.tar.gz\x1f 4096\n")
	if len(list) != 2 || list[0].Size != 4096 || list[1].At.Day() != 5 {
		t.Fatalf("list %+v", list)
	}
}
