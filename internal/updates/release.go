package updates

import (
	"net/url"
	"strings"

	"github.com/dev1antdev/remuda/internal/health"
	"github.com/dev1antdev/remuda/internal/script"
)

// remuda's own releases on GitHub, offered to a machine whose remuda is older; nothing is fetched until Run.

// AssetName is the release asset a machine needs, remuda-<os>-<arch>, remuda-windows-<arch>.exe for a Windows one; ""
// for a platform the releases do not cover.
func AssetName(os, arch string) string {
	o, ext := strings.ToLower(os), ""
	switch {
	case strings.HasPrefix(o, "macos") || strings.HasPrefix(o, "darwin"):
		o = "darwin"
	case strings.HasPrefix(o, "linux"):
		o = "linux"
	case strings.HasPrefix(o, "windows"):
		o, ext = "windows", ".exe"
	default:
		return ""
	}
	if arch != "arm64" && arch != "amd64" {
		return ""
	}
	return "remuda-" + o + "-" + arch + ext
}

// ReleaseUpdate offers the release to a machine running have, when it is newer and has a binary for it.
// local: the machine's remuda serves this very page, so it must restart after the job, not during it.
func ReleaseUpdate(have, os, arch string, rel health.Release, local bool) (Update, bool) {
	if rel.Version == "" || have == "" || !health.Newer(have, rel.Version) {
		return Update{}, false
	}
	asset := AssetName(os, arch)
	url := rel.Assets[asset]
	if asset == "" || url == "" || !releaseURLOK(url, rel.Tag) || (rel.Checksums != "" && !releaseURLOK(rel.Checksums, rel.Tag)) {
		return Update{}, false
	}
	note := have + " → " + rel.Tag + ", the release on github.com/" + health.ReleasesRepo + ". The machine downloads the binary itself"
	if rel.Checksums != "" {
		note += ", checks it against the release's checksums"
	} else {
		note += " (this release carries no checksums.txt, so the download is not verified)"
	}
	if windowsAsset(asset) {
		note += ", and puts it in place as ~/.local/bin/remuda.exe; there is no service on Windows, the next connection runs it."
	} else {
		note += ", installs it as ~/.local/bin/remuda and restarts the service."
	}
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
	return strings.HasPrefix(u.Path, "/"+health.ReleasesRepo+"/releases/download/"+tag+"/") && !strings.Contains(u.Path, "/../")
}

// InstallScript puts ~/.local/bin/remuda.new where the service runs remuda from, once it has shown it runs at all.
const InstallScript = `chmod +x "$HOME/.local/bin/remuda.new" && "$HOME/.local/bin/remuda.new" -version && mv "$HOME/.local/bin/remuda.new" "$HOME/.local/bin/remuda"`

// RestartScript restarts the service, launchd or systemd, and says so when there is none — or neither manager at all,
// since the new binary then sits beside a remuda still running the old one.
const RestartScript = `if command -v launchctl >/dev/null 2>&1; then launchctl kickstart -k "gui/$(id -u)/dev.mardeus.remuda" 2>&1 || echo "(no launchd service; start remuda there yourself)"; ` +
	`elif command -v systemctl >/dev/null 2>&1; then systemctl --user restart remuda.service 2>&1 || echo "(no systemd service; start remuda there yourself)"; ` +
	`else echo "(neither launchd nor systemd here: start the new remuda yourself)"; fi`

// restartLater is for the machine whose remuda runs the job itself: detached
// and two seconds on, so the job ends first and the page hears it did.
const restartLater = `if launchctl print "gui/$(id -u)/dev.mardeus.remuda" >/dev/null 2>&1 || systemctl --user is-active remuda.service >/dev/null 2>&1; then ` +
	`echo "restarting remuda in 2 s…"; (sleep 2; ` + RestartScript + `) >/dev/null 2>&1 & ` +
	`else echo "no service runs remuda here: start the new binary yourself"; fi`

// ReleaseScript is what the machine runs: download, the checksum when the release has one, install, restart.
func ReleaseScript(url, sums, asset string, local bool) string {
	if windowsAsset(asset) {
		return windowsReleaseScript(url, sums, asset)
	}
	var b strings.Builder
	b.WriteString("set -e; " + script.PATH + "\n")
	b.WriteString(`mkdir -p "$HOME/.local/bin"` + "\n")
	b.WriteString(downloadScript(url, sums, asset, "remuda.new"))
	b.WriteString(InstallScript + "\n")
	if local {
		b.WriteString(restartLater)
	} else {
		b.WriteString(RestartScript)
	}
	return b.String()
}
