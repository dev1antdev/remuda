package recap

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// DigestLines is how many agents the body of a digest names before "+N more".
const DigestLines = 4

// Digest is the standup as one notification: a title with the counts ("Standup: 3 finished, 1 waiting, $4.20") and a
// body with a line per agent that did something, the ones waiting for the person first; empty when nobody did anything.
func Digest(items []Item, since, now time.Time) (title, body string, empty bool) {
	var busy []Item
	finished, waiting, working := 0, 0, 0
	cost := 0.0
	for _, item := range items {
		if item.Error != "" || item.Recap.Quiet() {
			continue
		}
		busy = append(busy, item)
		switch {
		case item.State == "working":
			working++
		case item.Recap.Answers > 0:
			finished++
		}
		if item.Alert != "" || item.State == "blocked" {
			waiting++
		}
		cost += item.Recap.Cost
	}
	if len(busy) == 0 {
		return "Standup: nothing to report", "", true
	}
	var parts []string
	if finished > 0 {
		parts = append(parts, fmt.Sprintf("%d finished", finished))
	}
	if waiting > 0 {
		parts = append(parts, fmt.Sprintf("%d waiting", waiting))
	}
	if working > 0 {
		parts = append(parts, fmt.Sprintf("%d working", working))
	}
	if cost > 0 {
		parts = append(parts, usd(cost))
	}
	if len(parts) == 0 {
		parts = append(parts, plural(len(busy), "agent")+" active")
	}
	title = "Standup: " + strings.Join(parts, ", ")
	sort.SliceStable(busy, func(i, j int) bool {
		if ri, rj := digestRank(busy[i]), digestRank(busy[j]); ri != rj {
			return ri < rj
		}
		return busy[i].Recap.Cost > busy[j].Recap.Cost
	})
	lines := make([]string, 0, DigestLines+1)
	for i, item := range busy {
		if i == DigestLines {
			lines = append(lines, fmt.Sprintf("+%d more", len(busy)-DigestLines))
			break
		}
		lines = append(lines, digestLine(item))
	}
	return title, strings.Join(lines, "\n"), false
}

// digestRank orders the body as the standup does: the agents that need the person, the finished, the working, the rest.
func digestRank(item Item) int {
	if item.Alert != "" {
		return 0
	}
	switch item.State {
	case "blocked":
		return 0
	case "done":
		return 1
	case "working":
		return 2
	}
	return 3
}

// digestLine is one agent's line: where it is and what it last said, else its figures.
func digestLine(item Item) string {
	where := item.Workspace
	if item.Machine != "local" && item.Label != "" {
		where += " on " + item.Label
	}
	said := item.Recap.Said
	if said == "" {
		said = Numbers(item.Recap)
	}
	return where + ": " + oneLine(said, 80)
}
