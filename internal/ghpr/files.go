package ghpr

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/dev1antdev/remuda/internal/shell"
)

// File is one file the pull request changes, with GitHub's patch of it.
type File struct {
	Path      string `json:"path"`
	Previous  string `json:"previous,omitempty"` // the name it had before a rename
	Status    string `json:"status"`             // added, removed, modified, renamed, copied, changed, unchanged
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Patch     string `json:"patch,omitempty"`
	NoPatch   bool   `json:"no_patch,omitempty"` // GitHub left the patch out (a binary or a very large file), or it is past MaxPatches
}

// Files is the pull request's files, as many as were read.
type Files struct {
	Files []File `json:"files"`
	Cut   bool   `json:"cut,omitempty"` // the list ran past what is read
}

// MaxFilesRead is how much of the files' list the machine sends; MaxPatches how much patch text the answer carries.
const (
	MaxFilesRead = 8 << 20
	MaxPatches   = 2 << 20
)

// ReadFiles reads the files a hundred at a time (GitHub stops at three thousand), each page an array gh prints one after
// the other; a list cut on the machine keeps the files read whole.
func ReadFiles(host shell.Shell, repo Repo, number int) (Files, error) {
	out, err := runKept(host, []string{"api", "--paginate", fmt.Sprintf("repos/%s/pulls/%d/files?per_page=100", repo, number)}, "", firstBytes(MaxFilesRead), Timeout)
	if err != nil {
		return Files{}, err
	}
	return ParseFiles(out), nil
}

// ParseFiles reads gh's pages of files, stopping at the first that does not decode whole.
func ParseFiles(out string) Files {
	got := Files{Files: []File{}}
	dec := json.NewDecoder(strings.NewReader(out))
	patches := 0
	for {
		if _, err := dec.Token(); err != nil { // a page's [
			if err != io.EOF {
				got.Cut = true
			}
			return got
		}
		for dec.More() {
			var raw struct {
				Filename  string `json:"filename"`
				Previous  string `json:"previous_filename"`
				Status    string `json:"status"`
				Additions int    `json:"additions"`
				Deletions int    `json:"deletions"`
				Patch     string `json:"patch"`
			}
			if err := dec.Decode(&raw); err != nil {
				got.Cut = true
				return got
			}
			f := File{Path: raw.Filename, Previous: raw.Previous, Status: raw.Status, Additions: raw.Additions, Deletions: raw.Deletions, Patch: raw.Patch}
			if f.Patch == "" || patches+len(f.Patch) > MaxPatches {
				f.Patch, f.NoPatch = "", true
			}
			patches += len(f.Patch)
			got.Files = append(got.Files, f)
		}
		if _, err := dec.Token(); err != nil { // its ]
			got.Cut = true
			return got
		}
	}
}
