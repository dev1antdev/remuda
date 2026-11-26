package usage

import (
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/transcript"
)

func TestOpencodeAnswersJoinTheReport(t *testing.T) {
	loc := time.UTC
	at := time.Date(2026, 11, 26, 10, 0, 0, 0, loc)
	b := newBuilder(at.Add(-24*time.Hour), loc)
	b.addOpencode([]transcript.OpencodeResponse{
		{MessageID: "m1", Session: "ses_1", Dir: "/code/app", Model: "deepseek-v4.1-flash", At: at, Input: 100, Output: 10, CacheRead: 200, Cost: 1.5},
		{MessageID: "m2", Session: "ses_1", Dir: "/code/app", Model: "deepseek-v4.1-flash", At: at.Add(time.Minute), Input: 50, Output: 5, CacheRead: 100},
	})
	rep := b.report()
	if rep.Total.Turns != 2 || rep.Total.Tokens.Input != 150 || rep.Total.Tokens.Output != 15 || rep.Total.Tokens.CacheRead != 300 || rep.Total.Cost != 1.5 || rep.Total.Unknown != 1 {
		t.Fatalf("total: %+v", rep.Total)
	}
	if len(rep.Days) != 1 || rep.Days[0].Date != "2026-11-26" || rep.Days[0].Turns != 2 {
		t.Fatalf("days: %+v", rep.Days)
	}
	if len(rep.Projects) != 1 || rep.Projects[0].Project != "app" || rep.Projects[0].Sessions != 1 {
		t.Fatalf("projects: %+v", rep.Projects)
	}
	if len(rep.Sessions) != 1 || rep.Sessions[0].Session != "ses_1" || rep.Sessions[0].First != "2026-11-26T10:00:00Z" || rep.Sessions[0].Last != "2026-11-26T10:01:00Z" {
		t.Fatalf("sessions: %+v", rep.Sessions)
	}
	merged := mergeReports(Report{Days: []DayTotals{{Date: "2026-11-26", Totals: Totals{Turns: 3}}}}, rep)
	if len(merged.Days) != 1 || merged.Days[0].Turns != 5 || merged.Total.Turns != 2 {
		t.Fatalf("merged: %+v", merged)
	}
}
