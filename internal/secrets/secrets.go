// Package secrets finds what looks like a credential in text: the shapes of the common keys and tokens, and a long
// random value assigned to a name such as API_KEY or password, with the obvious placeholders left alone. A line is
// read once, whatever its length, and a finding carries its place, its rule and a masked excerpt, never the secret.
package secrets

import (
	"sort"
	"strings"
)

// The rules a finding names.
const (
	RulePrivateKey   = "private-key"
	RuleAWS          = "aws-access-key"
	RuleGitHub       = "github-token"
	RuleGitHubFine   = "github-fine-grained-token"
	RuleGitLab       = "gitlab-token"
	RuleSlack        = "slack-token"
	RuleSlackWebhook = "slack-webhook"
	RuleStripe       = "stripe-live-key"
	RuleAnthropic    = "anthropic-key"
	RuleOpenAI       = "openai-key"
	RuleGoogle       = "google-api-key"
	RuleTelegram     = "telegram-bot-token"
	RuleNPM          = "npm-token"
	RuleHuggingFace  = "huggingface-token"
	RuleGroq         = "groq-key"
	RuleXAI          = "xai-key"
	RuleOpenRouter   = "openrouter-key"
	RuleSendGrid     = "sendgrid-key"
	RuleDigitalOcean = "digitalocean-token"
	RuleDocker       = "docker-token"
	RulePyPI         = "pypi-token"
	RuleGoogleOAuth  = "google-oauth-secret"
	RuleShopify      = "shopify-token"
	RuleLinear       = "linear-key"
	RuleAtlassian    = "atlassian-token"
	RuleAge          = "age-key"
	RuleVault        = "vault-token"
	RuleJWT          = "jwt"
	RuleAuthHeader   = "auth-header"
	RuleURLPassword  = "url-password"
	RuleAssigned     = "assigned-secret"
	RuleKeyFile      = "key-file"
)

// Rules is every rule, in the order the page lists their words.
var Rules = []string{RulePrivateKey, RuleAWS, RuleGitHub, RuleGitHubFine, RuleGitLab, RuleSlack, RuleSlackWebhook, RuleStripe,
	RuleAnthropic, RuleOpenAI, RuleGoogle, RuleTelegram, RuleNPM, RuleHuggingFace, RuleGroq, RuleXAI, RuleOpenRouter, RuleSendGrid,
	RuleDigitalOcean, RuleDocker, RulePyPI, RuleGoogleOAuth, RuleShopify, RuleLinear, RuleAtlassian, RuleAge, RuleVault, RuleJWT,
	RuleAuthHeader, RuleURLPassword, RuleAssigned, RuleKeyFile}

// Finding is one thing that looks like a secret: where it is, which rule saw it, and the line around it with every
// secret on it masked.
type Finding struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Rule    string `json:"rule"`
	Excerpt string `json:"excerpt"`
	Commit  string `json:"commit,omitempty"` // the commit that adds it, for one a push would carry; Staged for the index's
}

// Staged is the Commit of a finding only the index holds: staged, then changed or gone in the file, it is what git
// commit carries all the same.
const Staged = "staged"

// The caps on what one scan reports: a file of a thousand keys says so once.
const (
	MaxPerLine  = 4
	MaxPerFile  = 20
	MaxFindings = 100
)

// hit is one secret on a line: the bytes it spans and the leading ones the mask may show (the kind's own prefix).
type hit struct {
	start, end int
	rule       string
	keep       int
	noTail     bool // the mask shows none of its last bytes: a private key's body
}

// allowMarkers are the comments that let a line through: gitleaks' and detect-secrets' own, and remuda's.
var allowMarkers = []string{"gitleaks:allow", "pragma: allowlist secret", "remuda:allow-secret"}

// scanLine is every secret on one line, in order, none overlapping another.
func scanLine(line string) []hit {
	for _, marker := range allowMarkers {
		if strings.Contains(line, marker) {
			return nil
		}
	}
	var hits []hit
	hits = privateKeys(line, hits)
	hits = slackWebhooks(line, hits)
	hits = tokens(line, hits)
	if len(hits) < 2 {
		return hits
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].start < hits[j].start })
	kept := hits[:1]
	for _, next := range hits[1:] {
		if next.start >= kept[len(kept)-1].end {
			kept = append(kept, next)
		}
	}
	return kept
}

// Collector gathers the findings of many lines and files under the caps.
type Collector struct {
	findings []Finding
	more     int
	perFile  map[string]int
	seen     map[string]bool // the file and the secret of every finding, kept only while the scan runs
}

// NewCollector is an empty collector.
func NewCollector() *Collector {
	return &Collector{perFile: map[string]int{}, seen: map[string]bool{}}
}

// Line scans one line of a file; number counts from 1, commit names the commit that adds it, "" for the work.
func (c *Collector) Line(file string, number int, text, commit string) {
	text = strings.TrimSuffix(text, "\r")
	hits := scanLine(text)
	for index, found := range hits {
		if index >= MaxPerLine || c.perFile[file] >= MaxPerFile || len(c.findings) >= MaxFindings {
			c.more += len(hits) - index
			return
		}
		// a secret a commit adds that the work or an earlier commit already showed in the file is said once
		secret := file + "\x00" + text[found.start:found.end]
		if commit != "" && c.seen[secret] {
			continue
		}
		c.seen[secret] = true
		finding := Finding{File: file, Line: number, Rule: found.rule, Excerpt: excerpt(text, hits, index), Commit: commit}
		c.perFile[file]++
		c.findings = append(c.findings, finding)
	}
}

// keyFileEndings are the binary stores of keys and certificates, which no line of text gives away.
var keyFileEndings = []string{".p12", ".pfx", ".jks", ".keystore", ".kdbx"}

// KeyFile says whether a binary file is by its name a store of private keys.
func KeyFile(path string) bool {
	lowered := strings.ToLower(path)
	for _, ending := range keyFileEndings {
		if strings.HasSuffix(lowered, ending) {
			return true
		}
	}
	return false
}

// Binary notes a binary file the work or a commit adds: a finding on the file as a whole (line 0) when its name says
// it holds keys, nothing otherwise.
func (c *Collector) Binary(file, commit string) {
	if !KeyFile(file) || c.seen[file] || len(c.findings) >= MaxFindings {
		return
	}
	c.seen[file] = true
	c.findings = append(c.findings, Finding{File: file, Rule: RuleKeyFile, Excerpt: file[strings.LastIndex(file, "/")+1:], Commit: commit})
}

// Text scans a file's text whole, line by line.
func (c *Collector) Text(file, text string) {
	for number := 1; text != ""; number++ {
		line, rest, _ := strings.Cut(text, "\n")
		c.Line(file, number, line, "")
		text = rest
	}
}

// Findings is what was found, in the order it was read.
func (c *Collector) Findings() []Finding {
	if c.findings == nil {
		return []Finding{}
	}
	return c.findings
}

// More counts the findings past the caps.
func (c *Collector) More() int { return c.more }

// Scan is every finding in one file's text.
func Scan(file, text string) []Finding {
	collector := NewCollector()
	collector.Text(file, text)
	return collector.Findings()
}
