// Package rpath is a path on a machine of the fleet, spelled the way that machine spells it: POSIX on a Mac or Linux,
// Windows' own on a Windows PC (C:\Users\me\app, as herdr and Claude Code name a folder there). The cockpit runs on a
// Mac or Linux, so Go's own path and path/filepath read every path as POSIX; a Windows machine's paths go through here.
// A machine's home tells its spelling: a Windows machine's home is C:\Users\me.
package rpath

import (
	"path"
	"strings"
)

// Style is the way one machine spells its paths.
type Style struct {
	Windows bool
}

// POSIX is a Mac's or Linux's spelling.
var POSIX = Style{}

// For is the style of the machine whose home is home.
func For(home string) Style { return Style{Windows: IsWindows(home)} }

// IsWindows says the path is spelled as Windows spells it: a drive (C:\, C:/) or a share (\\server\share).
func IsWindows(p string) bool {
	return drive(p) || strings.HasPrefix(p, `\\`)
}

// drive says the path starts with a drive letter and its colon.
func drive(p string) bool {
	return len(p) >= 2 && p[1] == ':' && letter(p[0]) && (len(p) == 2 || p[2] == '\\' || p[2] == '/')
}

func letter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// msysDrive says the path is Git Bash's spelling of a drive, /c or /c/…
func msysDrive(p string) bool {
	return len(p) >= 2 && p[0] == '/' && letter(p[1]) && (len(p) == 2 || p[2] == '/')
}

// Clean is the path in the machine's own spelling with . and .. resolved and no separator at its end but a root's:
// on Windows backslashes, the drive letter in capitals, and Git Bash's /c/Users read as C:\Users.
func (s Style) Clean(p string) string {
	if !s.Windows {
		if p == "" {
			return ""
		}
		return path.Clean(p)
	}
	return cleanWindows(p)
}

func cleanWindows(p string) string {
	if p == "" {
		return ""
	}
	slashed := strings.ReplaceAll(p, `\`, "/")
	switch {
	case msysDrive(slashed):
		slashed = strings.ToUpper(slashed[1:2]) + ":" + slashed[2:]
	case strings.HasPrefix(slashed, "//"):
		return `\\` + strings.ReplaceAll(strings.TrimPrefix(path.Clean("/"+strings.TrimLeft(slashed, "/")), "/"), "/", `\`)
	}
	if drive(slashed) {
		rest := path.Clean("/" + slashed[2:])
		return strings.ToUpper(slashed[:1]) + ":" + strings.ReplaceAll(rest, "/", `\`)
	}
	return strings.ReplaceAll(path.Clean(slashed), "/", `\`)
}

// IsAbs says the path names its place whole: from / on POSIX, from a drive or a share on Windows (Git Bash's /c too).
func (s Style) IsAbs(p string) bool {
	if !s.Windows {
		return strings.HasPrefix(p, "/")
	}
	return IsWindows(p) || msysDrive(strings.ReplaceAll(p, `\`, "/"))
}

// Join is the elements joined by the machine's separator and cleaned.
func (s Style) Join(elem ...string) string {
	var parts []string
	for _, e := range elem {
		if e != "" {
			parts = append(parts, e)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	if !s.Windows {
		return path.Join(parts...)
	}
	return cleanWindows(strings.Join(parts, `\`))
}

// Dir is the folder holding the path; a root is its own.
func (s Style) Dir(p string) string {
	if !s.Windows {
		return path.Dir(p)
	}
	clean := cleanWindows(p)
	i := strings.LastIndex(clean, `\`)
	switch {
	case i < 0:
		return "."
	case i == 2 && drive(clean):
		return clean[:3]
	}
	return clean[:i]
}

// Base is the path's last element.
func (s Style) Base(p string) string {
	if !s.Windows {
		return path.Base(p)
	}
	clean := cleanWindows(p)
	if i := strings.LastIndex(clean, `\`); i >= 0 && i < len(clean)-1 {
		return clean[i+1:]
	}
	return clean
}

// Equal says two spellings name the same place: Windows ignores case and the separator's kind.
func (s Style) Equal(a, b string) bool {
	if !s.Windows {
		return s.Clean(a) == s.Clean(b)
	}
	return strings.EqualFold(cleanWindows(a), cleanWindows(b))
}

// Within says p is root or lies under it.
func (s Style) Within(root, p string) bool {
	_, ok := s.Rel(root, p)
	return ok
}

// Rel is p relative to root, with forward slashes, "." for root itself; false when p is not under root.
func (s Style) Rel(root, p string) (string, bool) {
	root, p = s.Clean(root), s.Clean(p)
	if root == "" || p == "" {
		return "", false
	}
	sep := "/"
	if s.Windows {
		sep = `\`
	}
	prefix := strings.TrimSuffix(root, sep) + sep
	switch {
	case s.Equal(root, p):
		return ".", true
	case len(p) > len(prefix) && s.samePrefix(p[:len(prefix)], prefix):
		return strings.ReplaceAll(p[len(prefix):], sep, "/"), true
	}
	return "", false
}

func (s Style) samePrefix(a, b string) bool {
	if s.Windows {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// Slash is the path with forward slashes, the way Git Bash and git itself also read a Windows path (C:/Users/me).
func (s Style) Slash(p string) string {
	if !s.Windows {
		return p
	}
	return strings.ReplaceAll(p, `\`, "/")
}

// Separator is the machine's own.
func (s Style) Separator() string {
	if s.Windows {
		return `\`
	}
	return "/"
}
