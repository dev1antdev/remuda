package notes

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestANoteIsKeptPerCheckoutAndOutlivesTheStore(t *testing.T) {
	dir := t.TempDir()
	s := Open(dir)
	kept, err := s.Set("local", "/w/app/", "  Waiting on the API team\r\nfor the new endpoint  ")
	if err != nil || kept != "Waiting on the API team\nfor the new endpoint" {
		t.Fatalf("kept %q, %v", kept, err)
	}
	if got := s.Of("local", "/w/app"); got != kept {
		t.Fatalf("the folder spelt without its slash reads %q", got)
	}
	if s.Of("gog", "/w/app") != "" || s.Of("local", "/w/app-feature") != "" {
		t.Fatal("another machine or folder reads the note")
	}
	again := Open(dir)
	if again.Of("local", "/w/app") != kept {
		t.Fatal("the note did not outlive the store")
	}
	info, err := os.Stat(filepath.Join(dir, file))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("notes.json is %v", info.Mode().Perm())
	}
}

func TestSavingANoteEmptyRemovesIt(t *testing.T) {
	s := Open(t.TempDir())
	if _, err := s.Set("local", "/w/app", "remember"); err != nil {
		t.Fatal(err)
	}
	if kept, err := s.Set("local", "/w/app", " \n "); err != nil || kept != "" || s.Of("local", "/w/app") != "" {
		t.Fatalf("an empty save kept %q, %v", kept, err)
	}
	if err := s.Remove("local", "/w/none"); err != nil {
		t.Fatalf("removing no note: %v", err)
	}
}

func TestAWindowsFolderFindsItsNoteHoweverItIsSpelt(t *testing.T) {
	s := Open(t.TempDir())
	if _, err := s.Set("windows", `c:\Users\me\app\`, "the PC's"); err != nil {
		t.Fatal(err)
	}
	if s.Of("windows", `C:\Users\me\app`) != "the PC's" || s.Of("windows", "C:/Users/me/app") != "the PC's" {
		t.Fatal("a Windows folder spelt another way lost its note")
	}
}

func TestWhatANoteCannotBeIsRefused(t *testing.T) {
	s := Open(t.TempDir())
	cases := map[string]struct {
		machine, folder, text string
		want                  error
	}{
		"no machine":     {"", "/w/app", "x", ErrNoPlace},
		"no folder":      {"local", "", "x", ErrNoPlace},
		"a relative one": {"local", "w/app", "x", ErrRelative},
		"too long":       {"local", "/w/app", strings.Repeat("ж", MaxText/2+1), ErrTooLong},
		"a control byte": {"local", "/w/app", "bell\x07", ErrNotText},
		"not utf-8":      {"local", "/w/app", "\xff\xfe", ErrNotText},
	}
	for name, c := range cases {
		if _, err := s.Set(c.machine, c.folder, c.text); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
	if kept, err := s.Set("local", "/w/app", strings.Repeat("a", MaxText)); err != nil || len(kept) != MaxText {
		t.Fatalf("a note of exactly MaxText: %d, %v", len(kept), err)
	}
	if kept, _ := s.Set("local", "/w/app", "a\tb\nc"); kept != "a\tb\nc" {
		t.Fatalf("tabs and line breaks: %q", kept)
	}
}

func TestADamagedFileIsReadAsNone(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, file), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Open(dir)
	if s.Of("local", "/w/app") != "" {
		t.Fatal("a damaged file read as a note")
	}
	if _, err := s.Set("local", "/w/app", "fresh"); err != nil || Open(dir).Of("local", "/w/app") != "fresh" {
		t.Fatalf("writing over a damaged file: %v", err)
	}
}
