package push

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
)

// receiver records every request a webhook sent, across goroutines.
type receiver struct {
	mu   sync.Mutex
	got  []received
	code int
}

type received struct {
	path, body string
	header     http.Header
}

func (r *receiver) serve(w http.ResponseWriter, req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	r.mu.Lock()
	r.got = append(r.got, received{path: req.URL.Path, body: string(body), header: req.Header.Clone()})
	code := r.code
	r.mu.Unlock()
	if code != 0 {
		http.Error(w, "nope", code)
	}
}

func (r *receiver) list() []received {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]received(nil), r.got...)
}

func newWebhooks(t *testing.T) (*Webhooks, *receiver, *httptest.Server) {
	t.Helper()
	rec := &receiver{}
	srv := httptest.NewServer(http.HandlerFunc(rec.serve))
	t.Cleanup(srv.Close)
	w := loadWebhooks(t.TempDir())
	return w, rec, srv
}

func noRetryPause(t *testing.T) {
	t.Helper()
	was := webhookRetry
	webhookRetry = time.Millisecond
	t.Cleanup(func() { webhookRetry = was })
}

func TestEachKindSendsItsOwnRequest(t *testing.T) {
	w, rec, srv := newWebhooks(t)
	was := telegramAPI
	telegramAPI = srv.URL
	t.Cleanup(func() { telegramAPI = was })
	err := w.Replace("https://remuda.tail.ts.net/", []Webhook{
		{Kind: "telegram", Name: "tg", Token: "123:ABC", Chat: "42", Kinds: []string{"question"}, Enabled: true},
		{Kind: "slack", Name: "sl", URL: srv.URL + "/services/T/B/secret", Kinds: []string{"question"}, Enabled: true},
		{Kind: "ntfy", Name: "nt", URL: srv.URL + "/remuda", Kinds: []string{"question"}, Enabled: true},
		{Kind: "generic", Name: "gen", URL: srv.URL + "/hook", Kinds: []string{"question"}, Enabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	w.Deliver(Payload{Machine: "local", Pane: "w3:p1", Kind: "input", Title: "api needs you", Body: "claude is waiting · Привет"})
	w.Wait()
	got := rec.list()
	if len(got) != 4 {
		t.Fatalf("want 4 requests, got %d", len(got))
	}
	by := map[string]received{}
	for _, r := range got {
		by[r.path] = r
	}
	link := "https://remuda.tail.ts.net/?machine=local&pane=w3%3Ap1"
	tg := by["/bot123:ABC/sendMessage"]
	var tgBody struct {
		Chat    string `json:"chat_id"`
		Text    string `json:"text"`
		Preview bool   `json:"disable_web_page_preview"`
	}
	if err := json.Unmarshal([]byte(tg.body), &tgBody); err != nil || tgBody.Chat != "42" || !tgBody.Preview || tgBody.Text != "api needs you\nclaude is waiting · Привет\n"+link {
		t.Fatalf("telegram: %s (%v)", tg.body, err)
	}
	sl := by["/services/T/B/secret"]
	var slBody struct{ Text string }
	if err := json.Unmarshal([]byte(sl.body), &slBody); err != nil || slBody.Text != "api needs you\nclaude is waiting · Привет\n"+link || sl.header.Get("Content-Type") != "application/json" {
		t.Fatalf("slack: %s %v", sl.body, sl.header)
	}
	nt := by["/remuda"]
	if nt.body != "claude is waiting · Привет" || nt.header.Get("Title") != "api needs you" || nt.header.Get("Click") != link || nt.header.Get("Priority") != "high" {
		t.Fatalf("ntfy: %q %v", nt.body, nt.header)
	}
	var m Message
	if err := json.Unmarshal([]byte(by["/hook"].body), &m); err != nil || m.Kind != "input" || m.Pane != "w3:p1" || m.Link != link || m.At.IsZero() {
		t.Fatalf("generic: %s (%v)", by["/hook"].body, err)
	}
	_, views := w.Views()
	for _, v := range views {
		if v.LastOK == nil || v.LastError != "" {
			t.Fatalf("%s should note a success: %+v", v.Name, v)
		}
	}
}

func TestNonASCIITitleIsEncodedForNtfy(t *testing.T) {
	w, rec, srv := newWebhooks(t)
	if err := w.Replace("", []Webhook{{Kind: "ntfy", URL: srv.URL + "/t", Kinds: []string{"finish"}, Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	w.Deliver(Payload{Kind: "done", Title: "сайт finished", Body: "done"})
	w.Wait()
	got := rec.list()
	if len(got) != 1 || !strings.HasPrefix(got[0].header.Get("Title"), "=?utf-8?b?") || got[0].header.Get("Click") != "" {
		t.Fatalf("got %+v", got)
	}
}

func TestKindsAndQuietHoursFilter(t *testing.T) {
	w, rec, srv := newWebhooks(t)
	err := w.Replace("", []Webhook{
		{Kind: "generic", Name: "questions", URL: srv.URL + "/q", Kinds: []string{"question", "watchdog"}, Enabled: true},
		{Kind: "generic", Name: "off", URL: srv.URL + "/off", Kinds: []string{"question"}},
		{Kind: "generic", Name: "night", URL: srv.URL + "/night", Kinds: []string{"question"}, Enabled: true, Quiet: &Quiet{From: "22:00", To: "07:00", TZ: "UTC"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	w.now = func() time.Time { return time.Date(2026, 10, 29, 23, 30, 0, 0, time.UTC) }
	if !w.Wants("input") || !w.Wants("stuck") || w.Wants("done") || w.Wants("test") {
		t.Fatal("wants: question and the watchdog's kinds only")
	}
	w.Deliver(Payload{Kind: "input", Title: "q"})
	w.Deliver(Payload{Kind: "loop", Title: "w"})
	w.Deliver(Payload{Kind: "done", Title: "d"})
	w.Wait()
	var paths []string
	for _, r := range rec.list() {
		paths = append(paths, r.path)
	}
	if strings.Join(paths, " ") != "/q /q" {
		t.Fatalf("want the question and the loop at /q alone, got %v", paths)
	}
	w.now = func() time.Time { return time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC) }
	w.Deliver(Payload{Kind: "input", Title: "q"})
	w.Wait()
	if n := len(rec.list()); n != 4 {
		t.Fatalf("by day the night webhook hears too: want 4 requests, got %d", n)
	}
}

func TestViewsMaskSecretsAndReplaceKeepsThem(t *testing.T) {
	w, _, _ := newWebhooks(t)
	err := w.Replace("http://127.0.0.1:7317", []Webhook{
		{Kind: "telegram", Name: "tg", Token: "123:ABC", Chat: "42", Kinds: []string{"question", "bogus", "question"}, Enabled: true},
		{Kind: "slack", Name: "sl", URL: "https://hooks.slack.com/services/T/B/secret", Kinds: []string{"finish"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	base, views := w.Views()
	if base != "http://127.0.0.1:7317" || len(views) != 2 {
		t.Fatalf("got %q %+v", base, views)
	}
	tg, sl := views[0], views[1]
	if tg.Token != "" || !tg.HasToken || tg.Chat != "42" || strings.Join(tg.Kinds, ",") != "question" || tg.ID == "" {
		t.Fatalf("telegram view: %+v", tg)
	}
	if sl.URL != "https://hooks.slack.com/…" || sl.HasToken {
		t.Fatalf("slack view: %+v", sl)
	}
	raw, _ := json.Marshal(views)
	if strings.Contains(string(raw), "123:ABC") || strings.Contains(string(raw), "secret") {
		t.Fatalf("a secret leaks through the view: %s", raw)
	}
	// the page sends the views back: the token empty, the URL masked
	if err := w.Replace("", []Webhook{tg.Webhook, sl.Webhook}); err != nil {
		t.Fatal(err)
	}
	if w.hooks[0].Token != "123:ABC" || w.hooks[1].URL != "https://hooks.slack.com/services/T/B/secret" {
		t.Fatalf("secrets not kept: %+v", w.hooks)
	}
	// a new token replaces the stored one
	tg.Token = "999:NEW"
	if err := w.Replace("", []Webhook{tg.Webhook}); err != nil {
		t.Fatal(err)
	}
	if len(w.hooks) != 1 || w.hooks[0].Token != "999:NEW" {
		t.Fatalf("new token not taken: %+v", w.hooks)
	}
	file, _ := os.ReadFile(filepath.Join(w.dir, webhooksFile))
	if !strings.Contains(string(file), "999:NEW") {
		t.Fatalf("not saved: %s", file)
	}
	again := loadWebhooks(w.dir)
	if len(again.hooks) != 1 || again.hooks[0].Token != "999:NEW" {
		t.Fatalf("not loaded back: %+v", again.hooks)
	}
}

func TestReplaceRefusesWhatCannotBeSent(t *testing.T) {
	w, _, _ := newWebhooks(t)
	bad := []struct {
		name string
		base string
		hook Webhook
	}{
		{"kind", "", Webhook{Kind: "sms", URL: "https://x"}},
		{"no chat", "", Webhook{Kind: "telegram", Token: "t"}},
		{"no token", "", Webhook{Kind: "telegram", Chat: "1"}},
		{"bad url", "", Webhook{Kind: "ntfy", URL: "ntfy.sh/topic"}},
		{"bad base", "remuda.local", Webhook{Kind: "ntfy", URL: "https://ntfy.sh/topic"}},
	}
	for _, c := range bad {
		if err := w.Replace(c.base, []Webhook{c.hook}); err == nil {
			t.Errorf("%s: accepted", c.name)
		}
	}
	if len(w.hooks) != 0 {
		t.Fatalf("a refused list must leave the store as it was: %+v", w.hooks)
	}
	if err := w.Replace("", []Webhook{{Kind: "ntfy", URL: "https://ntfy.sh/topic"}}); err != nil || w.hooks[0].Name != "Ntfy" {
		t.Fatalf("a nameless webhook is named after its kind: %v %+v", err, w.hooks)
	}
}

func TestFailuresAreNotedWithoutTheAddress(t *testing.T) {
	noRetryPause(t)
	w, rec, srv := newWebhooks(t)
	was := telegramAPI
	telegramAPI = "http://127.0.0.1:1"
	t.Cleanup(func() { telegramAPI = was })
	err := w.Replace("", []Webhook{
		{Kind: "telegram", Name: "tg", Token: "123:SECRET", Chat: "1", Kinds: []string{"question"}, Enabled: true},
		{Kind: "generic", Name: "gen", URL: srv.URL + "/hook", Kinds: []string{"question"}, Enabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	rec.code = 500
	w.Deliver(Payload{Kind: "input", Title: "q"})
	w.Wait()
	if n := len(rec.list()); n != 2 {
		t.Fatalf("a 500 is tried once more: want 2 requests, got %d", n)
	}
	_, views := w.Views()
	if views[0].LastError == "" || strings.Contains(views[0].LastError, "SECRET") || strings.Contains(views[0].LastError, "sendMessage") {
		t.Fatalf("telegram's error must not carry the token: %q", views[0].LastError)
	}
	if views[1].LastError != "answered 500: nope" || views[1].LastErrorAt == nil {
		t.Fatalf("generic's error: %q", views[1].LastError)
	}
	rec.code = 404
	rec.got = nil
	if err := w.Test(views[1].ID); err == nil || !strings.HasPrefix(err.Error(), "answered 404") {
		t.Fatalf("test should answer the failure: %v", err)
	}
	if n := len(rec.list()); n != 1 {
		t.Fatalf("a 404 is not tried again: want 1 request, got %d", n)
	}
	rec.code = 0
	if err := w.Test(views[1].ID); err != nil {
		t.Fatal(err)
	}
	_, views = w.Views()
	if views[1].LastError != "" || views[1].LastOK == nil {
		t.Fatalf("a success clears the error: %+v", views[1])
	}
	if err := w.Test("nope"); err == nil {
		t.Fatal("an unknown id is refused")
	}
}

func TestLinkAndMasked(t *testing.T) {
	cases := []struct {
		base, want string
		p          Payload
	}{
		{"", "", Payload{Pane: "p"}},
		{"https://r.ts.net", "https://r.ts.net/?machine=gog&pane=w1%3Ap2", Payload{Machine: "gog", Pane: "w1:p2"}},
		{"https://r.ts.net", "https://r.ts.net/?panel=standup", Payload{Panel: "standup", Kind: "digest"}},
		{"https://r.ts.net", "https://r.ts.net", Payload{Kind: "test"}},
	}
	for _, c := range cases {
		if got := Link(c.base, c.p); got != c.want {
			t.Errorf("Link(%q, %+v) = %q, want %q", c.base, c.p, got, c.want)
		}
	}
	for raw, want := range map[string]string{"": "", "https://ntfy.sh/my-topic?x=1": "https://ntfy.sh/…", "nonsense": "…"} {
		if got := Masked(raw); got != want {
			t.Errorf("Masked(%q) = %q, want %q", raw, got, want)
		}
	}
	for kind, want := range map[string]string{"input": "question", "done": "finish", "stuck": "watchdog", "waiting": "watchdog", "loop": "watchdog", "digest": "digest", "task": "task"} {
		if got := HookKind(kind); got != want {
			t.Errorf("HookKind(%q) = %q, want %q", kind, got, want)
		}
	}
}

// The service: with no device subscribed, a question still reaches a webhook after the delay, and an answer at the desk calls it off.
func TestWebhooksHearTheScheduledAlerts(t *testing.T) {
	shrinkDelays(t)
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rec := &receiver{}
	srv := httptest.NewServer(http.HandlerFunc(rec.serve))
	t.Cleanup(srv.Close)
	if err := s.Webhooks.Replace("", []Webhook{{Kind: "generic", URL: srv.URL + "/h", Kinds: []string{"question", "finish"}, Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	pane := events.PaneRef{Machine: "local", Pane: "p1"}
	s.OnStatus(events.PaneStatus{Pane: pane, Workspace: "api", Agent: "claude", From: "working", To: "blocked"})
	time.Sleep(10 * time.Millisecond)
	s.OnStatus(events.PaneStatus{Pane: pane, From: "blocked", To: "working"})
	time.Sleep(80 * time.Millisecond)
	s.Webhooks.Wait()
	if n := len(rec.list()); n != 0 {
		t.Fatalf("answered at the desk: want nothing, got %d", n)
	}
	s.OnStatus(events.PaneStatus{Pane: pane, Workspace: "api", Agent: "claude", From: "working", To: "blocked"})
	time.Sleep(80 * time.Millisecond)
	s.Webhooks.Wait()
	got := rec.list()
	if len(got) != 1 || !strings.Contains(got[0].body, `"title":"api needs you"`) {
		t.Fatalf("want the question once, got %+v", got)
	}
	s.OnStatus(events.PaneStatus{Pane: pane, Workspace: "api", Agent: "claude", From: "working", To: "done", Worked: 10 * time.Millisecond})
	time.Sleep(120 * time.Millisecond)
	s.Webhooks.Wait()
	if got := rec.list(); len(got) != 2 || !strings.Contains(got[1].body, `"kind":"done"`) {
		t.Fatalf("a finish goes once, as the feed hears it: %d requests", len(got))
	}
}
