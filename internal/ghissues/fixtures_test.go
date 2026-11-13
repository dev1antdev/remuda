package ghissues

// GitHub's answers as its documentation shapes them, written for these tests: an anonymous repository acme/widgets,
// people named alice, bob and carol, nothing read from GitHub itself.

const listJSON = `[
  {"number": 41, "title": "Crash on start", "author": {"login": "bob", "is_bot": false}, "state": "OPEN", "stateReason": "",
   "labels": [{"name": "bug", "color": "d73a4a", "description": "Something isn't working"}], "assignees": [{"login": "alice"}],
   "updatedAt": "2026-11-13T09:00:00Z", "url": "https://github.com/acme/widgets/issues/41"},
  {"number": 42, "title": "Dark mode", "author": {"login": "app/renovate", "is_bot": true}, "state": "CLOSED", "stateReason": "NOT_PLANNED",
   "labels": [], "assignees": [], "updatedAt": "2026-11-13T10:00:00Z", "url": "https://github.com/acme/widgets/issues/42"},
  {"number": 40, "title": "Docs", "author": {"login": "carol"}, "state": "CLOSED", "stateReason": "COMPLETED",
   "labels": [{"name": "docs", "color": "#0075CA"}, {"name": "odd", "color": "nothex"}], "assignees": [],
   "updatedAt": "2026-11-12T10:00:00Z", "url": "https://github.com/acme/widgets/issues/40"}
]`

const issueJSON = `{
  "number": 41, "title": "Crash on start", "body": "It crashes. See #40.", "html_url": "https://github.com/acme/widgets/issues/41",
  "user": {"login": "bob", "type": "User"}, "state": "open", "state_reason": null, "author_association": "CONTRIBUTOR",
  "labels": [{"name": "bug", "color": "d73a4a", "description": "Something isn't working"}],
  "assignees": [{"login": "alice", "type": "User"}], "milestone": {"title": "v1.2", "html_url": "https://github.com/acme/widgets/milestone/3"},
  "comments": 2, "created_at": "2026-11-10T08:00:00Z", "updated_at": "2026-11-13T09:00:00Z", "closed_at": null,
  "reactions": {"url": "https://api.github.com/repos/acme/widgets/issues/41/reactions", "total_count": 3, "+1": 2, "-1": 0, "laugh": 0, "hooray": 0, "confused": 0, "heart": 1, "rocket": 0, "eyes": 0}
}`

const pullJSON = `{"number": 43, "title": "Fix the crash", "html_url": "https://github.com/acme/widgets/pull/43", "user": {"login": "alice"},
  "state": "open", "labels": [], "assignees": [], "created_at": "2026-11-10T08:00:00Z", "updated_at": "2026-11-13T09:00:00Z",
  "pull_request": {"url": "https://api.github.com/repos/acme/widgets/pulls/43", "merged_at": null}}`

// timelineJSON is two pages, as gh api --paginate prints them one after the other.
const timelineJSON = `[
  {"event": "labeled", "actor": {"login": "alice"}, "created_at": "2026-11-10T09:00:00Z", "label": {"name": "bug", "color": "d73a4a"}},
  {"event": "labeled", "actor": {"login": "alice"}, "created_at": "2026-11-10T09:00:20Z", "label": {"name": "p1", "color": "b60205"}},
  {"event": "unlabeled", "actor": {"login": "alice"}, "created_at": "2026-11-10T09:00:40Z", "label": {"name": "triage", "color": "ededed"}},
  {"event": "labeled", "actor": {"login": "bob"}, "created_at": "2026-11-10T09:00:50Z", "label": {"name": "help", "color": "008672"}},
  {"event": "commented", "id": 1, "user": {"login": "carol", "type": "User"}, "actor": {"login": "carol"}, "created_at": "2026-11-10T10:00:00Z",
   "body": "I see it too.", "author_association": "MEMBER", "html_url": "https://github.com/acme/widgets/issues/41#issuecomment-1",
   "reactions": {"total_count": 1, "+1": 0, "eyes": 1}},
  {"event": "renamed", "actor": {"login": "bob"}, "created_at": "2026-11-11T08:00:00Z", "rename": {"from": "Crash", "to": "Crash on start"}},
  {"event": "assigned", "actor": {"login": "alice"}, "assignee": {"login": "alice"}, "created_at": "2026-11-11T09:00:00Z"},
  {"event": "subscribed", "actor": {"login": "alice"}, "created_at": "2026-11-11T09:00:01Z"}
][
  {"event": "cross-referenced", "actor": {"login": "alice"}, "created_at": "2026-11-12T08:00:00Z",
   "source": {"type": "issue", "issue": {"number": 43, "title": "Fix the crash", "state": "open", "html_url": "https://github.com/acme/widgets/pull/43",
     "pull_request": {"merged_at": null}, "repository": {"full_name": "acme/widgets"}}}},
  {"event": "referenced", "actor": {"login": "alice"}, "created_at": "2026-11-12T09:00:00Z", "commit_id": "0123456789abcdef0123456789abcdef01234567",
   "commit_url": "https://api.github.com/repos/acme/widgets/commits/0123456789abcdef0123456789abcdef01234567"},
  {"event": "closed", "actor": {"login": "dependabot[bot]"}, "created_at": "2026-11-12T10:00:00Z", "state_reason": "not_planned"},
  {"event": "reopened", "actor": {"login": "bob"}, "created_at": "2026-11-12T11:00:00Z"},
  {"event": "milestoned", "actor": {"login": "bob"}, "created_at": "2026-11-12T12:00:00Z", "milestone": {"title": "v1.2"}}
]`

const closedByJSON = `{"data": {"repository": {"issue": {"closedByPullRequestsReferences": {"nodes": [
  {"number": 43, "title": "Fix the crash", "url": "https://github.com/acme/widgets/pull/43", "state": "OPEN", "isDraft": true, "repository": {"nameWithOwner": "acme/widgets"}}
]}}}}}`

const closedByRefused = `{"errors": [{"type": "undefinedField", "message": "Field 'closedByPullRequestsReferences' doesn't exist on type 'Issue'"}]}`

const headers200 = "HTTP/2.0 200 OK\nContent-Type: application/json; charset=utf-8\nEtag: W/\"aaa111\"\nX-Github-Request-Id: 1\n"

const headers304 = "HTTP/2.0 304 Not Modified\nEtag: W/\"aaa111\"\n"
