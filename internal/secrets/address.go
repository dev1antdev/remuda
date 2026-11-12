package secrets

import "strings"

// minURLPassword is the least length of a password in an address worth a look: "postgres:postgres" and its kind are
// a development database's, and a short word is no generated password.
const minURLPassword = 8

// urlPassword is the password of an address's user, scheme://user:password@host — a database's, a queue's, a cache's —
// when the scheme is the word [start, end) and the password is none of a placeholder, a reference, the user's name
// again, a word of one kind of letter, or a password for this machine's loopback.
func urlPassword(line string, start, end int) (hit, bool) {
	if !strings.HasPrefix(line[end:], "://") {
		return hit{}, false
	}
	userStart := end + len("://")
	userEnd := userStart
	for userEnd < len(line) && userEnd-userStart <= MaxValue && isUserinfoByte(line[userEnd]) && line[userEnd] != ':' {
		userEnd++
	}
	if userEnd >= len(line) || line[userEnd] != ':' {
		return hit{}, false
	}
	passwordStart := userEnd + 1
	passwordEnd := passwordStart
	for passwordEnd < len(line) && passwordEnd-passwordStart <= MaxValue && isUserinfoByte(line[passwordEnd]) {
		passwordEnd++
	}
	if passwordEnd >= len(line) || line[passwordEnd] != '@' {
		return hit{}, false
	}
	user, password := line[userStart:userEnd], line[passwordStart:passwordEnd]
	host := line[passwordEnd+1 : min(len(line), passwordEnd+1+len("localhost"))]
	if _, ok := shaped(line, passwordStart, wordEnd(line, passwordStart)); ok || !generatedPassword(user, password) || loopback(host) {
		return hit{}, false // a token of a known shape is said by its own rule, when the walk reaches it
	}
	return hit{start: passwordStart, end: passwordEnd, rule: RuleURLPassword}, true
}

// generatedPassword says whether an address's password is one somebody would keep secret: long enough, no
// placeholder or reference, not made of the user's name ("admin123" for admin), and of two kinds of bytes at least.
func generatedPassword(user, password string) bool {
	if len(password) < minURLPassword || len(password) > MaxValue || strings.ContainsAny(password[:1], "$%{<") {
		return false
	}
	if len(user) >= 3 && strings.Contains(strings.ToLower(password), strings.ToLower(user)) {
		return false
	}
	if placeholder(password) || wordy(password) {
		return false
	}
	var kinds [4]bool
	for i := 0; i < len(password); i++ {
		c := password[i]
		switch {
		case c >= 'a' && c <= 'z':
			kinds[0] = true
		case c >= 'A' && c <= 'Z':
			kinds[1] = true
		case isDigit(c):
			kinds[2] = true
		default:
			kinds[3] = true
		}
	}
	count := 0
	for _, kind := range kinds {
		if kind {
			count++
		}
	}
	return count >= 2
}

// loopbackHosts are the hosts of this very machine: a password for one is a development database's.
var loopbackHosts = []string{"localhost", "127.", "[::1]", "0.0.0.0"}

// loopback says whether the start of the host after the user's @ names this machine.
func loopback(host string) bool {
	host = strings.ToLower(host)
	for _, name := range loopbackHosts {
		if strings.HasPrefix(host, name) {
			return true
		}
	}
	return false
}

// isUserinfoByte is a byte of an address's user and password: anything printable but a blank, a quote, the @ that
// ends them and the bytes that end the address's authority.
func isUserinfoByte(c byte) bool {
	return c > ' ' && c < 0x7f && !strings.ContainsRune("@/?#\"'`<>\\", rune(c))
}
