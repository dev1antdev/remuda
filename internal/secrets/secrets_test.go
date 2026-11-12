package secrets

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

// The secrets these tests find are made when they run, from a fixed seed: none sits in the source as it would in a
// leak, so neither this scanner nor gitleaks flags the file.
const (
	upperAlnum = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	alnum      = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	word       = alnum + "_-"
	hex        = "0123456789abcdef"
)

// random is n bytes of the alphabet that look as random as a key, the same on every run.
func random(seed uint32, alphabet string, n int) string {
	state := seed*2654435761 + 12345
	out := make([]byte, n)
	for i := range out {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		out[i] = alphabet[state%uint32(len(alphabet))]
	}
	return string(out)
}

func jwtOf(seed uint32) string {
	encode := base64.RawURLEncoding.EncodeToString
	return encode([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." + encode([]byte(`{"sub":"`+random(seed, alnum, 12)+`","iat":1700000000}`)) + "." + random(seed+1, word, 43)
}

func TestEveryRuleFindsItsShape(t *testing.T) {
	basic := base64.StdEncoding.EncodeToString([]byte("deploy:" + random(31, alnum, 18)))
	privateHeader := "-----BEGIN " + "RSA PRIVATE" + " KEY-----"
	cases := []struct {
		rule, before, secret, after, prefix string // the prefix is what the mask shows of the secret
	}{
		{RulePrivateKey, `key := "`, privateHeader, `\n` + random(1, alnum, 40) + `"`, privateHeader},
		{RulePrivateKey, "", "-----BEGIN " + "OPENSSH PRIVATE" + " KEY-----", "", "-----BEGIN " + "OPENSSH PRIVATE" + " KEY-----"},
		{RulePrivateKey, "", "-----BEGIN " + "PGP PRIVATE" + " KEY BLOCK-----", "", "-----BEGIN " + "PGP PRIVATE" + " KEY BLOCK-----"},
		{RuleAWS, "aws_access_key_id = ", "AKIA" + random(2, upperAlnum, 16), "", "AKIA"},
		{RuleGitHub, `"Bash(curl -H 'Authorization: token `, "ghp_" + random(3, alnum, 36), `' https://api.github.com)"`, "ghp_"},
		{RuleGitHub, "GH=", "gho_" + random(4, alnum, 36), "", "gho_"},
		{RuleGitHubFine, "export GH_TOKEN=", "github_pat_" + random(5, alnum, 22) + "_" + random(6, alnum, 59), "", "github_pat_"},
		{RuleGitLab, "token: ", "glpat-" + random(7, word, 20), "", "glpat-"},
		{RuleSlack, "SLACK=", "xoxb-" + random(70, "0123456789", 13) + "-" + random(71, "0123456789", 13) + "-" + random(8, alnum, 24), "", "xoxb-"},
		{RuleSlackWebhook, `url = "https://`, "hooks.slack.com/services/" + "T" + random(9, upperAlnum, 8) + "/B" + random(10, upperAlnum, 8) + "/" + random(11, alnum, 24), `"`, "hooks.slack.com/services/"},
		{RuleStripe, "stripe.api_key = '", "sk_live_" + random(12, alnum, 24), "'", "sk_live_"},
		{RuleAnthropic, "ANTHROPIC_API_KEY=", "sk-ant-api03-" + random(13, word, 95), "", "sk-ant-api03-"},
		{RuleOpenAI, "OPENAI_API_KEY=", "sk-proj-" + random(14, word, 120), "", "sk-proj-"},
		{RuleOpenAI, "openai.api_key = '", "sk-" + random(15, alnum, 20) + openAIMarker + random(16, alnum, 20), "'", "sk-"},
		{RuleGoogle, `const maps = "`, "AIza" + random(17, word, 35), `";`, "AIza"},
		{RuleTelegram, "BOT=", "7123456789:" + "AA" + random(18, word, 33), "", "7123456789:"},
		{RuleNPM, "//registry.npmjs.org/:_authToken=", "npm_" + random(19, alnum, 36), "", "npm_"},
		{RuleJWT, `curl -H "Authorization: Bearer `, jwtOf(20), `"`, "eyJ"},
		{RuleAuthHeader, `fetch(url, { headers: { Authorization: "Bearer `, random(21, alnum, 40), `" } })`, ""},
		{RuleAuthHeader, `curl -H "Authorization: Basic `, basic, `" https://ci.internal`, ""},
		{RuleAssigned, "DATABASE_PASSWORD=", random(22, alnum, 24), "", ""},
		{RuleAssigned, `  "clientSecret": "`, random(23, alnum+"+/", 40), `",`, ""},
		{RuleAssigned, "apiKey: '", random(24, hex, 32), "'", ""},
		{RuleAssigned, `os.environ["SERVICE_TOKEN"] = "`, random(25, alnum, 30), `"`, ""},
		{RuleAssigned, `"Bash(SENTRY_AUTH_TOKEN=`, random(26, hex, 64), ` npx sentry-cli releases new)"`, ""},
		{RuleAssigned, "secret_key := `", random(27, alnum, 32), "`", ""},
		{RuleAssigned, "--api-key=", random(28, alnum, 32), "", ""},
	}
	for _, c := range cases {
		line := c.before + c.secret + c.after
		found := Scan("app/config.txt", line)
		if len(found) != 1 || found[0].Rule != c.rule || found[0].Line != 1 || found[0].File != "app/config.txt" {
			t.Errorf("%s: %+v in %q", c.rule, found, line)
			continue
		}
		if c.prefix == c.secret {
			if !strings.Contains(found[0].Excerpt, c.secret) {
				t.Errorf("%s: the excerpt %q shows the header, which holds nothing of the key", c.rule, found[0].Excerpt)
			}
			continue
		}
		body := c.secret[len(c.prefix):]
		if !strings.Contains(found[0].Excerpt, c.prefix+bullets) || strings.Contains(found[0].Excerpt, body[:8]) || strings.Contains(found[0].Excerpt, body[len(body)/2-4:len(body)/2+4]) {
			t.Errorf("%s: the excerpt %q shows more than the prefix %q of the secret", c.rule, found[0].Excerpt, c.prefix)
		}
	}
}

func TestPlaceholdersAndCodeStayQuiet(t *testing.T) {
	for _, line := range []string{
		"GITHUB_TOKEN=" + "ghp_" + strings.Repeat("x", 36),
		"aws_access_key_id = " + "AKIA" + "IOSFODNN7" + "EXAMPLE",
		"API_KEY=<your-api-key>",
		"API_KEY=your_" + "api_key_here_123456",
		"password: change" + "me1234567890",
		"TOKEN=" + strings.Repeat("a", 24),
		"secret = '" + "0123456789abcdef" + "0123'",
		`api_key = os.getenv("API_KEY_2024_ROTATED")`,
		"token = process.env.GITHUB_TOKEN_2",
		`if password == "` + random(40, alnum, 24) + `" {`,
		`<assemblyIdentity publicKeyToken="` + random(41, hex, 16) + `"/>`,
		`<li key="item-` + random(42, alnum, 20) + `">`,
		"  token: ${{ secrets.GITHUB_TOKEN }}",
		"TOKEN={{ vault_token_2024 }}",
		"Basic auth is what the proxy asks for, " + random(43, alnum, 20),
		"const token = \"s3cr3t-" + "token-value\"",
		"max_tokens: 4096",
		"TOKEN_URL=https://example.com/oauth/token/" + random(44, alnum, 20),
		"SECRET_NAME=" + "DEPLOY_SIGNING_KEY_V2",
		"AUTH_TOKEN=" + random(45, alnum, 24) + " # gitleaks:allow",
		"DB_PASSWORD=" + random(46, alnum, 24) + " # pragma: allowlist secret",
		"passwordHash: $2b$12$" + random(47, alnum, 30),
		"sessionToken: urn:uuid:" + random(48, hex, 8) + "-" + random(49, hex, 4),
		"monkey = '" + random(50, alnum, 30) + "'",
		"key: " + random(51, alnum, 30),
		`apiToken: "fake_` + random(52, alnum, 24) + `"`,
		"CLIENT_SECRET=" + random(53, "abc", 30),
		"PASSWORD=short1",
		"token := strings.Repeat(\"test-token-\", 4)",
		"Authorization: Bearer $TOKEN",
		"Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("user:password")),
		"ghp_" + random(54, alnum, 20),
		"eyJhbGciOiJub25lIn0.eyJzdWIiOiJ4In0.",
		"-----BEGIN CERTIFICATE-----",
		"-----BEGIN PUBLIC KEY-----",
		"the time was 1234567890:" + random(55, word, 35)[:10],
	} {
		if found := Scan("f", line); len(found) != 0 {
			t.Errorf("%q: %+v", line, found)
		}
	}
}

func TestTheNameDecides(t *testing.T) {
	for name, want := range map[string]bool{
		"password": true, "DB_PASSWORD": true, "dbPassword": true, "PGPASSWORD": true, "passwd": true, "passphrase": true,
		"secret": true, "client_secret": true, "webhookSecret": true, "GITHUB_TOKEN": true, "accessToken": true, "_authToken": true,
		"api_key": true, "apiKey": true, "APIKey": true, "x-api-key": true, "AWS_SECRET_ACCESS_KEY": true, "privateKey": true, "--api-key": true,
		"key": false, "cacheKey": false, "primaryKey": false, "monkey": false, "tokenizer": false, "max_tokens": false, "secretName": false,
		"passwordHash": false, "publicKeyToken": false, "public_key": false, "TOKEN_URL": false, "author": false,
		strings.Repeat("a", 70) + "token": false,
	} {
		if got := secretName(name); got != want {
			t.Errorf("secretName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestTheExcerptMasksEverySecretInIt(t *testing.T) {
	first, second := "ghp_"+random(60, alnum, 36), "sk-ant-api03-"+random(61, word, 95)
	line := strings.Repeat("padding ", 10) + `env: GH=` + first + ` CLAUDE=` + second + strings.Repeat(" tail", 30)
	found := Scan("f", line)
	if len(found) != 2 {
		t.Fatalf("%+v", found)
	}
	for _, finding := range found {
		if strings.Contains(finding.Excerpt, first[10:30]) || strings.Contains(finding.Excerpt, second[20:60]) {
			t.Errorf("an excerpt shows a secret: %q", finding.Excerpt)
		}
		if !strings.HasPrefix(finding.Excerpt, "…") || !strings.HasSuffix(finding.Excerpt, "…") || len([]rune(finding.Excerpt)) > 200 {
			t.Errorf("the excerpt is not a window of the line: %q", finding.Excerpt)
		}
	}
	if !strings.Contains(found[0].Excerpt, "ghp_"+bullets+first[len(first)-4:]) {
		t.Errorf("a long secret's last four bytes tell it apart: %q", found[0].Excerpt)
	}
	if got := mask("AKIA"+random(62, upperAlnum, 16), 4); got != "AKIA"+bullets {
		t.Errorf("a short body shows nothing past its prefix: %q", got)
	}
	tabbed := Scan("f", "\tpassword:\t"+random(63, alnum, 20)+"\x1b[0m")
	if len(tabbed) != 1 || strings.ContainsAny(tabbed[0].Excerpt, "\t\x1b") {
		t.Errorf("control characters stay out of an excerpt: %+v", tabbed)
	}
}

func TestTheCapsAndTheCommitsCopies(t *testing.T) {
	var many strings.Builder
	for i := range 30 {
		many.WriteString("TOKEN_" + string(rune('A'+i%26)) + "=" + "ghp_" + random(uint32(100+i), alnum, 36) + "\n")
	}
	collector := NewCollector()
	collector.Text("many.env", many.String())
	if len(collector.Findings()) != MaxPerFile || collector.More() != 30-MaxPerFile {
		t.Errorf("a file says at most %d: %d, %d more", MaxPerFile, len(collector.Findings()), collector.More())
	}
	oneLine := strings.Repeat("ghp_"+random(200, alnum, 36)+" ", 3) + strings.Repeat("npm_"+random(201, alnum, 36)+" ", 3)
	if found := Scan("line", oneLine); len(found) != MaxPerLine {
		t.Errorf("a line says at most %d: %d", MaxPerLine, len(found))
	}
	secret := "AWS_SECRET_ACCESS_KEY=" + random(202, alnum+"+/", 40)
	collector = NewCollector()
	collector.Line("deploy.sh", 3, secret, "")
	collector.Line("deploy.sh", 3, secret, "abc1234")
	collector.Line("deploy.sh", 9, "TOKEN="+"ghp_"+random(203, alnum, 36), "abc1234")
	if found := collector.Findings(); len(found) != 2 || found[0].Commit != "" || found[1].Commit != "abc1234" || found[1].Line != 9 {
		t.Errorf("a commit's copy of the work's secret is said once: %+v", found)
	}
	collector.Binary("certs/signing.p12", "")
	collector.Binary("certs/signing.p12", "abc1234")
	collector.Binary("icon.png", "")
	if found := collector.Findings(); len(found) != 3 || found[2].Rule != RuleKeyFile || found[2].Line != 0 || found[2].Excerpt != "signing.p12" {
		t.Errorf("a keystore is a finding on the whole file, once: %+v", found)
	}
}

// TestALineIsReadInTimeLinearInItsLength feeds the scanner lines built to make a scan that restarts at every position
// cost the square of their length: each must stay far under the time that would take.
func TestALineIsReadInTimeLinearInItsLength(t *testing.T) {
	const size = 2 << 20
	repeat := func(unit string) string { return strings.Repeat(unit, size/len(unit)+1)[:size] }
	lines := map[string]string{
		"jwt parts":       repeat("eyJabcdefghijk."),
		"names":           repeat("key="),
		"secret names":    repeat("token="),
		"quoted names":    repeat(`token="`),
		"json names":      repeat(`"password":"`),
		"mixed quotes":    `token="` + repeat(`token='a`),
		"private headers": repeat("-----BEGIN PRIVATE KEY"),
		"key markers":     repeat("PRIVATE KEY-----"),
		"webhooks":        repeat("hooks.slack.com/services/"),
		"bearers":         repeat("Bearer "),
		"basics":          repeat("Basic dXNlcjpwYXNz"),
		"one long word":   repeat("a"),
		"one long token":  "ghp_" + repeat("A"),
		"telegram ids":    repeat("123456789:"),
		"dashes":          repeat("-"),
		"base64":          random(300, alnum+"+/", size),
		"assignments":     repeat("API_KEY=" + random(301, alnum, 24) + ";"),
	}
	for name, line := range lines {
		start := time.Now()
		collector := NewCollector()
		collector.Line("long", 1, line, "")
		if took := time.Since(start); took > 2*time.Second {
			t.Errorf("%s: a %d MB line took %s", name, size>>20, took)
		}
	}
}
