package triggers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckRefusesWhatHerdrCouldNotWatch(t *testing.T) {
	good := Rule{Name: " Deployed ", Match: "substring", Pattern: "DEPLOYED", Action: ActionPush, Column: "x"}
	if err := Check(&good); err != nil || good.Name != "Deployed" || good.Column != "" {
		t.Fatalf("a good rule: %v %+v", err, good)
	}
	cases := map[string]Rule{
		"a trigger needs a name":         {Match: "substring", Pattern: "x", Action: ActionPush},
		"looks for nothing":              {Name: "n", Match: "substring", Pattern: "  ", Action: ActionPush},
		"holds no line break":            {Name: "n", Match: "substring", Pattern: "a\nb", Action: ActionPush},
		"match by substring or regex":    {Name: "n", Match: "glob", Pattern: "x", Action: ActionPush},
		"the regex does not read":        {Name: "n", Match: "regex", Pattern: "(", Action: ActionPush},
		`\Q…\E is not in herdr's regex`:  {Name: "n", Match: "regex", Pattern: `\Qa.b\E`, Action: ActionPush},
		"matches an empty line":          {Name: "n", Match: "regex", Pattern: `.*`, Action: ActionPush},
		"unknown action":                 {Name: "n", Match: "substring", Pattern: "x", Action: "shell"},
		"name the column":                {Name: "n", Match: "substring", Pattern: "x", Action: ActionTask},
		"the folder is an absolute path": {Name: "n", Match: "substring", Pattern: "x", Action: ActionPush, Machine: "local", Dir: "src"},
		"a folder belongs to a machine":  {Name: "n", Match: "substring", Pattern: "x", Action: ActionPush, Dir: "/src"},
		"bytes at most":                  {Name: "n", Match: "substring", Pattern: strings.Repeat("x", MaxPattern+1), Action: ActionPush},
	}
	for want, rule := range cases {
		if err := Check(&rule); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%+v: %v, want %q", rule, err, want)
		}
	}
	lookahead := Rule{Name: "n", Match: "regex", Pattern: `(?=x)`, Action: ActionPush}
	if err := Check(&lookahead); err == nil {
		t.Error("a look-ahead reads in neither dialect")
	}
}

func TestTheRulesAreKeptPrivateAndReplacedWhole(t *testing.T) {
	dir := t.TempDir()
	st := Open(dir)
	if list := st.List(); list == nil || len(list) != 0 {
		t.Fatalf("no rules is an empty list: %#v", list)
	}
	out, err := st.Replace([]Rule{
		{ID: "a", Name: "Deployed", Match: "substring", Pattern: "DEPLOYED", Action: ActionFeed, Enabled: true},
		{ID: "a", Name: "Ready", Machine: "gog", Dir: "/home/me/api/", Match: "regex", Pattern: `ready in \d+ms`, Action: ActionTask, Column: "Testing", Enabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out[0].ID != "a" || out[1].ID == "a" || out[1].ID == "" || out[1].Dir != "/home/me/api" {
		t.Fatalf("ids and folders: %+v", out)
	}
	info, err := os.Stat(filepath.Join(dir, "triggers.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("triggers.json: %v %v", info, err)
	}
	if again := Open(dir).List(); len(again) != 2 || again[1].Column != "Testing" {
		t.Fatalf("read back: %+v", again)
	}
	if _, err := st.Replace([]Rule{{Name: "bad", Match: "regex", Pattern: "(", Action: ActionPush}}); err == nil {
		t.Fatal("a bad rule was kept")
	}
	if len(st.List()) != 2 {
		t.Fatal("a refused list leaves the rules as they were")
	}
	if _, err := st.Replace(make([]Rule, MaxRules+1)); err == nil {
		t.Fatal("past the cap")
	}
	if err := os.WriteFile(filepath.Join(dir, "triggers.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if list := Open(dir).List(); len(list) != 0 {
		t.Fatalf("a damaged file is no rules: %+v", list)
	}
}

func TestARuleCoversItsFolderAndBelow(t *testing.T) {
	everywhere := Rule{Enabled: true}
	folder := Rule{Enabled: true, Machine: "gog", Dir: "/home/me/api"}
	machine := Rule{Enabled: true, Machine: "gog"}
	off := Rule{}
	cases := []struct {
		rule         Rule
		machine, dir string
		cwd          string
		want         bool
		what         string
	}{
		{everywhere, "local", "/x", "/x", true, "everywhere"},
		{off, "local", "/x", "/x", false, "a rule switched off"},
		{folder, "gog", "/home/me/api", "/home/me/api", true, "the folder itself"},
		{folder, "gog", "/home/me/api-v2", "/tmp", false, "a folder that only starts the same"},
		{folder, "gog", "/home/me", "/home/me/api/web", true, "a shell gone into it"},
		{folder, "local", "/home/me/api", "/home/me/api", false, "the same folder on another machine"},
		{machine, "gog", "/srv", "/srv", true, "every folder of the machine"},
	}
	for _, c := range cases {
		if got := c.rule.Covers(c.machine, c.dir, c.cwd); got != c.want {
			t.Errorf("%s: %v", c.what, got)
		}
	}
}

func TestTryRunsARuleOverAPanesRows(t *testing.T) {
	text := "$ npm run dev\n\n  VITE v6.0.0  ready in 300 ms\n\n  ➜  Local:   http://localhost:5173/\n  ➜  Network: use --host\n"
	got, err := Try(Rule{Match: "regex", Pattern: `Local:\s+http`, Action: ActionPreview}, text)
	if err != nil || got.Count != 1 || got.Port != 5173 || got.Lines[0] != "  ➜  Local:   http://localhost:5173/" {
		t.Fatalf("tried: %+v %v", got, err)
	}
	if got, _ := Try(Rule{Match: "substring", Pattern: "nothing like it", Action: ActionPush}, text); got.Count != 0 || got.Lines == nil {
		t.Fatalf("no match is an empty list: %+v", got)
	}
	if _, err := Try(Rule{Match: "regex", Pattern: "(", Action: ActionPush}, text); err == nil {
		t.Fatal("a bad pattern is said so")
	}
	many := strings.Repeat("hit\n", MaxShown+5)
	if got, _ := Try(Rule{Match: "substring", Pattern: "hit", Action: ActionFeed}, many); got.Count != MaxShown+5 || len(got.Lines) != MaxShown {
		t.Fatalf("the last rows are shown, all are counted: %d %d", got.Count, len(got.Lines))
	}
}
