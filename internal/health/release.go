package health

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/dev1antdev/remuda/internal/sshx"
)

// remuda's own releases on GitHub, offered to a machine whose remuda is older; nothing is fetched until Run.

const releasesRepo = "dev1antdev/remuda"

var releasesURL = "https://api.github.com/repos/" + releasesRepo + "/releases/latest"

// Release is the newest release as GitHub lists it.
type Release struct {
	Tag       string            `json:"tag"`
	Version   string            `json:"version"`
	Assets    map[string]string `json:"-"` // asset name → download URL
	Checksums string            `json:"-"` // the checksums.txt asset; "" when the release has none
}

func fetchRelease(ctx context.Context, url string) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "remuda")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Release{}, &httpError{url, res.StatusCode}
	}
	return parseRelease(res.Body)
}

// parseRelease reads GitHub's release object: the tag and the assets.
func parseRelease(r io.Reader) (Release, error) {
	var m struct {
		Tag    string `json:"tag_name"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(r).Decode(&m); err != nil {
		return Release{}, err
	}
	rel := Release{Tag: m.Tag, Version: semver.FindString(m.Tag), Assets: map[string]string{}}
	if rel.Version == "" {
		return Release{}, fmt.Errorf("release tag %q carries no version", m.Tag)
	}
	for _, a := range m.Assets {
		if a.Name == "checksums.txt" {
			rel.Checksums = a.URL
			continue
		}
		rel.Assets[a.Name] = a.URL
	}
	return rel, nil
}

// AssetName is the release asset a machine needs, remuda-<os>-<arch>; "" for a platform the releases do not cover.
func AssetName(os, arch string) string {
	o := strings.ToLower(os)
	switch {
	case strings.HasPrefix(o, "macos") || strings.HasPrefix(o, "darwin"):
		o = "darwin"
	case strings.HasPrefix(o, "linux"):
		o = "linux"
	default:
		return ""
	}
	if arch != "arm64" && arch != "amd64" {
		return ""
	}
	return "remuda-" + o + "-" + arch
}

// ReleaseUpdate offers the release to a machine running have, when it is newer and has a binary for it.
// local: the machine's remuda serves this very page, so it must restart after the job, not during it.
func ReleaseUpdate(have, os, arch string, rel Release, local bool) (Update, bool) {
	if rel.Version == "" || have == "" || !Newer(have, rel.Version) {
		return Update{}, false
	}
	asset := AssetName(os, arch)
	url := rel.Assets[asset]
	if asset == "" || url == "" || !releaseURLOK(url, rel.Tag) || (rel.Checksums != "" && !releaseURLOK(rel.Checksums, rel.Tag)) {
		return Update{}, false
	}
	note := have + " → " + rel.Tag + ", the release on github.com/" + releasesRepo + ". The machine downloads the binary itself"
	if rel.Checksums != "" {
		note += ", checks it against the release's checksums"
	} else {
		note += " (this release carries no checksums.txt, so the download is not verified)"
	}
	note += ", installs it as ~/.local/bin/remuda and restarts the service."
	if local {
		note += " This is the remuda serving this page: it restarts two seconds after the install, and the page offers a reload once the new version answers."
	}
	return Update{What: "remuda-release", Title: "Download & install remuda " + rel.Tag, Command: ReleaseScript(url, rel.Checksums, asset, local), Note: note}, true
}

// releaseURLOK accepts only a download URL as GitHub hands them out for this repository's release:
// it goes into a shell script, so anything else (a hostile mirror, a quote, a subshell) is no release.
func releaseURLOK(raw, tag string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.String() != raw {
		return false
	}
	if strings.ContainsAny(raw, "'\"\\ $`;|&<>()\n\r\t") {
		return false
	}
	return strings.HasPrefix(u.Path, "/"+releasesRepo+"/releases/download/"+tag+"/") && !strings.Contains(u.Path, "/../")
}

const pathLine = `PATH="$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:$PATH"; export PATH`

// InstallScript puts ~/.local/bin/remuda.new where the service runs remuda from, once it has shown it runs at all.
const InstallScript = `chmod +x "$HOME/.local/bin/remuda.new" && "$HOME/.local/bin/remuda.new" -version && mv "$HOME/.local/bin/remuda.new" "$HOME/.local/bin/remuda"`

// RestartScript restarts the service, launchd or systemd, and says so when there is none.
const RestartScript = `if command -v launchctl >/dev/null 2>&1; then launchctl kickstart -k "gui/$(id -u)/dev.mardeus.remuda" 2>&1 || echo "(no launchd service; start remuda there yourself)"; ` +
	`elif command -v systemctl >/dev/null 2>&1; then systemctl --user restart remuda.service 2>&1 || echo "(no systemd service; start remuda there yourself)"; fi`

// restartLater is for the machine whose remuda runs the job itself: detached
// and two seconds on, so the job ends first and the page hears it did.
const restartLater = `if launchctl print "gui/$(id -u)/dev.mardeus.remuda" >/dev/null 2>&1 || systemctl --user is-active remuda.service >/dev/null 2>&1; then ` +
	`echo "restarting remuda in 2 s…"; (sleep 2; ` + RestartScript + `) >/dev/null 2>&1 & ` +
	`else echo "no service runs remuda here: start the new binary yourself"; fi`

// ReleaseScript is what the machine runs: download, the checksum when the release has one, install, restart.
func ReleaseScript(url, sums, asset string, local bool) string {
	var b strings.Builder
	b.WriteString("set -e; " + pathLine + "\n")
	b.WriteString(`mkdir -p "$HOME/.local/bin"` + "\n")
	b.WriteString(`echo downloading ` + sshx.Quote(url) + "\n")
	b.WriteString(`curl -fsSL --retry 3 -o "$HOME/.local/bin/remuda.new" ` + sshx.Quote(url) + "\n")
	if sums != "" {
		b.WriteString(`if curl -fsSL --retry 3 -o "$HOME/.local/bin/remuda.sums" ` + sshx.Quote(sums) + `; then` + "\n")
		b.WriteString(`  want=$(awk -v n=` + sshx.Quote(asset) + ` '{ f=$NF; sub(/^\*/, "", f); if (f == n) print $1 }' "$HOME/.local/bin/remuda.sums" | head -1)` + "\n")
		b.WriteString(`  have=$( (shasum -a 256 "$HOME/.local/bin/remuda.new" 2>/dev/null || sha256sum "$HOME/.local/bin/remuda.new") | cut -d' ' -f1)` + "\n")
		b.WriteString(`  rm -f "$HOME/.local/bin/remuda.sums"` + "\n")
		b.WriteString(`  if [ -z "$want" ] || [ "$want" != "$have" ]; then echo "!! checksum mismatch: the download is not what the release says (want ${want:-?}, got $have)"; rm -f "$HOME/.local/bin/remuda.new"; exit 1; fi` + "\n")
		b.WriteString(`  echo "checksum ok"` + "\n")
		b.WriteString(`else echo "!! could not fetch the release's checksums.txt"; rm -f "$HOME/.local/bin/remuda.new"; exit 1; fi` + "\n")
	} else {
		b.WriteString(`echo "(this release carries no checksums.txt; the download is not verified)"` + "\n")
	}
	b.WriteString(InstallScript + "\n")
	if local {
		b.WriteString(restartLater)
	} else {
		b.WriteString(RestartScript)
	}
	return b.String()
}
