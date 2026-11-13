package ghpr

import (
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestTheBranchsPullRequestsComeFromItsOwnRemotesOpenFirst(t *testing.T) {
	answer := `{"data":{"r0":{"pullRequests":{"nodes":[` +
		`{"id":"A","number":470,"title":"Old try","url":"u","state":"CLOSED","isDraft":false,"updatedAt":"2026-11-13T12:00:00Z","headRefName":"offline-trips","baseRefName":"main","author":{"login":"sam"},"headRepository":{"nameWithOwner":"octo/trips"},"repository":{"nameWithOwner":"octo/trips"},"commits":{"nodes":[]}},` +
		`{"id":"B","number":482,"title":"Save trips","url":"u","state":"OPEN","isDraft":false,"updatedAt":"2026-11-13T09:00:00Z","headRefName":"offline-trips","baseRefName":"main","author":{"login":"sam"},"headRepository":{"nameWithOwner":"Octo/Trips"},"repository":{"nameWithOwner":"octo/trips"},"commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE"}}}]}},` +
		`{"id":"C","number":490,"title":"A fork's","url":"u","state":"OPEN","isDraft":false,"updatedAt":"2026-11-13T13:00:00Z","headRefName":"offline-trips","baseRefName":"main","author":{"login":"eve"},"headRepository":{"nameWithOwner":"eve/trips"},"repository":{"nameWithOwner":"octo/trips"},"commits":{"nodes":[]}}]}},` +
		`"r1":{"pullRequests":{"nodes":[{"id":"B","number":482,"title":"Save trips","url":"u","state":"OPEN","isDraft":false,"updatedAt":"2026-11-13T09:00:00Z","headRefName":"offline-trips","baseRefName":"main","author":{"login":"sam"},"headRepository":{"nameWithOwner":"octo/trips"},"repository":{"nameWithOwner":"octo/trips"},"commits":{"nodes":[]}}]}}}}`
	host := fakeGH([2]string{"graphql", answered(answer, "", 0)})
	c := Checkout{Branch: "offline-trips", Remotes: []Remote{{Name: "origin", Repo: "octo/trips"}, {Name: "mine", Repo: "sam/trips"}, {Name: "lab", Repo: ""}}}
	cards, err := BranchPRs(host, c)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 2 || cards[0].Number != 482 || cards[1].Number != 470 || cards[0].Checks != "FAILURE" || cards[0].Repository != "octo/trips" {
		t.Fatalf("cards = %+v", cards)
	}
	in := inputOf(t, host.Scripts()[0])
	query := in["query"].(string)
	if !strings.Contains(query, `r0: repository(owner:"octo",name:"trips")`) || !strings.Contains(query, `r1: repository(owner:"sam",name:"trips")`) || strings.Count(query, "repository(owner") != 2 {
		t.Fatalf("query = %s", query)
	}
	if in["variables"].(map[string]any)["branch"] != "offline-trips" {
		t.Fatalf("variables = %v", in["variables"])
	}
	none, err := BranchPRs(host, Checkout{Remotes: c.Remotes})
	if err != nil || len(none) != 0 || len(host.Scripts()) != 1 {
		t.Fatal("a detached checkout asked GitHub")
	}
}

func TestTheRepoListSearchesAndPages(t *testing.T) {
	page := func(count int, next bool, ids ...string) string {
		nodes := []string{}
		for i, id := range ids {
			nodes = append(nodes, `{"id":"`+id+`","number":`+itoa(500+i)+`,"title":"t","url":"u","state":"OPEN","isDraft":false,"updatedAt":"2026-11-1`+itoa(i)+`T00:00:00Z","headRefName":"h","baseRefName":"main","author":{"login":"sam"},"headRepository":null,"repository":{"nameWithOwner":"octo/trips"},"commits":{"nodes":[]}}`)
		}
		end := "null"
		if next {
			end = `"CUR"`
		}
		return `{"issueCount":` + itoa(count) + `,"pageInfo":{"hasNextPage":` + map[bool]string{true: "true", false: "false"}[next] + `,"endCursor":` + end + `},"nodes":[` + strings.Join(nodes, ",") + `]}`
	}
	repo := Repo{"octo", "trips"}
	all := fakeGH([2]string{"graphql", answered(`{"data":{"s0":`+page(120, true, "A", "B")+`}}`, "", 0)})
	found, err := SearchPRs(all, repo, Search{Text: "label:bug offline", Scope: "all", State: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	if found.Total == nil || *found.Total != 120 || found.Cursors["all"] != "CUR" || len(found.Items) != 2 || found.Items[0].ID != "B" {
		t.Fatalf("found = %+v", found)
	}
	in := inputOf(t, all.Scripts()[0])
	if q := in["variables"].(map[string]any)["q0"]; q != "repo:octo/trips is:pr is:open draft:true sort:updated-desc label:bug offline" {
		t.Fatalf("q = %v", q)
	}

	involved := fakeGH([2]string{"graphql", answered(`{"data":{"s0":`+page(3, false, "A")+`,"s1":`+page(2, true, "A", "C")+`,"s2":`+page(1, false, "D")+`}}`, "", 0)})
	found, err = SearchPRs(involved, repo, Search{Scope: "involved", State: "open"})
	if err != nil {
		t.Fatal(err)
	}
	if found.Total != nil || len(found.Items) != 3 || len(found.Cursors) != 1 || found.Cursors["review"] != "CUR" {
		t.Fatalf("involved = %+v", found)
	}
	in = inputOf(t, involved.Scripts()[0])
	vars := in["variables"].(map[string]any)
	if !strings.HasSuffix(vars["q0"].(string), "involves:@me") || !strings.HasSuffix(vars["q1"].(string), "review-requested:@me") || !strings.HasSuffix(vars["q2"].(string), "reviewed-by:@me") {
		t.Fatalf("vars = %v", vars)
	}
	more := fakeGH([2]string{"graphql", answered(`{"data":{"s0":`+page(2, false, "E")+`}}`, "", 0)})
	if _, err := SearchPRs(more, repo, Search{Scope: "involved", State: "open", More: true, Cursors: found.Cursors}); err != nil {
		t.Fatal(err)
	}
	in = inputOf(t, more.Scripts()[0])
	vars = in["variables"].(map[string]any)
	if vars["c0"] != "CUR" || !strings.HasSuffix(vars["q0"].(string), "review-requested:@me") || vars["q1"] != nil {
		t.Fatalf("the next page asked again what had no next page: %v", vars)
	}

	number := fakeGH([2]string{"pullRequest(number", answered(`{"data":{"repository":{"pullRequest":{"id":"N","number":482,"title":"t","url":"u","state":"MERGED","isDraft":false,"updatedAt":"x","headRefName":"h","baseRefName":"main","author":null,"headRepository":null,"repository":{"nameWithOwner":"octo/trips"},"commits":{"nodes":[]}}}}}`, "", 0)})
	found, err = SearchPRs(number, repo, Search{Text: "#482", State: "open"})
	if err != nil || len(found.Items) != 1 || found.Items[0].Author != "ghost" || *found.Total != 1 {
		t.Fatalf("a number did not find its pull request whatever the filters: %+v %v", found, err)
	}
	if _, err := SearchPRs(all, repo, Search{Scope: "theirs"}); err == nil {
		t.Fatal("an unknown scope was searched")
	}
}

func TestOnePullRequestIsReadInFull(t *testing.T) {
	host := fakeGH(
		[2]string{`"comments":"C1"`, answered(fixture(t, "detail-page2.json"), "", 0)},
		[2]string{"reviewRequests", answered(fixture(t, "reviewers.json"), "", 0)},
		[2]string{"reviewThreads", answered(fixture(t, "detail.json"), "", 0)},
	)
	d, err := ReadDetail(host, Repo{"octo", "trips"}, 482)
	if err != nil {
		t.Fatal(err)
	}
	if d.Number != 482 || d.HeadOid != "0123456789abcdef0123456789abcdef01234567" || d.MergeState != "BLOCKED" || !d.AutoAllowed || d.Viewer != "sam" || d.Repository != "octo/trips" {
		t.Fatalf("detail = %+v", d)
	}
	if strings.Join(d.MergeMethods, ",") != "merge,squash" {
		t.Fatalf("methods = %v", d.MergeMethods)
	}
	if len(d.Comments) != 2 || d.Comments[1].Body != "Second page" {
		t.Fatalf("the second page of comments was not read: %+v", d.Comments)
	}
	if len(d.Reviews) != 2 || d.Reviews[1].Author != "lee" {
		t.Fatalf("a pending review was shown, or one read twice: %+v", d.Reviews)
	}
	th := d.Threads[0]
	if len(d.Threads) != 2 || th.Line != 42 || !th.CanResolve || th.Comments[1].Author != "ghost" || !strings.HasPrefix(th.Comments[0].Hunk, "@@ -38,4") {
		t.Fatalf("threads = %+v", d.Threads)
	}
	if d.Threads[1].Line != 3 || !d.Threads[1].CanResolve || !d.Threads[1].Outdated {
		t.Fatalf("a resolved thread = %+v", d.Threads[1])
	}
	unit := d.Checks[0]
	if len(d.Checks) != 4 || unit.Run != 777 || unit.Job != 1229 || unit.Workflow != "CI" || unit.Result != "FAILURE" {
		t.Fatalf("checks = %+v", d.Checks)
	}
	if d.Checks[2].Job != 0 || d.Checks[3].Name != "ci/deploy" || d.Checks[3].Status != "PENDING" {
		t.Fatalf("another app's check run or a status: %+v", d.Checks[2:])
	}
	want := []Reviewer{{"priya", "CHANGES_REQUESTED", false}, {"noor", "APPROVED", false}, {"lee", "REQUESTED", false}, {"octo/mobile", "REQUESTED", true}}
	if len(d.Reviewers) != len(want) {
		t.Fatalf("reviewers = %+v", d.Reviewers)
	}
	for i := range want {
		if d.Reviewers[i] != want[i] {
			t.Fatalf("reviewer %d = %+v, want %+v", i, d.Reviewers[i], want[i])
		}
	}
	gone := fakeGH([2]string{"graphql", answered(`{"data":{"viewer":{"login":"sam"},"repository":{"nameWithOwner":"octo/trips","pullRequest":null}}}`, "", 0)})
	if _, err := ReadDetail(gone, Repo{"octo", "trips"}, 9); err != ErrNotFound {
		t.Fatalf("err = %v", err)
	}
	noReviewers := fakeGH(
		[2]string{"reviewRequests", answered(`{"data":null,"errors":[{"message":"Resource not accessible by integration"}]}`, "", 1)},
		[2]string{"reviewThreads", answered(strings.Replace(fixture(t, "detail.json"), `"hasNextPage":true`, `"hasNextPage":false`, 1), "", 0)},
	)
	d, err = ReadDetail(noReviewers, Repo{"octo", "trips"}, 482)
	if err != nil || !strings.Contains(d.ReviewersError, "not accessible") || len(d.Reviewers) != 0 {
		t.Fatalf("a reviewers' read that failed cost the view: %v %+v", err, d.ReviewersError)
	}
}

func TestAJobsStepsTheLogsEndAndTheFiles(t *testing.T) {
	host := fakeGH(
		[2]string{"actions/jobs/1229", answered(fixture(t, "job.json"), "", 0)},
		[2]string{"'--log-failed'", answered("unit\tRun tests\t2026-11-13T10:03:19.1234567Z \x1b[31mFAIL\x1b[0m src/offline/sync.test.ts\nunit\tRun tests\t2026-11-13T10:03:20.0000000Z Tests: 1 failed\n", "", 0)},
		[2]string{"pulls/482/files", answered(fixture(t, "files.json"), "", 0)},
	)
	repo := Repo{"octo", "trips"}
	job, err := ReadJob(host, repo, 1229)
	if err != nil || len(job.Steps) != 2 || job.Steps[1].Result != "failure" || job.URL == "" {
		t.Fatalf("job = %+v %v", job, err)
	}
	log, err := ReadLog(host, repo, 1229)
	if err != nil || log.Text != "FAIL src/offline/sync.test.ts\nTests: 1 failed" || log.Cut {
		t.Fatalf("log = %q %v", log.Text, err)
	}
	if s := host.Scripts()[1]; !strings.Contains(s, "gh 'run' 'view' '--job' '1229' '-R' 'octo/trips' '--log-failed'") || !strings.Contains(s, "tail -c 61440") {
		t.Fatalf("log script = %q", s)
	}
	big := CleanLog(strings.Repeat("x line of output\n", 2000))
	if !big.Cut || len(big.Text) > LogShown || strings.HasPrefix(big.Text, "line") {
		t.Fatalf("a long log is %d bytes, cut %v", len(big.Text), big.Cut)
	}
	files, err := ReadFiles(host, repo, 482)
	if err != nil || len(files.Files) != 3 || files.Cut {
		t.Fatalf("files = %+v %v", files, err)
	}
	if !files.Files[1].NoPatch || files.Files[2].Previous != "docs/cache.md" || !strings.HasPrefix(files.Files[0].Patch, "@@") {
		t.Fatalf("files = %+v", files.Files)
	}
	cut := ParseFiles(fixture(t, "files.json")[:300])
	if !cut.Cut || len(cut.Files) != 1 {
		t.Fatalf("a list cut on the machine: %+v", cut)
	}
}
