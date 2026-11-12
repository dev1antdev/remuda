package review

import "sort"

// Signals are what else is known of a folder's work beside its profile, for its risk.
type Signals struct {
	Secrets      int    // findings of the last secret check of this very work; 0 when it found none or none ran
	ChecksFailed int    // failed checks of the open pull request its agents named
	Conflicts    bool   // that request cannot merge as it is
	Changes      bool   // a reviewer asked for changes on it
	Collision    string // "lines" when another workspace of the repository changes the same lines, "file" for the same files only
	CollidesWith string // that workspace's name
}

// Reason is one thing that makes a review riskier, and the points it adds.
type Reason struct {
	Kind   string   `json:"kind"` // size | untested | migration | lockfile | ci | auth | secrets | checks | conflicts | changes | collision
	Points int      `json:"points"`
	Count  int      `json:"count,omitempty"` // files, findings or checks
	Lines  int      `json:"lines,omitempty"` // the change's size, for size
	Paths  []string `json:"paths,omitempty"` // the first MaxNamed of them
	Same   bool     `json:"same,omitempty"`  // a collision on the same lines, not only the same files
	With   string   `json:"with,omitempty"`  // the workspace a collision is with
}

// Risk is a review's score out of 100, its level, and every reason, the weightiest first.
type Risk struct {
	Score   int      `json:"score"`
	Level   string   `json:"level"` // low | medium | high
	Reasons []Reason `json:"reasons"`
}

// The weights of the rule, out of a score capped at 100: a secret about to leave weighs most, then what a merge
// would break — the same lines changed elsewhere, a red CI — then what is hard to review: a migration, auth, a big
// change, code with no test changed beside it.
const (
	pointsBig            = 25 // from bigLines lines or bigFiles files
	pointsMedium         = 10 // from mediumLines lines or mediumFiles files
	pointsUntestedEach   = 5
	pointsUntestedMost   = 20
	pointsMigration      = 15
	pointsAuth           = 15
	pointsCI             = 10
	pointsLockfile       = 5
	pointsSecrets        = 40
	pointsChecks         = 20
	pointsConflicts      = 15
	pointsChanges        = 10
	pointsCollisionLines = 20
	pointsCollisionFile  = 8

	bigLines, bigFiles       = 500, 25
	mediumLines, mediumFiles = 150, 10

	HighFrom, MediumFrom = 50, 20
)

var sensitivePoints = map[string]int{KindMigration: pointsMigration, KindAuth: pointsAuth, KindCI: pointsCI, KindLockfile: pointsLockfile}

// RiskOf scores a folder's work for the review queue: the reasons the profile and the signals give, their points
// summed and capped at 100, high from HighFrom, medium from MediumFrom.
func RiskOf(profile Profile, signals Signals) Risk {
	var reasons []Reason
	add := func(reason Reason) { reasons = append(reasons, reason) }
	lines := profile.Lines()
	switch {
	case lines >= bigLines || profile.Files >= bigFiles:
		add(Reason{Kind: "size", Points: pointsBig, Count: profile.Files, Lines: lines})
	case lines >= mediumLines || profile.Files >= mediumFiles:
		add(Reason{Kind: "size", Points: pointsMedium, Count: profile.Files, Lines: lines})
	}
	if profile.UntestedCount > 0 {
		add(Reason{Kind: "untested", Points: min(pointsUntestedMost, pointsUntestedEach*profile.UntestedCount), Count: profile.UntestedCount, Paths: profile.Untested})
	}
	for _, sensitive := range profile.Sensitive {
		add(Reason{Kind: sensitive.Kind, Points: sensitivePoints[sensitive.Kind], Count: sensitive.Count, Paths: sensitive.Paths})
	}
	if signals.Secrets > 0 {
		add(Reason{Kind: "secrets", Points: pointsSecrets, Count: signals.Secrets})
	}
	if signals.ChecksFailed > 0 {
		add(Reason{Kind: "checks", Points: pointsChecks, Count: signals.ChecksFailed})
	}
	if signals.Conflicts {
		add(Reason{Kind: "conflicts", Points: pointsConflicts})
	}
	if signals.Changes {
		add(Reason{Kind: "changes", Points: pointsChanges})
	}
	switch signals.Collision {
	case "lines":
		add(Reason{Kind: "collision", Points: pointsCollisionLines, Same: true, With: signals.CollidesWith})
	case "file":
		add(Reason{Kind: "collision", Points: pointsCollisionFile, With: signals.CollidesWith})
	}
	sort.SliceStable(reasons, func(i, j int) bool { return reasons[i].Points > reasons[j].Points })
	score := 0
	for _, reason := range reasons {
		score += reason.Points
	}
	score = min(score, 100)
	level := "low"
	switch {
	case score >= HighFrom:
		level = "high"
	case score >= MediumFrom:
		level = "medium"
	}
	if reasons == nil {
		reasons = []Reason{}
	}
	return Risk{Score: score, Level: level, Reasons: reasons}
}
