package gitgraph

import (
	"errors"
	"strconv"
	"strings"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

const (
	MaxFiles = 500     // a commit's files listed, the rest counted
	MaxPatch = 600_000 // the bytes of one file's patch
	maxList  = 2_000_000
)

// File is one file a commit changed against its first parent: its status letter, its path (and the path it had before,
// for a rename or a copy), the lines added and removed, binary when git counts none.
type File struct {
	Status string `json:"status"`
	Path   string `json:"path"`
	From   string `json:"from,omitempty"`
	Add    int    `json:"add"`
	Del    int    `json:"del"`
	Binary bool   `json:"binary,omitempty"`
}

// Detail is what the history's row does not carry: the whole message, the committer, the files.
type Detail struct {
	Hash      string `json:"hash"`
	Parent    string `json:"parent,omitempty"` // the first parent the files are counted against, "" for a root commit
	Author    string `json:"author"`
	Email     string `json:"email"`
	Time      int64  `json:"time"`
	Committer string `json:"committer"`
	CEmail    string `json:"committer_email"`
	CTime     int64  `json:"committed"`
	Message   string `json:"message"`
	Files     []File `json:"files"`
	More      int    `json:"more"` // files past MaxFiles
}

// ErrNoCommit is a hash the repository does not hold.
var ErrNoCommit = errors.New("the repository holds no such commit")

// commitPrelude enters the folder and stops with a word of its own when the commit is not there; else $parent is its
// first parent and $range what the files are counted over.
func commitPrelude(dir, hash string) string {
	return script.PATH + "; " + Read +
		`cd ` + script.Quote(dir) + ` 2>/dev/null || { printf 'gone\n'; exit 0; }; sha=` + script.Quote(hash) + `; ` +
		`g cat-file -e "$sha^{commit}" 2>/dev/null || { printf 'nocommit\n'; exit 0; }; ` +
		`parent=$(g rev-parse -q --verify "$sha^1" 2>/dev/null); ` +
		`tree() { if [ -n "$parent" ]; then g diff-tree -r -M -z --no-commit-id "$@" "$parent" "$sha"; else g diff-tree --root -r -M -z --no-commit-id "$@" "$sha"; fi; }; `
}

// ReadCommit reads a commit's message, committer and files.
func ReadCommit(host shell.Host, dir, hash string) (Detail, error) {
	if !HashOK(hash) {
		return Detail{}, ErrInput
	}
	out, err := shell.Text(host, commitPrelude(dir, hash)+
		`printf 'commit\n%s\n%s\n' "$(g rev-parse "$sha^{commit}")" "$parent"; `+
		`printf '\036body\n'; g show -s --format='%an%x1f%ae%x1f%at%x1f%cn%x1f%ce%x1f%ct%x1f%B' "$sha" 2>/dev/null; `+
		`printf '\036numstat\n'; tree --numstat 2>/dev/null | head -c `+strconv.Itoa(maxList)+`; `+
		`printf '\036names\n'; tree --name-status 2>/dev/null | head -c `+strconv.Itoa(maxList)+`; `+
		`printf '\036end\n'`, readFor)
	if err != nil {
		return Detail{}, err
	}
	parts := sectionsOf(out)
	header := strings.Split(strings.TrimRight(parts.head, "\n"), "\n")
	if header[0] != "commit" || len(header) < 2 {
		return Detail{}, ErrNoCommit
	}
	detail := Detail{Hash: header[1], Files: []File{}}
	if len(header) > 2 {
		detail.Parent = header[2]
	}
	if body, ok := parts.get("body"); ok {
		fields := strings.SplitN(body.body, script.US, 7)
		if len(fields) == 7 {
			detail.Author, detail.Email, detail.Committer, detail.CEmail = fields[0], fields[1], fields[3], fields[4]
			detail.Time, _ = strconv.ParseInt(fields[2], 10, 64)
			detail.CTime, _ = strconv.ParseInt(fields[5], 10, 64)
			detail.Message = strings.TrimRight(fields[6], "\n")
		}
	}
	numstat, _ := parts.get("numstat")
	names, _ := parts.get("names")
	files := joinFiles(names.body, numstat.body)
	if len(files) > MaxFiles {
		detail.More = len(files) - MaxFiles
		files = files[:MaxFiles]
	}
	detail.Files = files
	return detail, nil
}

// joinFiles puts git's --name-status -z and --numstat -z together by path: a rename is "R100\0old\0new" in the first,
// "a\td\t\0old\0new" in the second.
func joinFiles(names, numstat string) []File {
	counts := map[string]File{}
	fields := strings.Split(numstat, "\x00")
	for i := 0; i < len(fields); i++ {
		parts := strings.SplitN(fields[i], "\t", 3)
		if len(parts) != 3 {
			continue
		}
		file := File{Binary: parts[0] == "-"}
		file.Add, _ = strconv.Atoi(parts[0])
		file.Del, _ = strconv.Atoi(parts[1])
		path := parts[2]
		if path == "" && i+2 < len(fields) {
			path = fields[i+2]
			i += 2
		}
		counts[path] = file
	}
	var files []File
	fields = strings.Split(names, "\x00")
	for i := 0; i+1 < len(fields); i++ {
		status := fields[i]
		if status == "" {
			continue
		}
		file := File{Status: status[:1]}
		if (file.Status == "R" || file.Status == "C") && i+2 < len(fields) {
			file.From, file.Path = fields[i+1], fields[i+2]
			i += 2
		} else {
			file.Path = fields[i+1]
			i++
		}
		count := counts[file.Path]
		file.Add, file.Del, file.Binary = count.Add, count.Del, count.Binary
		files = append(files, file)
	}
	if files == nil {
		files = []File{}
	}
	return files
}

// ReadPatch is one file's patch in a commit against its first parent, git's own diff with rename detection; from is the
// path it had before a rename, so git sees both sides of it.
func ReadPatch(host shell.Host, dir, hash, path, from string) (string, error) {
	if !HashOK(hash) || !PathOK(path) || (from != "" && !PathOK(from)) {
		return "", ErrInput
	}
	paths := script.Quote(path)
	if from != "" && from != path {
		paths = script.Quote(from) + " " + paths
	}
	out, err := shell.Text(host, commitPrelude(dir, hash)+`printf 'patch\n'; `+
		`if [ -n "$parent" ]; then g diff --no-ext-diff --no-textconv -M "$parent" "$sha" -- `+paths+`; `+
		`else g show --format= --no-ext-diff --no-textconv -M "$sha" -- `+paths+`; fi 2>/dev/null | head -c `+strconv.Itoa(MaxPatch), readFor)
	if err != nil {
		return "", err
	}
	word, patch, _ := strings.Cut(out, "\n")
	if word != "patch" {
		return "", ErrNoCommit
	}
	return patch, nil
}
