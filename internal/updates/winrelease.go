package updates

import (
	"strings"

	"github.com/dev1antdev/remuda/internal/script"
)

// A Windows machine's remuda is ~/.local/bin/remuda.exe, run by Git Bash there: no service to restart, every bridge to
// herdr a fresh remuda.exe, so the next request runs the new one. Windows renames a running .exe but writes over
// none, so the one in place steps aside first, under a name of its own, and is cleared once nothing runs it.

// windowsInstall puts ~/.local/bin/remuda.new.exe in place of remuda.exe once it has shown it runs at all.
const windowsInstall = `b="$HOME/.local/bin"; chmod +x "$b/remuda.new.exe" && "$b/remuda.new.exe" -version && ` +
	`{ [ ! -f "$b/remuda.exe" ] || mv -f "$b/remuda.exe" "$b/remuda.exe.old.$$"; } && mv "$b/remuda.new.exe" "$b/remuda.exe" && ` +
	`{ rm -f "$b"/remuda.exe.old.* 2>/dev/null; true; } && ` +
	`echo "no service on Windows: the next connection to herdr runs the new remuda.exe"`

// windowsAsset says the asset is a Windows binary.
func windowsAsset(asset string) bool { return strings.HasSuffix(asset, ".exe") }

// windowsReleaseScript is ReleaseScript for a Windows machine: the same download and checksum, then windowsInstall.
func windowsReleaseScript(url, sums, asset string) string {
	var b strings.Builder
	b.WriteString("set -e; " + script.PATH + "\n")
	b.WriteString(`mkdir -p "$HOME/.local/bin"` + "\n")
	b.WriteString(downloadScript(url, sums, asset, "remuda.new.exe"))
	b.WriteString(windowsInstall)
	return b.String()
}

// downloadScript fetches the asset into ~/.local/bin/<into> and, when the release has its checksums, checks it.
func downloadScript(url, sums, asset, into string) string {
	file := `"$HOME/.local/bin/` + into + `"`
	var b strings.Builder
	b.WriteString(`echo downloading ` + script.Quote(url) + "\n")
	b.WriteString(`curl -fsSL --retry 3 -o ` + file + ` ` + script.Quote(url) + "\n")
	if sums == "" {
		b.WriteString(`echo "(this release carries no checksums.txt; the download is not verified)"` + "\n")
		return b.String()
	}
	b.WriteString(`if curl -fsSL --retry 3 -o "$HOME/.local/bin/remuda.sums" ` + script.Quote(sums) + `; then` + "\n")
	b.WriteString(`  want=$(awk -v n=` + script.Quote(asset) + ` '{ f=$NF; sub(/^\*/, "", f); if (f == n) print $1 }' "$HOME/.local/bin/remuda.sums" | head -1)` + "\n")
	b.WriteString(`  have=$( (shasum -a 256 ` + file + ` 2>/dev/null || sha256sum ` + file + `) | cut -d' ' -f1)` + "\n")
	b.WriteString(`  rm -f "$HOME/.local/bin/remuda.sums"` + "\n")
	b.WriteString(`  if [ -z "$want" ] || [ "$want" != "$have" ]; then echo "!! checksum mismatch: the download is not what the release says (want ${want:-?}, got $have)"; rm -f ` + file + `; exit 1; fi` + "\n")
	b.WriteString(`  echo "checksum ok"` + "\n")
	b.WriteString(`else echo "!! could not fetch the release's checksums.txt"; rm -f ` + file + `; exit 1; fi` + "\n")
	return b.String()
}
