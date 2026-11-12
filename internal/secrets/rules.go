package secrets

import (
	"encoding/base64"
	"strings"
	"unicode/utf8"
)

// shape is a kind of token known by its prefix: the bytes its body may hold and how long it runs.
type shape struct {
	rule     string
	prefixes []string // the first that matches is the part the mask shows
	min      int      // the body's least length past the prefix
	exact    int      // the whole token's length, for a kind that has one
	body     func(byte) bool
	digit    bool // the body holds a digit
}

var shapes = []shape{
	{rule: RuleAWS, prefixes: []string{"AKIA", "ASIA", "ABIA", "ACCA"}, exact: 20, body: isUpperAlnum},
	{rule: RuleGitHubFine, prefixes: []string{"github_pat_"}, min: 22, body: isAlnumUnderscore},
	{rule: RuleGitHub, prefixes: []string{"ghp_", "gho_", "ghu_", "ghs_", "ghr_"}, min: 36, body: isAlnum},
	{rule: RuleGitLab, prefixes: []string{"glpat-", "gldt-", "glrt-", "glptt-", "glsoat-", "glcbt-", "glft-", "glimt-", "glagent-", "gloas-"}, min: 20, body: isWord},
	{rule: RuleSlack, prefixes: []string{"xoxb-", "xoxp-", "xoxa-", "xoxr-", "xoxs-", "xoxe-", "xapp-"}, min: 20, body: isWord, digit: true},
	{rule: RuleStripe, prefixes: []string{"sk_live_", "rk_live_"}, min: 20, body: isAlnum},
	{rule: RuleAnthropic, prefixes: []string{"sk-ant-api03-", "sk-ant-admin01-", "sk-ant-"}, min: 32, body: isWord},
	{rule: RuleOpenAI, prefixes: []string{"sk-proj-", "sk-svcacct-", "sk-admin-"}, min: 32, body: isWord},
	{rule: RuleGoogle, prefixes: []string{"AIza"}, exact: 39, body: isWord},
	{rule: RuleNPM, prefixes: []string{"npm_"}, min: 36, body: isAlnum},
}

// openAIMarker is base64 of "OpenAI", in the middle of every key the older format made.
const openAIMarker = "T3BlbkFJ"

// tokens walks the line's words once: each is tried as a token of a known shape, the value of an Authorization
// header, or the name of a secret with its value after it; a found secret is stepped over whole.
func tokens(line string, hits []hit) []hit {
	for i := 0; i < len(line); {
		if !isWord(line[i]) {
			i++
			continue
		}
		end := wordEnd(line, i)
		found, ok := shaped(line, i, end)
		if !ok {
			found, ok = authHeader(line, i, end)
		}
		if !ok {
			found, ok = assigned(line, i, end)
		}
		if ok {
			hits = append(hits, found)
			i = max(found.end, end)
			continue
		}
		i = end
	}
	return hits
}

// shaped is the token that starts at the word [start, end) when it is one of a known kind; a JWT and a Telegram bot's
// token run past the word, over the dots and the colon between their parts.
func shaped(line string, start, end int) (hit, bool) {
	for start < end && line[start] == '-' { // a flag's dashes, never a token's
		start++
	}
	word := line[start:end]
	for _, kind := range shapes {
		prefix := firstPrefix(word, kind.prefixes)
		if prefix == "" {
			continue
		}
		body := word[len(prefix):]
		if kind.exact > 0 && len(word) != kind.exact || len(body) < kind.min || !every(body, kind.body) || kind.digit && !strings.ContainsAny(body, "0123456789") || placeholder(body) {
			return hit{}, false
		}
		return hit{start: start, end: end, rule: kind.rule, keep: len(prefix)}, true
	}
	if strings.HasPrefix(word, "sk-") && len(word) >= 40 && strings.Contains(word, openAIMarker) && every(word[3:], isAlnum) && !placeholder(word[3:]) {
		return hit{start: start, end: end, rule: RuleOpenAI, keep: 3}, true
	}
	if found, ok := jwt(line, start, end); ok {
		return found, true
	}
	return telegram(line, start, end)
}

// jwt is a JSON Web Token: a header and a payload that are base64 of JSON objects, and a signature.
func jwt(line string, start, end int) (hit, bool) {
	if end-start < 10 || !strings.HasPrefix(line[start:end], "eyJ") || end >= len(line) || line[end] != '.' {
		return hit{}, false
	}
	payload := end + 1
	payloadEnd := wordEnd(line, payload)
	if payloadEnd-payload < 10 || !strings.HasPrefix(line[payload:payloadEnd], "eyJ") || payloadEnd >= len(line) || line[payloadEnd] != '.' {
		return hit{}, false
	}
	signatureEnd := wordEnd(line, payloadEnd+1)
	if signatureEnd-(payloadEnd+1) < 16 || placeholder(line[payloadEnd+1:signatureEnd]) {
		return hit{}, false
	}
	return hit{start: start, end: signatureEnd, rule: RuleJWT, keep: 3}, true
}

// telegram is a bot's token: its numeric id, a colon, and 35 bytes of secret.
func telegram(line string, start, end int) (hit, bool) {
	digits := end - start
	if digits < 8 || digits > 10 || !every(line[start:end], isDigit) || end >= len(line) || line[end] != ':' || start > 0 && line[start-1] == ':' {
		return hit{}, false
	}
	secretEnd := wordEnd(line, end+1)
	secret := line[end+1 : secretEnd]
	if len(secret) != 35 || !strings.ContainsAny(secret, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") || placeholder(secret) {
		return hit{}, false
	}
	return hit{start: start, end: secretEnd, rule: RuleTelegram, keep: digits + 1}, true
}

// privateKeys are the headers of PEM and PGP private keys; the header alone gives the key away.
func privateKeys(line string, hits []hit) []hit {
	const marker, begin = "PRIVATE KEY", "-----BEGIN"
	for from := 0; ; {
		at := strings.Index(line[from:], marker)
		if at < 0 {
			return hits
		}
		at += from
		from = at + len(marker)
		window := max(0, at-40)
		start := strings.LastIndex(line[window:at], begin)
		if start < 0 {
			continue
		}
		start += window
		if !every(line[start+len(begin):at], func(c byte) bool { return c == ' ' || c >= 'A' && c <= 'Z' }) {
			continue
		}
		end := from
		if strings.HasPrefix(line[end:], " BLOCK") {
			end += len(" BLOCK")
		}
		if !strings.HasPrefix(line[end:], "-----") {
			continue
		}
		end += len("-----")
		hits = append(hits, hit{start: start, end: end, rule: RulePrivateKey, keep: end - start})
	}
}

// slackWebhooks are Slack's incoming webhooks, whose path is the secret.
func slackWebhooks(line string, hits []hit) []hit {
	const host = "hooks.slack.com/"
	for from := 0; ; {
		at := strings.Index(line[from:], host)
		if at < 0 {
			return hits
		}
		at += from
		from = at + len(host)
		kind := firstPrefix(line[from:], []string{"services/", "workflows/", "triggers/"})
		if kind == "" {
			continue
		}
		pathStart := from + len(kind)
		end := pathStart
		for end < len(line) && end-pathStart < MaxValue && (isWord(line[end]) || line[end] == '/') {
			end++
		}
		if path := line[pathStart:end]; len(path) < 20 || !strings.Contains(path, "/") || placeholder(path) {
			continue
		}
		hits = append(hits, hit{start: at, end: end, rule: RuleSlackWebhook, keep: pathStart - at})
		from = end
	}
}

// authHeader is the credential after "Bearer" or "Basic", as an Authorization header carries it: a Basic one only when
// it is base64 of a user and a password that are no placeholder, since "basic" is also a word.
func authHeader(line string, start, end int) (hit, bool) {
	word := line[start:end]
	basic := word == "Basic"
	if !basic && !strings.EqualFold(word, "bearer") {
		return hit{}, false
	}
	valueStart := end
	for valueStart < len(line) && line[valueStart] == ' ' {
		valueStart++
	}
	if valueStart == end {
		return hit{}, false
	}
	valueEnd := valueStart
	for valueEnd < len(line) && valueEnd-valueStart <= MaxValue && isCredentialByte(line[valueEnd]) {
		valueEnd++
	}
	value := line[valueStart:valueEnd]
	if basic {
		if !basicCredential(value) {
			return hit{}, false
		}
	} else if _, ok := shaped(line, valueStart, wordEnd(line, valueStart)); ok || !looksSecret(value) {
		return hit{}, false // a token of a known shape is said by its own rule, when the walk reaches it
	}
	return hit{start: valueStart, end: valueEnd, rule: RuleAuthHeader}, true
}

// basicCredential says whether a Basic header's value is base64 of "user:password" with a password worth hiding.
func basicCredential(value string) bool {
	if len(value) < 8 || len(value) > MaxValue {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		if decoded, err = base64.RawStdEncoding.DecodeString(value); err != nil {
			return false
		}
	}
	user, password, ok := strings.Cut(string(decoded), ":")
	return ok && user != "" && len(password) >= 8 && utf8.ValidString(password) && !placeholder(password) && !wordy(password)
}
