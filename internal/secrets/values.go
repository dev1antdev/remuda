package secrets

import (
	"math"
	"strings"
)

// The bounds of a value worth a look when only its name says it is a secret.
const (
	MinValue = 16
	MaxValue = 1024
	maxName  = 64
)

// strongWords end the name of a secret: "dbPassword", "GITHUB_TOKEN", "client_secret".
var strongWords = []string{"password", "passwd", "passphrase", "secret", "token", "credential", "credentials"}

// keyWords are the names a key goes by; a bare "key" is too often a sort key, a cache key or a React list's.
var keyWords = []string{"apikey", "accesskey", "secretkey", "privatekey", "clientkey", "authkey", "masterkey", "encryptionkey",
	"signingkey", "servicekey", "appkey", "licensekey", "accountkey", "sessionkey"}

// placeholderMarkers are the words of a value nobody would deploy: a template's, a fixture's, a redaction's.
var placeholderMarkers = []string{"xxxx", "****", "....", "changeme", "change_me", "change-me", "changeit", "replaceme", "replace_me",
	"replace-me", "placeholder", "example", "dummy", "sample", "your", "insert_", "insert-", "redacted", "notreal", "not_real",
	"not-real", "fake", "mock", "test", "<", ">", "{{", "${"}

// assigned is the value after a name such as API_KEY, password or clientSecret and an = or a colon, quoted or bare,
// when it is long and random enough to be a secret.
func assigned(line string, start, end int) (hit, bool) {
	if !secretName(line[start:end]) {
		return hit{}, false
	}
	i := end
	if i < len(line) && isQuote(line[i]) { // "password": as JSON writes it
		i++
	}
	if i < len(line) && line[i] == ']' { // ENV["TOKEN"] =
		i++
	}
	i = skipBlanks(line, i)
	operator := operatorAt(line, i)
	if operator == 0 {
		return hit{}, false
	}
	i = skipBlanks(line, i+operator)
	if i >= len(line) {
		return hit{}, false
	}
	valueStart, valueEnd := i, i
	if quote := line[i]; isQuote(quote) {
		valueStart = i + 1
		valueEnd = valueStart
		for valueEnd < len(line) && line[valueEnd] != quote && valueEnd-valueStart <= MaxValue {
			if line[valueEnd] == '\\' {
				valueEnd++
			}
			valueEnd++
		}
		valueEnd = min(valueEnd, len(line))
	} else {
		for valueEnd < len(line) && isBareByte(line[valueEnd]) && valueEnd-valueStart <= MaxValue {
			valueEnd++
		}
		if valueEnd < len(line) && line[valueEnd] == '(' { // a call, not a value
			return hit{}, false
		}
	}
	if _, ok := shaped(line, valueStart, wordEnd(line, valueStart)); ok || !looksSecret(line[valueStart:valueEnd]) {
		return hit{}, false // a token of a known shape is said by its own rule, when the walk reaches it
	}
	return hit{start: valueStart, end: valueEnd, rule: RuleAssigned}, true
}

// secretName says whether a name is a secret's by its last word, or its last two words for a key.
func secretName(name string) bool {
	if len(name) > maxName {
		return false
	}
	words := nameWords(name)
	if len(words) == 0 {
		return false
	}
	for _, word := range words {
		if word == "public" { // publicKey, publicKeyToken: meant to be seen
			return false
		}
	}
	last := words[len(words)-1]
	lastTwo := last
	if len(words) > 1 {
		lastTwo = words[len(words)-2] + last
	}
	for _, word := range strongWords {
		if strings.HasSuffix(last, word) {
			return true
		}
	}
	for _, word := range keyWords {
		if strings.HasSuffix(lastTwo, word) {
			return true
		}
	}
	return false
}

// nameWords is a name cut at its separators and its changes of case, lowercased: "AWS_SECRET_ACCESS_KEY" and
// "awsSecretAccessKey" both read aws secret access key, "APIKey" api key.
func nameWords(name string) []string {
	var words []string
	var word []byte
	flush := func() {
		if len(word) > 0 {
			words = append(words, string(word))
			word = word[:0]
		}
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == '_' || c == '-' || c == '.' {
			flush()
			continue
		}
		upper := c >= 'A' && c <= 'Z'
		if upper && i > 0 {
			before := name[i-1]
			nextLower := i+1 < len(name) && name[i+1] >= 'a' && name[i+1] <= 'z'
			if before >= 'a' && before <= 'z' || before >= 'A' && before <= 'Z' && nextLower {
				flush()
			}
		}
		word = append(word, lower(c))
	}
	flush()
	return words
}

// looksSecret says whether a value a name calls secret is one: long, letters and digits with no blank, random by its
// entropy, and none of a placeholder, a reference to something else or a constant's name.
func looksSecret(value string) bool {
	if len(value) < MinValue || len(value) > MaxValue {
		return false
	}
	var letter, digit bool
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c <= ' ' || c >= 0x7f {
			return false
		}
		letter = letter || isLetter(c)
		digit = digit || isDigit(c)
	}
	if !letter || !digit || placeholder(value) || wordy(value) || reference(value) || constantName(value) {
		return false
	}
	threshold := 3.5
	if every(value, isHex) {
		threshold = 3.0 // sixteen symbols at most: a random hex string reads lower than base64
	}
	return entropy(value) >= threshold
}

// placeholder says whether a value is one nobody would deploy: a marker word, one character over and over, or a run
// of six in a row such as 123456 or abcdef.
func placeholder(value string) bool {
	folded := strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + 'a' - 'A'
		}
		return r
	}, value)
	for _, marker := range placeholderMarkers {
		if strings.Contains(folded, marker) {
			return true
		}
	}
	return oneCharacter(value) || inARow(value, 6)
}

// selfNamed are the words a value written by hand for a test spells out, "s3cr3t-token-value"; a random key holds none.
var selfNamed = []string{"secret", "token", "password", "passwd", "value"}

// wordy says whether a value only its name calls secret names itself as one: a fixture's, not a key's.
func wordy(value string) bool {
	folded := strings.ToLower(value)
	for _, word := range selfNamed {
		if strings.Contains(folded, word) {
			return true
		}
	}
	return false
}

// oneCharacter says whether a value is the same byte throughout, its separators aside.
func oneCharacter(value string) bool {
	var first byte
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c == '-' || c == '_' || c == '.' {
			continue
		}
		if first == 0 {
			first = c
		} else if c != first {
			return false
		}
	}
	return true
}

// inARow says whether a value holds a run of n bytes counting up or down by one.
func inARow(value string, n int) bool {
	up, down := 1, 1
	for i := 1; i < len(value); i++ {
		step := int(value[i]) - int(value[i-1])
		up, down = runOn(up, step == 1), runOn(down, step == -1)
		if up >= n || down >= n {
			return true
		}
	}
	return false
}

func runOn(run int, goesOn bool) int {
	if goesOn {
		return run + 1
	}
	return 1
}

// reference says whether a value points elsewhere rather than being a secret: a variable, a template, a path, an
// address, a dotted name in the code.
func reference(value string) bool {
	if strings.ContainsAny(value[:1], "$%@&*!/~#") || strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") {
		return true
	}
	if strings.Contains(value, "://") || strings.Contains(value, "<%") {
		return true
	}
	if scheme := strings.IndexByte(value, ':'); scheme > 0 && scheme <= 10 && isLetter(value[0]) && every(value[:scheme], isAlnum) {
		return true // urn:uuid:…, arn:aws:…, sha256:…: a name or a digest, not a credential
	}
	if !strings.Contains(value, ".") {
		return false
	}
	for _, part := range strings.Split(value, ".") {
		if part == "" || isDigit(part[0]) || !every(part, isAlnumUnderscore) {
			return false
		}
	}
	return true
}

// constantName says whether a value is a SCREAMING_SNAKE name: the name of another setting, not its value.
func constantName(value string) bool {
	return strings.Contains(value, "_") && every(value, func(c byte) bool { return c >= 'A' && c <= 'Z' || isDigit(c) || c == '_' })
}

// entropy is the value's Shannon entropy in bits per byte.
func entropy(value string) float64 {
	var counts [256]int
	for i := 0; i < len(value); i++ {
		counts[value[i]]++
	}
	total := float64(len(value))
	var bits float64
	for _, count := range counts {
		if count > 0 {
			share := float64(count) / total
			bits -= share * math.Log2(share)
		}
	}
	return bits
}

// operatorAt is the length of the assignment at i: = (not ==), :=, =>, or a colon (not ::); 0 for none.
func operatorAt(line string, i int) int {
	if i >= len(line) {
		return 0
	}
	next := byte(0)
	if i+1 < len(line) {
		next = line[i+1]
	}
	switch {
	case line[i] == ':' && next == '=', line[i] == '=' && next == '>':
		return 2
	case line[i] == '=' && next != '=' && next != '~':
		return 1
	case line[i] == ':' && next != ':':
		return 1
	}
	return 0
}

func skipBlanks(line string, i int) int {
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return i
}

// wordEnd is where the run of word bytes from i ends.
func wordEnd(line string, i int) int {
	for i < len(line) && isWord(line[i]) {
		i++
	}
	return i
}

func firstPrefix(word string, prefixes []string) string {
	for _, prefix := range prefixes {
		if strings.HasPrefix(word, prefix) {
			return prefix
		}
	}
	return ""
}

func every(s string, ok func(byte) bool) bool {
	for i := 0; i < len(s); i++ {
		if !ok(s[i]) {
			return false
		}
	}
	return true
}

func lower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

func isLetter(c byte) bool          { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isDigit(c byte) bool           { return c >= '0' && c <= '9' }
func isAlnum(c byte) bool           { return isLetter(c) || isDigit(c) }
func isAlnumUnderscore(c byte) bool { return isAlnum(c) || c == '_' }
func isUpperAlnum(c byte) bool      { return c >= 'A' && c <= 'Z' || isDigit(c) }
func isHex(c byte) bool             { return isDigit(c) || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' }
func isWord(c byte) bool            { return isAlnum(c) || c == '_' || c == '-' }
func isQuote(c byte) bool           { return c == '"' || c == '\'' || c == '`' }

// isCredentialByte is a byte of a header's credential: base64, base64url and the dots of a JWT.
func isCredentialByte(c byte) bool {
	return isWord(c) || c == '.' || c == '~' || c == '+' || c == '/' || c == '='
}

// isBareByte is a byte of an unquoted value: anything but a blank, a quote, a bracket, or the = and : that start
// another assignment (so no value read overlaps the next one's).
func isBareByte(c byte) bool {
	return c > ' ' && c < 0x7f && !strings.ContainsRune(",;(){}[]<>\"'`=:&|", rune(c))
}
