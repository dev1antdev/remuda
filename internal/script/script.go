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
