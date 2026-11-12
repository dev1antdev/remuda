package review

import (
	"path"
	"sort"
	"strings"
	"unicode"
)

// Profile is what the badge sweep read of a folder's uncommitted work, for the review queue's risk: its size, the
// source files changed with no test changed beside them, and the paths that deserve a second look.
type Profile struct {
	Files         int         `json:"files"`
	Added         int         `json:"added"`     // lines of the tracked files, as git diff --numstat HEAD counts them
	Removed       int         `json:"removed"`   //
	NewFiles      int         `json:"new_files"` // untracked files git does not ignore
	NewBytes      int64       `json:"new_bytes"` // and their size: a file not added yet has no lines git counts
	Untested      []string    `json:"untested,omitempty"`
	UntestedCount int         `json:"untested_count,omitempty"` // every source file changed without a test, Untested naming the first MaxNamed
	Sensitive     []Sensitive `json:"sensitive,omitempty"`
}

// Sensitive is a kind of path worth a second look and the changed files of that kind, the first MaxNamed of them.
type Sensitive struct {
	Kind  string   `json:"kind"`
	Paths []string `json:"paths"`
	Count int      `json:"count"`
}

// The kinds of a sensitive path.
const (
	KindMigration = "migration" // a database migration or schema
	KindLockfile  = "lockfile"  // the dependencies pinned
	KindCI        = "ci"        // what the pipeline runs
	KindAuth      = "auth"      // sign-in, permissions, secrets, crypto
)

// MaxNamed is how many paths a reason names; the rest are counted.
const MaxNamed = 3

// bytesPerLine turns the bytes of files git counts no lines for yet into lines, for the size: a line of code is
// about forty bytes.
const bytesPerLine = 40

// profileOf weighs a folder's uncommitted files — every path changed against HEAD or untracked, gone for a deleted
// one — with the lines git counted and the untracked files' size.
func profileOf(paths []string, gone map[string]bool, added, removed, newFiles int, newBytes int64) Profile {
	profile := Profile{Files: len(paths), Added: added, Removed: removed, NewFiles: newFiles, NewBytes: newBytes}
	sorted := append([]string(nil), paths...)
	sort.Strings(sorted)
	var tests []string
	for _, p := range sorted {
		if isTest(p) {
			tests = append(tests, p)
		}
	}
	sensitive := map[string]*Sensitive{}
	var order []string
	for _, p := range sorted {
		if kind := sensitiveKind(p); kind != "" {
			entry, ok := sensitive[kind]
			if !ok {
				entry = &Sensitive{Kind: kind}
				sensitive[kind] = entry
				order = append(order, kind)
			}
			entry.Count++
			if len(entry.Paths) < MaxNamed {
				entry.Paths = append(entry.Paths, p)
			}
		}
		if !gone[p] && isSource(p) && !isTest(p) && !testedBy(p, tests) {
			profile.UntestedCount++
			if len(profile.Untested) < MaxNamed {
				profile.Untested = append(profile.Untested, p)
			}
		}
	}
	for _, kind := range order {
		profile.Sensitive = append(profile.Sensitive, *sensitive[kind])
	}
	return profile
}

// Lines is the change's size in lines: what git counted, and the untracked files' bytes read as lines.
func (p Profile) Lines() int {
	return p.Added + p.Removed + int(p.NewBytes/bytesPerLine)
}

var sourceExtensions = map[string]bool{
	".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true, ".cjs": true, ".py": true, ".rb": true,
	".rs": true, ".java": true, ".kt": true, ".swift": true, ".c": true, ".cc": true, ".cpp": true, ".h": true, ".hpp": true,
	".cs": true, ".php": true, ".scala": true, ".ex": true, ".exs": true, ".dart": true, ".vue": true, ".svelte": true,
	".m": true, ".mm": true, ".sh": true, ".lua": true, ".zig": true,
}

// isSource says whether a path is code a test could cover, not a document, a config or an asset.
func isSource(p string) bool {
	return sourceExtensions[strings.ToLower(path.Ext(p))]
}

var testFolders = map[string]bool{"test": true, "tests": true, "__tests__": true, "spec": true, "specs": true, "testing": true, "e2e": true, "testdata": true}

// isTest says whether a path is a test by Go's, JavaScript's, Python's, Ruby's or the JVM's habits, or sits in a
// test folder.
func isTest(p string) bool {
	lower := strings.ToLower(p)
	base := path.Base(lower)
	stem := strings.TrimSuffix(base, path.Ext(base))
	switch {
	case strings.HasSuffix(stem, "_test"), strings.HasSuffix(stem, "_spec"), strings.HasPrefix(base, "test_"),
		strings.Contains(base, ".test."), strings.Contains(base, ".spec."),
		strings.HasSuffix(path.Base(p), "Test"+path.Ext(p)), strings.HasSuffix(path.Base(p), "Tests"+path.Ext(p)):
		return true
	}
	for _, folder := range strings.Split(path.Dir(lower), "/") {
		if testFolders[folder] {
			return true
		}
	}
	return false
}

// testedBy says whether one of the tests changed sits beside the source file — in its folder — or carries its name:
// auth.ts and auth.test.ts anywhere, foo.py and test_foo.py.
func testedBy(source string, tests []string) bool {
	dir := path.Dir(source)
	stem := strings.ToLower(strings.TrimSuffix(path.Base(source), path.Ext(source)))
	for _, test := range tests {
		if path.Dir(test) == dir {
			return true
		}
		if stem != "" && strings.Contains(strings.ToLower(path.Base(test)), stem) {
			return true
		}
	}
	return false
}

var lockfiles = map[string]bool{
	"package-lock.json": true, "npm-shrinkwrap.json": true, "yarn.lock": true, "pnpm-lock.yaml": true, "bun.lockb": true, "bun.lock": true,
	"go.sum": true, "cargo.lock": true, "gemfile.lock": true, "poetry.lock": true, "pipfile.lock": true, "uv.lock": true,
	"composer.lock": true, "package.resolved": true, "flake.lock": true, "mix.lock": true, "pubspec.lock": true,
	"packages.lock.json": true, "gradle.lockfile": true, "podfile.lock": true,
}

var ciFiles = map[string]bool{
	".gitlab-ci.yml": true, "jenkinsfile": true, ".travis.yml": true, "azure-pipelines.yml": true, "bitbucket-pipelines.yml": true,
	".drone.yml": true, "cloudbuild.yaml": true, "cloudbuild.yml": true,
}

var ciFolders = []string{".github/workflows/", ".github/actions/", ".circleci/", ".buildkite/", ".woodpecker/", ".gitlab/ci/"}

var migrationFolders = map[string]bool{"migrations": true, "migration": true, "migrate": true, "alembic": true, "schema-migrations": true}

var schemaFiles = map[string]bool{"schema.prisma": true, "schema.rb": true, "structure.sql": true, "schema.sql": true}

// authWords are the words of a path that say it is about who may do what: sign-in, permissions, secrets, crypto.
var authWords = map[string]bool{
	"auth": true, "authn": true, "authz": true, "oauth": true, "oauth2": true, "login": true, "logout": true, "signin": true,
	"signup": true, "password": true, "passwords": true, "passwd": true, "permission": true, "permissions": true, "acl": true,
	"rbac": true, "jwt": true, "csrf": true, "credential": true, "credentials": true, "sso": true, "saml": true, "oidc": true,
	"crypto": true, "secret": true, "secrets": true, "authentication": true, "authorization": true,
}

// sensitiveKind is the kind of a path worth a second look — a migration, a lockfile, CI config, an auth-looking
// one — or "" for any other.
func sensitiveKind(p string) string {
	lower := strings.ToLower(p)
	base := path.Base(lower)
	switch {
	case lockfiles[base]:
		return KindLockfile
	case ciFiles[base] || hasAnyPrefix(lower, ciFolders):
		return KindCI
	case schemaFiles[base]:
		return KindMigration
	}
	for _, folder := range strings.Split(path.Dir(lower), "/") {
		if migrationFolders[folder] {
			return KindMigration
		}
	}
	for _, word := range pathWords(p) {
		if authWords[word] {
			return KindAuth
		}
	}
	return ""
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) || strings.Contains(s, "/"+prefix) {
			return true
		}
	}
	return false
}

// pathWords cuts a path into its lowercase words at slashes, dots, dashes, underscores and case changes:
// "internal/oauthClient/signIn.go" is internal, oauth, client, sign, in, go — and "signIn" read whole is signin too.
func pathWords(p string) []string {
	var words []string
	for _, part := range strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '.' || r == '-' || r == '_' || r == ' ' }) {
		words = append(words, strings.ToLower(part))
		var word []rune
		flush := func() {
			if len(word) > 0 {
				words = append(words, strings.ToLower(string(word)))
				word = word[:0]
			}
		}
		runes := []rune(part)
		for i, r := range runes {
			if i > 0 && unicode.IsUpper(r) && !unicode.IsUpper(runes[i-1]) {
				flush()
			}
			word = append(word, r)
		}
		flush()
	}
	return words
}
