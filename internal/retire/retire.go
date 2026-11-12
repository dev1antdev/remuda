// Package retire knows the models, and the modes of a model, being retired, and finds what remuda starts or watches
// that is set to one: a recipe, an agent schedule, a project's agents and its pipeline's stages, and a live pane whose
// meter reads it. After the day their requests fail; before it, the roster says so in the machine's vitals.
package retire

import (
	"regexp"
	"strings"
	"time"
)

// Retirement is a model, or one mode of it, going away on a day.
type Retirement struct {
	Agent           string `json:"agent"`          // the agent kind whose model it is
	Model           string `json:"model"`          // the model's id, as the agent's CLI and its transcripts name it
	Name            string `json:"name"`           // as a person reads it
	Mode            string `json:"mode,omitempty"` // "fast": only that mode goes; "": the whole model
	On              string `json:"on"`             // the day it goes, YYYY-MM-DD (UTC): from then on its requests fail
	Replacement     string `json:"replacement"`    // the model to move to
	ReplacementName string `json:"replacement_name"`
	Source          string `json:"source"` // where the word came from
}

// Table is every retirement remuda knows of; an entry stays a while past its day, so what still uses it says it fails.
var Table = []Retirement{{
	Agent:           "claude",
	Model:           "claude-opus-4-8",
	Name:            "Opus 4.8",
	Mode:            "fast",
	On:              "2026-11-30",
	Replacement:     "claude-opus-5-5",
	ReplacementName: "Opus 5.5",
	Source:          "Claude Code 2.1.316: fast mode for Opus 4.8 deprecated as of 30 October 2026 and removed on 30 November 2026; after that its requests return an error and do not fall back to standard Opus 4.8",
}}

// Ends is the moment the retirement takes effect, the start of its day in UTC.
func (r Retirement) Ends() time.Time {
	day, err := time.Parse(time.DateOnly, r.On)
	if err != nil {
		return time.Time{}
	}
	return day
}

// Failing says whether its day has come.
func (r Retirement) Failing(now time.Time) bool {
	ends := r.Ends()
	return !ends.IsZero() && !now.Before(ends)
}

// Applies says whether an agent on this model, in fast mode or not, is on the retirement's way.
func (r Retirement) Applies(agent, model string, fast bool) bool {
	return agentOr(agent) == r.Agent && Canonical(model) == r.Model && (r.Mode == "" || (r.Mode == "fast" && fast))
}

var (
	bracketTail  = regexp.MustCompile(`\[[^\]]*\]$`)   // "claude-opus-4-8[1m]"
	versionTail  = regexp.MustCompile(`-v\d+(:\d+)?$`) // Bedrock's "-v1:0"
	dateTail     = regexp.MustCompile(`-\d{8}$`)       // "-20251101"
	fastSetting  = regexp.MustCompile(`"fastMode"\s*:\s*(true|false)`)
	modelOverArg = regexp.MustCompile(`--model(?:=|\s+)["']?([^\s"']+)`)
)

// Canonical is a model id as the table names it: lowercase, past a provider's prefix ("us.anthropic.", "anthropic/"),
// without a context window's "[1m]", a Vertex "@date", Bedrock's "-v1:0" or a date. An alias ("opus") stays as it is.
func Canonical(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	if i := strings.Index(m, "claude-"); i > 0 {
		m = m[i:]
	}
	m = bracketTail.ReplaceAllString(m, "")
	if i := strings.IndexByte(m, '@'); i > 0 {
		m = m[:i]
	}
	m = versionTail.ReplaceAllString(m, "")
	return dateTail.ReplaceAllString(m, "")
}

// unescaped is the arguments with a backslash before a double quote dropped, as a shell drops it inside double quotes:
// --settings "{\"fastMode\":true}" says what --settings '{"fastMode":true}' says
func unescaped(args string) string { return strings.ReplaceAll(args, `\"`, `"`) }

// FastIn reads what the extra arguments say of fast mode: on or off through a --settings JSON, or nothing (set false).
func FastIn(args string) (fast, said bool) {
	m := fastSetting.FindAllStringSubmatch(unescaped(args), -1)
	if len(m) == 0 {
		return false, false
	}
	return m[len(m)-1][1] == "true", true
}

// ModelIn is a model the extra arguments name with --model, which comes after remuda's own and wins; "" for none.
func ModelIn(args string) string {
	m := modelOverArg.FindAllStringSubmatch(unescaped(args), -1)
	if len(m) == 0 {
		return ""
	}
	return m[len(m)-1][1]
}

func agentOr(kind string) string {
	if kind == "" {
		return "claude"
	}
	return kind
}
