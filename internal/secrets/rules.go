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
	cases    bool // the body holds a capital and a small letter: a random one, never a name in the code
}

var shapes = []shape{
	{rule: RuleHuggingFace, prefixes: []string{"hf_"}, exact: 37, body: isAlnum, cases: true},
	{rule: RuleGroq, prefixes: []string{"gsk_"}, exact: 56, body: isAlnum, digit: true},
	{rule: RuleXAI, prefixes: []string{"xai-"}, exact: 84, body: isAlnum, digit: true},
	{rule: RuleOpenRouter, prefixes: []string{"sk-or-v1-"}, exact: 73, body: isHex},
	{rule: RuleDigitalOcean, prefixes: []string{"dop_v1_", "doo_v1_", "dor_v1_"}, exact: 71, body: isHex},
	{rule: RuleDocker, prefixes: []string{"dckr_pat_", "dckr_oat_"}, min: 24, body: isWord},
	{rule: RulePyPI, prefixes: []string{"pypi-AgEIcHlwaS5vcmc"}, min: 50, body: isWord},
	{rule: RuleGoogleOAuth, prefixes: []string{"GOCSPX-"}, exact: 35, body: isWord},
	{rule: RuleShopify, prefixes: []string{"shpat_", "shpca_", "shppa_", "shpss_"}, exact: 38, body: isHex},
	{rule: RuleLinear, prefixes: []string{"lin_api_"}, exact: 48, body: isAlnum},
	{rule: RuleAtlassian, prefixes: []string{"ATATT3"}, min: 100, body: isWord},
	{rule: RuleAge, prefixes: []string{"AGE-SECRET-KEY-1"}, exact: 74, body: isUpperAlnum},
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
		if !ok {
			found, ok = urlPassword(line, i, end)
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
		if kind.exact > 0 && len(word) != kind.exact || len(body) < kind.min || !every(body, kind.body) || kind.digit && !strings.ContainsAny(body, "0123456789") ||
			kind.cases && !(strings.ContainsAny(body, upperLetters) && strings.ContainsAny(body, lowerLetters)) || placeholder(body) {
			return hit{}, false
		}
		return hit{start: start, end: end, rule: kind.rule, keep: len(prefix)}, true
	}
	// OpenAI's keys of every kind but the newest prefixes, sk-None- among them, carry the marker
	if strings.HasPrefix(word, "sk-") && len(word) >= 40 && strings.Contains(word, openAIMarker) && !placeholder(word[3:]) {
		return hit{start: start, end: end, rule: RuleOpenAI, keep: 3}, true
	}
	if found, ok := jwt(line, start, end); ok {
		return found, true
	}
	if found, ok := dotted(line, start, end); ok {
		return found, true
	}
	return telegram(line, start, end)
}

const (
	upperLetters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	lowerLetters = "abcdefghijklmnopqrstuvwxyz"
)

// dotted is a token whose parts dots join: SendGrid's SG.<22>.<43>, and Vault's hvs., hvb. or hvr. and 90 bytes or more.
func dotted(line string, start, end int) (hit, bool) {
	switch line[start:end] {
	case "SG":
		first, ok := dottedPart(line, end, 22, 22)
		if !ok {
			return hit{}, false
		}
		second, ok := dottedPart(line, first, 43, 43)
		if !ok || placeholder(line[end+1:second]) {
			return hit{}, false
		}
		return hit{start: start, end: second, rule: RuleSendGrid, keep: 3}, true
	case "hvs", "hvb", "hvr":
		token, ok := dottedPart(line, end, 90, MaxValue)
		if !ok || placeholder(line[end+1:token]) {
			return hit{}, false
		}
		return hit{start: start, end: token, rule: RuleVault, keep: 4}, true
	}
	return hit{}, false
}

// dottedPart is where the word after the dot at i ends, when there is a dot and the word runs between least and most bytes.
func dottedPart(line string, i, least, most int) (int, bool) {
	if i >= len(line) || line[i] != '.' {
		return 0, false
	}
	end := wordEnd(line, i+1)
	length := end - (i + 1)
	return end, length >= least && length <= most
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
		body := keyBodyEnd(line, end)
		hits = append(hits, hit{start: start, end: body, rule: RulePrivateKey, keep: end - start, noTail: true})
		from = max(from, body)
	}
}

// keyBodyEnd is where the body of a key whose header ends at i stops on the header's own line — a key kept on one
// line, in a JSON string or an environment variable, with its line breaks escaped or turned into blanks — up to its
// END marker; the blanks after it are not the key's.
func keyBodyEnd(line string, i int) int {
	end := i
	for end < len(line) && (isAlnum(line[end]) || strings.IndexByte("+/=\\ \t", line[end]) >= 0) {
		end++
	}
	for end > i && (line[end-1] == ' ' || line[end-1] == '\t') {
		end--
	}
	return end
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
