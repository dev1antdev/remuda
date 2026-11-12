package herdr

// An output watch's regex is herdr's to read, in Rust's dialect with Unicode on: \d, \w, \s and \b go by Unicode,
// \< and \> are a word's start and end, a class may hold a class and combine them with && -- ~~, and there is no
// octal escape or backreference. herdr names no watch when a row matches, so remuda reads the row again with each of
// the pane's patterns in Go: CompileRegex reads a pattern the way herdr does, as far as Go can, and refuses what Go
// would read otherwise or herdr not at all.

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Unicode's digits, word characters and white space, as herdr's \d, \w and \s take them.
const (
	unicodeDigit = `\p{Nd}`
	unicodeWord  = `\p{L}\p{Nl}\p{M}\p{Nd}\p{Pc}\x{200C}\x{200D}`
	unicodeSpace = `\t\n\v\f\r \x{85}\p{Z}`
)

// Regexp is a herdr regex read in Go. Go's \b knows only ASCII words, so on a row outside ASCII a pattern holding \b
// or \B is read with them left out: a row herdr matched is not missed, at the price of one it would not have.
type Regexp struct {
	exact *regexp.Regexp
	loose *regexp.Regexp // the pattern without \b and \B; nil when it holds neither
}

// CompileRegex reads a herdr regex in Go, or says why herdr would refuse it or read it otherwise.
func CompileRegex(pattern string) (*Regexp, error) {
	if _, err := regexp.Compile(pattern); err != nil {
		return nil, fmt.Errorf("the regex does not read: %v", strings.TrimPrefix(err.Error(), "error parsing regexp: "))
	}
	exact, loose, err := translate(pattern)
	if err != nil {
		return nil, err
	}
	re := &Regexp{}
	if re.exact, err = regexp.Compile(exact); err != nil {
		return nil, fmt.Errorf("the regex is too big to read: %v", strings.TrimPrefix(err.Error(), "error parsing regexp: "))
	}
	if loose != exact {
		if re.loose, err = regexp.Compile(loose); err != nil {
			return nil, fmt.Errorf("the regex is too big to read: %v", strings.TrimPrefix(err.Error(), "error parsing regexp: "))
		}
	}
	return re, nil
}

// MatchString says whether the row matches, as herdr would.
func (re *Regexp) MatchString(row string) bool {
	if re.exact.MatchString(row) {
		return true
	}
	return re.loose != nil && !ascii(row) && re.loose.MatchString(row)
}

func ascii(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// translate is the pattern with herdr's Unicode classes spelled out for Go, and the same without \b and \B; an error
// for what herdr refuses or reads otherwise than Go.
func translate(p string) (exact, loose string, err error) {
	var ex, lo strings.Builder
	both := func(s string) {
		ex.WriteString(s)
		lo.WriteString(s)
	}
	inClass, classStart := false, 0
	for i := 0; i < len(p); {
		c := p[i]
		if c == '\\' && i+1 < len(p) {
			n := p[i+1]
			switch {
			case n >= '0' && n <= '9':
				return "", "", fmt.Errorf(`herdr reads no backreference or octal escape (\%c): write a character as \x41`, n)
			case n == 'Q':
				return "", "", errors.New(`\Q…\E is not in herdr's regex: escape the characters one by one`)
			case !inClass && (n == '<' || n == '>'):
				return "", "", fmt.Errorf(`herdr reads \%c as a word's start or end: write %c alone for the character, \b for a word's edge`, n, n)
			case !inClass && (n == 'b' || n == 'B'):
				if i+2 < len(p) && p[i+2] == '{' {
					return "", "", errors.New(`herdr reads \b{…} as a word's start or end: write \b`)
				}
				ex.WriteString(p[i : i+2])
				lo.WriteString(`(?:)`)
				i += 2
				continue
			case n == 'd' || n == 'w' || n == 's' || n == 'D' || n == 'W' || n == 'S':
				both(unicodeClass(n, inClass))
				i += 2
				if inClass && i+1 < len(p) && p[i] == '-' && p[i+1] != ']' {
					return "", "", errors.New(`herdr reads no range that starts at a class, as [\d-z]: write the class and the range apart`)
				}
				continue
			case n == 'p' || n == 'P' || n == 'x':
				end := i + 3
				if i+2 < len(p) && p[i+2] == '{' {
					if close := strings.IndexByte(p[i+2:], '}'); close >= 0 {
						end = i + 2 + close + 1
					}
				} else if n == 'x' {
					end = i + 4
				}
				both(p[i:min(end, len(p))])
				i = min(end, len(p))
				continue
			}
			_, size := utf8.DecodeRuneInString(p[i+1:])
			both(p[i : i+1+size])
			i += 1 + size
			continue
		}
		if inClass {
			switch {
			case c == '[' && i+1 < len(p) && p[i+1] == ':':
				end := strings.Index(p[i+2:], ":]")
				if end < 0 {
					return "", "", errors.New(`herdr reads a [ inside […] as a class of its own: escape it as \[`)
				}
				both(p[i : i+2+end+2])
				i += 2 + end + 2
				continue
			case c == '[':
				return "", "", errors.New(`herdr reads a [ inside […] as a class of its own: escape it as \[`)
			case c == ']' && i != classStart:
				inClass = false
			case (c == '&' || c == '-' || c == '~') && i+1 < len(p) && p[i+1] == c:
				return "", "", fmt.Errorf(`herdr reads %c%c inside […] as combining two classes: escape them as \%c\%c`, c, c, c, c)
			}
			both(p[i : i+1])
			i++
			continue
		}
		switch {
		case c == '[':
			inClass = true
			j := i + 1
			if j < len(p) && p[j] == '^' {
				j++
			}
			classStart = j
			both(p[i:j])
			i = j
			continue
		case c == '{' && !countAt(p[i:]):
			return "", "", errors.New(`herdr reads a { only as a count, as x{2} or x{2,5}: write \{ for the character, x{0,n} for at most n`)
		}
		both(p[i : i+1])
		i++
	}
	return ex.String(), lo.String(), nil
}

// countAt says whether s starts with a count as herdr reads one: {n}, {n,} or {n,m}, digits and nothing else.
func countAt(s string) bool {
	i, digits := 1, 0
	for ; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		digits++
	}
	if digits == 0 {
		return false
	}
	if i < len(s) && s[i] == ',' {
		for i++; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		}
	}
	return i < len(s) && s[i] == '}'
}

// unicodeClass is \d, \w or \s and their negations as Unicode classes; inside a class, the negations of \w and \s
// stay Go's own, since no class of Go's names what is outside them.
func unicodeClass(name byte, inClass bool) string {
	set := map[byte]string{'d': unicodeDigit, 'w': unicodeWord, 's': unicodeSpace}
	switch name {
	case 'd', 'w', 's':
		if inClass {
			return set[name]
		}
		return "[" + set[name] + "]"
	case 'D':
		return `\P{Nd}`
	}
	lower := name + ('a' - 'A')
	if inClass {
		return `\` + string(name)
	}
	return "[^" + set[lower] + "]"
}
