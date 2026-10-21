// Package script builds the POSIX scripts remuda runs on a machine, here or
// over ssh: the quoting, the preludes every script shares, the stat that
// works on both a Mac and Linux, and the separators an answer is split by.
package script

import "strings"

// Quote wraps s as one word for a POSIX shell.
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// PATH comes first in every script that calls a tool: a non-interactive shell over ssh starts without the package managers' bin dirs, or Kimi Code's own.
const PATH = `PATH="$HOME/.local/bin:$HOME/.kimi-code/bin:/opt/homebrew/bin:/usr/local/bin:$PATH"; export PATH`

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

// Remuda is the quoted path of a machine's own remuda, and the guard that it is this very version:
// an older one would not know a subcommand, or would start a server instead.
func Remuda(home, version string) (bin, guard string) {
	bin = Quote(home + "/.local/bin/remuda")
	guard = "[ -x " + bin + " ] && [ \"$(" + bin + " -version 2>/dev/null)\" = " + Quote("remuda "+version) + " ]"
	return bin, guard
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
