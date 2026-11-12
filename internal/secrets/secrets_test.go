package secrets

import (
	"encoding/base64"
	"os"
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
		{RuleAssigned, "DISCORD_TOKEN=", "MT" + random(29, alnum, 24) + "." + random(30, alnum, 6) + "." + random(32, alnum, 38), "", ""},
		{RuleOpenAI, "OPENAI_API_KEY=", "sk-None-" + random(33, alnum, 20) + openAIMarker + random(34, alnum, 20), "", "sk-"},
		{RuleHuggingFace, "model = load(token='", "hf_" + random(35, alnum[:52], 34), "')", "hf_"},
		{RuleGroq, "GROQ=", "gsk_" + random(36, alnum, 52), "", "gsk_"},
		{RuleXAI, "XAI=", "xai-" + random(37, alnum, 80), "", "xai-"},
		{RuleOpenRouter, "OR=", "sk-or-v1-" + random(38, hex, 64), "", "sk-or-v1-"},
		{RuleSendGrid, "sg.setApiKey('", "SG." + random(39, word, 22) + "." + random(76, word, 43), "')", "SG."},
		{RuleDigitalOcean, "client = Client('", "dop_v1_" + random(77, hex, 64), "')", "dop_v1_"},
		{RuleDocker, "echo ", "dckr_pat_" + random(78, word, 27), " | docker login", "dckr_pat_"},
		{RulePyPI, "password = ", "pypi-AgEIcHlwaS5vcmc" + random(79, word, 60), "", "pypi-AgEIcHlwaS5vcmc"},
		{RuleGoogleOAuth, `secret('`, "GOCSPX-" + random(80, word, 28), `')`, "GOCSPX-"},
		{RuleShopify, "X-Shopify-Access-Token: ", "shpat_" + random(81, hex, 32), "", "shpat_"},
		{RuleLinear, "LINEAR=", "lin_api_" + random(82, alnum, 40), "", "lin_api_"},
		{RuleAtlassian, "JIRA=", "ATATT3" + random(83, word, 180), "", "ATATT3"},
		{RuleAge, "# ", "AGE-SECRET-KEY-1" + random(84, upperAlnum, 58), "", "AGE-SECRET-KEY-1"},
		{RuleVault, "VAULT=", "hvs." + random(85, word, 95), "", "hvs."},
		{RuleURLPassword, "DATABASE_URL=postgres://admin:", random(86, alnum, 20), "@db.prod.internal:5432/app", ""},
		{RuleURLPassword, `uri = "mongodb+srv://app:`, random(87, alnum, 18) + "!", `@cluster0.abcde.mongodb.net/db"`, ""},
		{RuleURLPassword, "REDIS_URL=redis://:", random(88, alnum, 24), "@cache:6379", ""},
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

// TestThePageNamesEveryRule: a rule the page has no words for reads as a bare "Secret value" in the dialog.
func TestThePageNamesEveryRule(t *testing.T) {
	page, err := os.ReadFile("../../web/src/features/review/secrets.ts")
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range Rules {
		if rule != RuleAssigned && !strings.Contains(string(page), `"`+rule+`": t(`) && !strings.Contains(string(page), "  "+rule+": t(") {
			t.Errorf("the page has no words for %s", rule)
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
		"DATABASE_URL=postgres://postgres:postgres@db:5432/dev",
		"DATABASE_URL=postgres://app:" + random(56, alnum, 20) + "@localhost:5432/dev",
		"DATABASE_URL=postgres://app:" + random(57, alnum, 20) + "@127.0.0.1/dev",
		"DATABASE_URL=postgres://${DB_USER}:${DB_PASSWORD}@db:5432/app",
		"DATABASE_URL=postgres://app:development@db/app",
		"REDIS_URL=redis://:changeme123@cache:6379",
		"amqp://guest:guest@rabbit:5672",
		"MONGO_URL=mongodb://admin:Admin2024@mongo:27017",
		"see https://example.com/docs:" + random(58, alnum, 20) + " and mail me@there",
		"hf_" + "downloadmodelsfromthehubwithcaches",
		"token = this.settings.apiTokenForTheService2024",
		"VAULT=hvs." + random(59, word, 20),
		"SG.abc.def",
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
	if got := mask("AKIA"+random(62, upperAlnum, 16), 4, false); got != "AKIA"+bullets {
		t.Errorf("a short body shows nothing past its prefix: %q", got)
	}
	tabbed := Scan("f", "\tpassword:\t"+random(63, alnum, 20)+"\x1b[0m")
	if len(tabbed) != 1 || strings.ContainsAny(tabbed[0].Excerpt, "\t\x1b") {
		t.Errorf("control characters stay out of an excerpt: %+v", tabbed)
	}
}

// TestAKeyOnOneLineShowsNoneOfItsBody: a service account's JSON or an environment variable keeps a private key on its
// header's line, and the forty bytes after the header held most of an EC key's secret.
func TestAKeyOnOneLineShowsNoneOfItsBody(t *testing.T) {
	header, footer := "-----BEGIN "+"EC PRIVATE"+" KEY-----", "-----END "+"EC PRIVATE"+" KEY-----"
	body := "MHcCAQEEI" + random(64, alnum+"+/", 120)
	for _, line := range []string{
		`  "private_key": "` + header + `\n` + body[:64] + `\n` + body[64:] + `\n` + footer + `\n",`,
		`PRIVATE_KEY="` + header + ` ` + body[:64] + ` ` + body[64:] + ` ` + footer + `"`,
		`PRIVATE_KEY="` + header + `\r\n` + body,
	} {
		found := Scan("service-account.json", line)
		if len(found) != 1 || found[0].Rule != RulePrivateKey {
			t.Fatalf("%q: %+v", line, found)
		}
		for at := 0; at+6 <= len(body); at++ {
			if strings.Contains(found[0].Excerpt, body[at:at+6]) {
				t.Fatalf("the excerpt %q shows the key's body %q", found[0].Excerpt, body[at:at+6])
			}
		}
		if !strings.Contains(found[0].Excerpt, header+bullets) {
			t.Errorf("the excerpt %q names the key's kind", found[0].Excerpt)
		}
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
		"schemes":         repeat("a://b:"),
		"addresses":       repeat("postgres://u:" + random(302, alnum, 20) + "@db "),
		"open addresses":  "x://" + repeat("u:p"),
		"keys with body":  repeat("-----BEGIN PRIVATE" + " KEY----- MII "),
		"dotted parts":    repeat("SG.abc.hvs."),
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
