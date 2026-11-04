package icons

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// The icon dialog's Automatic tab: the pictures a project already keeps as its face — favicons, logos, its app icons —
// read on the folder's own machine, and its owner's avatar from its origin remote (GitHub's fetched here, GitLab's
// through glab on the machine). Asked for when the person opens the tab; nothing is kept until they pick one.

// MaxCandidates is how many of the project's own pictures are offered.
const MaxCandidates = 8

// GitHubBase is where a GitHub owner's avatar is read, a variable so tests can serve it.
var GitHubBase = "https://github.com"

// pictureDirs are the folders a project keeps its face in, the root first.
var pictureDirs = []string{".", "public", "static", "assets", "app", "src", "web/public", "docs"}

// pictureNames are the names a face goes by there, the favicon first.
var pictureNames = []string{
	"favicon.svg", "favicon.png", "favicon.ico", "apple-touch-icon.png",
	"logo.svg", "logo.png", "logo.webp", "logo.jpg", "logo.jpeg", "logo.ico",
	"icon.svg", "icon.png", "icon.webp", "icon.ico",
}

// Candidate is a picture that could be the folder's icon, as a data URL, with where it came from.
type Candidate struct {
	Data string `json:"data"`
	From string `json:"from"` // the file's path in the folder, or the owner the avatar is of
	Kind string `json:"kind"` // file | github | gitlab
}

// Found is what Candidates answers: the pictures, where it looked, and why an avatar is missing.
type Found struct {
	Candidates []Candidate `json:"candidates"`
	Looked     []string    `json:"looked"`
	Notes      []string    `json:"notes,omitempty"`
}

// Candidates finds the pictures that could be the folder's icon: the files the project keeps, then its owner's avatar.
func Candidates(ctx context.Context, host shell.Host, client *http.Client, folder string) (Found, error) {
	found := Found{Candidates: []Candidate{}, Looked: looked()}
	out, err := shell.Text(host, filesScript(folder), 30*time.Second)
	if err != nil {
		return found, fmt.Errorf("the folder could not be read on %s: %w", host.Label(), err)
	}
	sections := script.Sections(out)
	remote := ""
	if len(sections) > 0 {
		remote = strings.TrimSpace(sections[0])
	}
	seen := map[[32]byte]bool{}
	add := func(data []byte, from, kind string) bool {
		contentType := kindOf(data, from)
		if contentType == "" {
			return false
		}
		dataURL := DataURL(contentType, data)
		if _, _, err := ParseImage(dataURL); err != nil {
			return false
		}
		sum := sha256.Sum256(data)
		if seen[sum] {
			return true
		}
		seen[sum] = true
		found.Candidates = append(found.Candidates, Candidate{Data: dataURL, From: from, Kind: kind})
		return true
	}
	if len(sections) > 1 {
		for _, line := range script.Lines(sections[1]) {
			fields := script.Fields(line)
			if len(fields) != 2 || len(found.Candidates) >= MaxCandidates {
				continue
			}
			if data, err := base64.StdEncoding.DecodeString(fields[1]); err == nil {
				add(data, fields[0], "file")
			}
		}
	}
	server, path, ok := originOf(remote)
	switch {
	case remote == "":
		found.Notes = append(found.Notes, "no origin remote, so no owner's avatar")
	case !ok:
		found.Notes = append(found.Notes, "the origin remote names no host and owner")
	case isGitHub(server):
		owner := strings.SplitN(path, "/", 2)[0]
		data, err := gitHubAvatar(ctx, client, owner)
		if err != nil || !add(data, "github.com/"+owner, "github") {
			found.Notes = append(found.Notes, "GitHub has no avatar for "+owner)
		}
	default:
		found.Notes = append(found.Notes, gitLabAvatars(host, server, path, add)...)
	}
	return found, nil
}

// looked names the places the files script searches, for "none found".
func looked() []string {
	out := make([]string, 0, len(pictureDirs)+1)
	for _, dir := range pictureDirs {
		if dir == "." {
			out = append(out, "./")
			continue
		}
		out = append(out, dir+"/")
	}
	return append(out, "app icons (AppIcon.appiconset, ic_launcher)")
}

// filesScript prints the origin remote, then a line per picture: its path in the folder and its bytes in base64 —
// the named files in pictureDirs first, then the two largest app icons that fit; links and files over MaxImage skipped.
func filesScript(folder string) string {
	var names []string
	for _, dir := range pictureDirs {
		for _, name := range pictureNames {
			names = append(names, script.Quote(dir+"/"+name))
		}
	}
	limit := strconv.Itoa(MaxImage)
	return script.PATH + "; " + script.InDir(folder) +
		`git remote get-url origin 2>/dev/null; printf '\036\n'; ` +
		`n=0; emit() { [ "$n" -lt ` + strconv.Itoa(MaxCandidates*2) + ` ] || return 0; [ -f "$1" ] && [ ! -L "$1" ] || return 0; ` +
		`size=$(wc -c < "$1" | tr -d ' '); [ "$size" -gt 0 ] && [ "$size" -le ` + limit + ` ] || return 0; ` +
		`printf '%s\037' "${1#./}"; base64 < "$1" | tr -d '\n'; printf '\n'; n=$((n+1)); }; ` +
		`for f in ` + strings.Join(names, " ") + `; do emit "$f"; done; ` +
		`apps=$(find . -maxdepth 7 \( -name node_modules -o -name .git -o -name Pods -o -name build -o -name dist -o -name vendor \) -prune -o ` +
		`\( -path '*AppIcon.appiconset/*.png' -o -name 'ic_launcher*.png' -o -name 'ic_launcher*.webp' \) -type f -print 2>/dev/null | head -60 | ` +
		`while IFS= read -r f; do printf '%s %s\n' "$(wc -c < "$f" | tr -d ' ')" "$f"; done | sort -rn | ` +
		`awk -v max=` + limit + ` '$1 <= max { sub(/^[0-9]+ /, ""); print }' | head -2); ` +
		"while IFS= read -r f; do [ -n \"$f\" ] && emit \"$f\"; done <<EOF\n$apps\nEOF\nexit 0\n" // an empty last line leaves the loop at 1
}

// originOf reads a remote's host and path (owner/…/repo, no .git): scp-like git@host:path, or an ssh/http(s) URL.
func originOf(remote string) (host, path string, ok bool) {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return "", "", false
	}
	if strings.Contains(remote, "://") {
		u, err := url.Parse(remote)
		if err != nil || u.Hostname() == "" {
			return "", "", false
		}
		host, path = u.Hostname(), u.Path
	} else {
		at := strings.LastIndex(remote, "@")
		hostPart, pathPart, found := strings.Cut(remote[at+1:], ":")
		if !found {
			return "", "", false
		}
		host, path = hostPart, pathPart
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	if host == "" || !strings.Contains(path, "/") {
		return "", "", false
	}
	return strings.ToLower(host), path, true
}

func isGitHub(host string) bool { return host == "github.com" || host == "www.github.com" }

// gitHubAvatar reads the owner's avatar from GitHub, 128 pixels.
func gitHubAvatar(ctx context.Context, client *http.Client, owner string) ([]byte, error) {
	if owner == "" {
		return nil, fmt.Errorf("no owner")
	}
	data, _, err := fetch(ctx, client, GitHubBase+"/"+url.PathEscape(owner)+".png?size=128", MaxImage)
	return data, err
}

// gitLabAvatars asks glab on the folder's machine for the project's avatar, then its group's, answering why none came.
func gitLabAvatars(host shell.Host, server, path string, add func(data []byte, from, kind string) bool) []string {
	group := path[:strings.LastIndex(path, "/")]
	out, err := shell.Text(host, gitLabScript(server, path, group), 30*time.Second)
	if err != nil {
		return []string{"glab could not be asked on " + host.Label()}
	}
	if strings.TrimSpace(out) == "no-glab" {
		return []string{"no glab on " + host.Label() + " to read the GitLab avatar"}
	}
	for _, line := range script.Lines(out) {
		fields := script.Fields(line)
		if len(fields) != 2 || fields[1] == "" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(fields[1])
		if err != nil {
			continue
		}
		from := server + "/" + path
		if fields[0] == "group" {
			from = server + "/" + group
		}
		if add(data, from, "gitlab") {
			return nil
		}
	}
	return []string{"GitLab has no avatar for " + path + " or its group"}
}

// gitLabScript prints the project's and the group's avatar in base64, each on a line after its name; "no-glab" without glab.
func gitLabScript(server, path, group string) string {
	limit := strconv.Itoa(MaxImage + 1)
	avatar := func(name, what string) string {
		return `printf '` + name + `\037'; glab api --hostname ` + script.Quote(server) + ` ` + script.Quote(what+"/avatar") +
			` 2>/dev/null | head -c ` + limit + ` | base64 | tr -d '\n'; printf '\n'; `
	}
	return script.PATH + "; command -v glab >/dev/null 2>&1 || { printf 'no-glab\\n'; exit 0; }; " +
		avatar("project", "projects/"+url.PathEscape(path)) + avatar("group", "groups/"+url.PathEscape(group))
}
