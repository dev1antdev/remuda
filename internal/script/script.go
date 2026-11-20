// Package script builds the POSIX scripts remuda runs on a machine, here or
// over ssh: the quoting, the preludes every script shares, the stat that
// works on both a Mac and Linux, and the separators an answer is split by.
package script

import (
	"strings"

	"github.com/dev1antdev/remuda/internal/rpath"
)

// Quote wraps s as one word for a POSIX shell.
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// PATH comes first in every script that calls a tool: a non-interactive shell over ssh starts without the package
// managers' bin dirs, or Kimi Code's own. On a Windows machine, in Git Bash's spelling of the profile's folders, herdr's
// stable alias (the release on the account's PATH is the one the ssh connection began with, an update moves it) and
// npm's global bin; elsewhere those two are folders that are not there.
const PATH = `PATH="$HOME/.local/bin:$HOME/.kimi-code/bin:/opt/homebrew/bin:/usr/local/bin:$HOME/AppData/Local/Programs/Herdr/bin:$PATH:$HOME/AppData/Roaming/npm"; export PATH`

// InDir enters the folder, or ends the script quietly: a folder that is gone answers nothing, not an error.
func InDir(dir string) string {
	return "cd " + Quote(dir) + " 2>/dev/null || exit 0; "
}

// InRepo is InDir with the repository's root in $root; a folder outside git answers nothing.
func InRepo(dir string) string {
	return InDir(dir) + "root=$(git rev-parse --show-toplevel 2>/dev/null) || exit 0; "
}

// Stat prints the file's fields in the GNU form, falling back to BSD stat.
// file is a shell word already: a quoted path, or a variable in double quotes.
func Stat(file, gnuFormat, bsdFormat string) string {
	return "stat -c " + Quote(gnuFormat) + " " + file + " 2>/dev/null || stat -f " + Quote(bsdFormat) + " " + file
}

// ReadGit is git for a read of a checkout, which an agent may be committing in at that moment: no optional lock and no
// refresh of a touched file's stat by diff, so the index is never written and git add never meets an index.lock of ours,
// and no fsmonitor hook of the person's runs. A touched file then counts in `diff --name-only`; `--numstat` leaves it out.
const ReadGit = `git --no-optional-locks -c core.fsmonitor=false -c diff.autoRefreshIndex=false`

// ReadGitAll makes every later `git` of a script ReadGit, for a script that reads through many of them.
const ReadGitAll = `git() { command ` + ReadGit + ` "$@"; }` + "\n"

// PatchForm is git's patch in one shape whatever the person's or the repository's settings make of it, for whatever
// reads its lines: no colour, no external driver or textconv, a moved submodule as a plain hunk, paths under a/ and b/.
const PatchForm = `--no-color --no-ext-diff --no-textconv --submodule=short --src-prefix=a/ --dst-prefix=b/`

// Health asks the remuda at url how it is, past ~/.curlrc and any proxy, every door and peer being on the loopback; a
// live one's answer holds "ok":true. url is a shell word already, without its path.
func Health(url string) string {
	return "curl -q -s -m 2 --noproxy 127.0.0.1 " + url + "/api/health 2>/dev/null"
}

// WindowsDefaultShell is the one line, in an administrator PowerShell on a Windows PC, that makes Git Bash the shell its
// OpenSSH server runs a command in, so every script here runs there as it does on a Mac or Linux.
const WindowsDefaultShell = `New-ItemProperty -Path "HKLM:\SOFTWARE\OpenSSH" -Name DefaultShell -Value "C:\Program Files\Git\bin\bash.exe" -PropertyType String -Force`

// StateDir sets the shell variable to the dir remuda keeps on the machine, as state.Dir reckons it there: under
// Application Support on a Mac, %APPDATA%\remuda on Windows (in Git Bash's spelling), $XDG_CONFIG_HOME/remuda elsewhere.
func StateDir(variable string) string {
	return `case "$(uname -s)" in
  Darwin) ` + variable + `="$HOME/Library/Application Support/remuda" ;;
  MINGW*|MSYS*|CYGWIN*) ` + variable + `="$(cygpath -u "$APPDATA")/remuda" ;;
  *) ` + variable + `="${XDG_CONFIG_HOME:-$HOME/.config}/remuda" ;;
esac`
}

// Remuda is the quoted path of a machine's own remuda, and the guard that it is this very version:
// an older one would not know a subcommand, or would start a server instead.
// A Windows machine's home (C:\Users\me) makes it ~/.local/bin/remuda.exe, as Git Bash there reads it.
func Remuda(home, version string) (bin, guard string) {
	bin = Quote(RemudaPath(home))
	guard = "[ -x " + bin + " ] && [ \"$(" + bin + " -version 2>/dev/null)\" = " + Quote("remuda "+version) + " ]"
	return bin, guard
}

// CoreBin sets the shell variable to remuda-core's binary where `remuda core install` puts it, "" when it is not there:
// ~/.local/bin/remuda-core, or remuda-core.exe on Windows, counted only with the ConPTY it ships beside it.
func CoreBin(variable string) string {
	return variable + `="$HOME/.local/bin/remuda-core"; [ -x "$` + variable + `" ] || { ` + variable + `="$` + variable + `.exe"; [ -x "$` + variable + `" ] && [ -f "$HOME/.local/bin/conpty/conpty.dll" ] && [ -f "$HOME/.local/bin/conpty/x64/OpenConsole.exe" ] || ` + variable + `=; }`
}

// RemudaPath is where remuda lives on a machine with this home: ~/.local/bin/remuda, or remuda.exe there on Windows,
// spelled with forward slashes (C:/Users/me/.local/bin/remuda.exe), which Git Bash and Windows both read.
func RemudaPath(home string) string {
	style := rpath.For(home)
	if style.Windows {
		return style.Slash(style.Join(home, ".local", "bin", "remuda.exe"))
	}
	return home + "/.local/bin/remuda"
}

// The separators a script's answer is cut by, bytes no path or line of text holds.
// A script prints them as octal escapes: printf '\036\n' ends a section.
const (
	GS = "\x1d" // between records
	RS = "\x1e" // between sections
	US = "\x1f" // between fields
)

// Sections splits an answer at the lines holding RS alone.
func Sections(out string) []string {
	return strings.Split(out, RS+"\n")
}

// Lines is the non-empty lines of a section.
func Lines(section string) []string {
	var out []string
	for _, line := range strings.Split(section, "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// Fields splits a line at US.
func Fields(line string) []string {
	return strings.Split(line, US)
}
