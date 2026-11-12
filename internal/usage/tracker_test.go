package usage

import (
	"os"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/transcript"
)

// memFiles is a machine's transcripts in memory: the version moves with the size.
type memFiles map[string][]byte

func (m memFiles) Version(path string) (transcript.Version, error) {
	b, ok := m[path]
	if !ok {
		return transcript.Version{}, os.ErrNotExist
	}
	return transcript.Version{Size: int64(len(b)), MTime: int64(len(b))}, nil
}

func (m memFiles) DirVersions(string) (map[string]transcript.Version, error) {
	return map[string]transcript.Version{}, nil
}

func (m memFiles) ReadFrom(path string, offset int64) ([]byte, error) {
	b := m[path]
	if offset > int64(len(b)) {
		offset = int64(len(b))
	}
	return append([]byte(nil), b[offset:]...), nil
}

const (
	oldFile  = "/h/.claude/projects/-p/old.jsonl"
	liveFile = "/h/.claude/projects/-p/live.jsonl"
)

// a conversation moved to a new file: the live one opens with a copy of the old one's last two responses
func continuedFiles() memFiles {
	return memFiles{
		oldFile:  []byte(rec("m1", "claude-fable-5-1", "2026-10-06T10:00:00Z", 10) + rec("m2", "claude-fable-5-1", "2026-10-06T10:01:00Z", 20) + rec("m3", "claude-fable-5-1", "2026-10-06T10:02:00Z", 30)),
		liveFile: []byte(rec("m2", "claude-fable-5-1", "2026-10-06T10:01:00Z", 20) + rec("m3", "claude-fable-5-1", "2026-10-06T10:02:00Z", 30) + rec("m4", "claude-fable-5-1", "2026-10-06T10:03:00Z", 40)),
	}
}

func TestLineageCountsACopiedResponseOnce(t *testing.T) {
	tr := NewTracker()
	tr.Observe("m", continuedFiles(), "p1", []string{oldFile, liveFile})
	d, ok := tr.Display("m", "p1")
	if !ok || d.Turns != 4 || d.Tokens.Output != 10+20+30+40 {
		t.Fatalf("display %+v %v, want the four responses once", d, ok)
	}
	if d.Context != 100+1000 {
		t.Fatalf("context %d, want the live file's last response", d.Context)
	}
}

func TestAPredecessorFoundLaterLeavesOutWhatTheLiveFileCounts(t *testing.T) {
	tr := NewTracker()
	files := continuedFiles()
	tr.Observe("m", files, "p1", []string{liveFile})
	if d, _ := tr.Display("m", "p1"); d.Turns != 3 {
		t.Fatalf("the live file alone: %d turns", d.Turns)
	}
	tr.Observe("m", files, "p1", []string{oldFile, liveFile})
	d, _ := tr.Display("m", "p1")
	if d.Turns != 4 || d.Tokens.Output != 100 {
		t.Fatalf("with the predecessor: %+v, want m1 added once", d)
	}
}

func TestAFileJustMovedToKeepsTheMeter(t *testing.T) {
	files := continuedFiles()
	files[liveFile] = []byte(rec("m2", "claude-fable-5-1", "2026-10-06T10:01:00Z", 20) + rec("m3", "claude-fable-5-1", "2026-10-06T10:02:00Z", 30))
	tr := NewTracker()
	tr.Observe("m", files, "p1", []string{oldFile, liveFile})
	d, ok := tr.Display("m", "p1")
	if !ok || d.Turns != 3 || d.Tokens.Output != 60 || d.Context != 1100 || d.Name != "Fable 5.1" {
		t.Fatalf("display %+v %v, want the old file's sum and its last response's context", d, ok)
	}
}

// listedFiles is memFiles whose folders list the files in them
type listedFiles struct{ memFiles }

func (l listedFiles) DirVersions(dir string) (map[string]transcript.Version, error) {
	out := map[string]transcript.Version{}
	for path := range l.memFiles {
		if strings.HasPrefix(path, dir+"/") && !strings.Contains(strings.TrimPrefix(path, dir+"/"), "/") {
			out[path], _ = l.Version(path)
		}
	}
	return out, nil
}

func TestAForkedSubagentsCopyOfItsSpawningResponseCountsOnce(t *testing.T) {
	// a fork's transcript opens with a copy of the parent's response that spawned it, its usage and all
	fork := strings.TrimSuffix(liveFile, ".jsonl") + "/subagents/agent-a1.jsonl"
	files := listedFiles{memFiles{
		liveFile: []byte(rec("m1", "claude-fable-5-1", "2026-10-06T10:00:00Z", 10) + rec("m2", "claude-fable-5-1", "2026-10-06T10:01:00Z", 20)),
		fork:     []byte(rec("m1", "claude-fable-5-1", "2026-10-06T10:00:00Z", 10) + rec("f1", "claude-fable-5-1", "2026-10-06T10:00:30Z", 5)),
	}}
	tr := NewTracker()
	tr.Observe("m", files, "p1", []string{liveFile})
	d, ok := tr.Display("m", "p1")
	if !ok || d.Turns != 3 || d.Tokens.Output != 35 {
		t.Fatalf("display %+v %v, want m1, m2 and the fork's own f1, each once", d, ok)
	}
}

func TestTheMeterSaysTheSpeedOfTheLastResponse(t *testing.T) {
	fast := strings.Replace(rec("m2", "claude-opus-4-8", "2026-11-20T10:01:00Z", 20), `"cache_creation_input_tokens":0`, `"cache_creation_input_tokens":0,"speed":"fast"`, 1)
	files := memFiles{liveFile: []byte(rec("m1", "claude-opus-4-8", "2026-11-20T10:00:00Z", 10) + fast)}
	tr := NewTracker()
	tr.Observe("m", files, "p1", []string{liveFile})
	if d, ok := tr.Display("m", "p1"); !ok || d.Speed != "fast" || d.Model != "claude-opus-4-8" {
		t.Fatalf("display %+v", d)
	}
	standard := NewTracker()
	standard.Observe("m", memFiles{liveFile: []byte(fast + rec("m3", "claude-opus-4-8", "2026-11-20T10:02:00Z", 30))}, "p1", []string{liveFile})
	if d, _ := standard.Display("m", "p1"); d.Speed != "" {
		t.Fatalf("fast mode turned off since: %+v", d)
	}
}
