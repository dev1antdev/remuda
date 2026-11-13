import guardGo from "../../../../internal/prstatus/guard.go?raw";
import { describe, expect, it } from "vitest";
import { authorsOf, entries, fingerprint, hunkEnd, markSeen, MAX_SEEN, unseen } from "./conversation";
import { COMMENT_PROMPT, commentMessage, GUARD, unresolvedMessage } from "./messages";
import { filesURL, isPullURL, prFromURL } from "./links";
import { agentsOn, fileTree, fileTotals } from "./tree";
import type { Machine } from "@/features/roster/api";
import type { Detail, PRFile } from "./api";

const sample = (over: Partial<Detail>): Detail =>
  ({
    number: 482,
    url: "https://github.com/octo/trips/pull/482",
    head: "offline-trips",
    base: "main",
    head_oid: "0123456789abcdef0123456789abcdef01234567",
    comments: [],
    reviews: [],
    threads: [
      { id: "T1", path: "src/sync.ts", line: 42, resolved: false, outdated: false, can_resolve: true, can_reply: true, comments: [{ id: "C1", author: "priya", body: "Retry?", url: "u", at: "2026-11-13T10:00:00Z" }] },
      { id: "T2", path: "README.md", line: 3, resolved: true, outdated: false, can_resolve: true, can_reply: true, comments: [{ id: "C2", author: "noor", body: "Typo", url: "u", at: "2026-11-12T10:00:00Z" }] },
    ],
    ...over,
  }) as Detail;

const pr = sample({
  comments: [{ id: "IC1", author: "noor", body: "Nice.", url: "https://github.com/octo/trips/pull/482#issuecomment-1", at: "2026-11-13T09:00:00Z" }],
  reviews: [
    { id: "R1", author: "priya", body: "The sync needs a retry.", url: "u", at: "2026-11-13T10:00:00Z", state: "CHANGES_REQUESTED" },
    { id: "R2", author: "lee", body: "", url: "u", at: "2026-11-13T10:05:00Z", state: "COMMENTED" },
    { id: "R3", author: "kim", body: "", url: "u", at: "2026-11-13T10:06:00Z", state: "APPROVED" },
  ],
});

describe("the conversation", () => {
  it("lays everything said out in time, a silent comment-only review left out", () => {
    expect(entries(pr).map((e) => [e.id, e.kind])).toEqual([
      ["T2", "Resolved"],
      ["IC1", "Comment"],
      ["R1", "Changes requested"],
      ["T1", "Unresolved"],
      ["R3", "Approved"],
    ]);
    expect(entries(pr, { open: true }).map((e) => e.id)).toEqual(["T1"]);
    expect(entries(pr, { author: "noor" }).map((e) => e.id)).toEqual(["T2", "IC1"]);
    expect(authorsOf(pr)).toEqual(["kim", "noor", "priya"]);
  });
  it("tells a new comment, and an edited one, from one seen", () => {
    const all = unseen(pr, []);
    expect([...all].sort()).toEqual(["C1", "C2", "IC1", "R1", "R3"]);
    const seen = markSeen(pr, []);
    expect(unseen(pr, seen).size).toBe(0);
    const edited = sample({ ...pr, comments: [{ ...pr.comments[0]!, body: "Nice, really." }] });
    expect([...unseen(edited, seen)]).toEqual(["IC1"]);
    expect(fingerprint({ id: "x", body: "a" })).not.toBe(fingerprint({ id: "x", body: "b" }));
    expect(markSeen(pr, Array.from({ length: MAX_SEEN }, (_, i) => `old${i}`)).length).toBe(MAX_SEEN);
  });
  it("keeps a long hunk's head line and its end", () => {
    const hunk = ["@@ -1,20 +1,20 @@", ...Array.from({ length: 20 }, (_, i) => ` line ${i}`)].join("\n");
    const end = hunkEnd(hunk).split("\n");
    expect(end[0]).toBe("@@ -1,20 +1,20 @@");
    expect(end.at(-1)).toBe(" line 19");
    expect(end.length).toBe(9);
  });
});

describe("what goes to an agent", () => {
  it("ends with the guard the server's hand-offs end with", () => {
    const go: string = guardGo;
    expect(go).toContain(`const Guard = ${JSON.stringify(GUARD)}`);
  });
  it("hands the unresolved threads over at once, numbered", () => {
    const text = unresolvedMessage(pr, pr.threads);
    expect(text.startsWith("PR #482 (https://github.com/octo/trips/pull/482) has 1 unresolved review thread, on offline-trips at 0123456.")).toBe(true);
    expect(text).toContain("1. src/sync.ts:42 (u)\n   priya: Retry?");
    expect(text).not.toContain("README.md");
    expect(text.endsWith(GUARD)).toBe(true);
  });
  it("hands one comment over under the person's prompt", () => {
    const thread = entries(pr).find((e) => e.id === "T1")!;
    const text = commentMessage(pr, thread);
    expect(text.startsWith(COMMENT_PROMPT)).toBe(true);
    expect(text).toContain("About PR #482 (https://github.com/octo/trips/pull/482), offline-trips at 0123456 into main.");
    expect(text.endsWith(GUARD)).toBe(true);
    const comment = commentMessage(pr, entries(pr).find((e) => e.id === "IC1")!, "Reply to noor.");
    expect(comment).toContain("noor (https://github.com/octo/trips/pull/482#issuecomment-1):\nNice.");
    expect(comment.startsWith("Reply to noor.")).toBe(true);
  });
});

describe("links, files and agents", () => {
  it("reads a pull request's address", () => {
    expect(prFromURL("https://github.com/octo/trips/pull/482/files#diff")).toEqual({ repo: "octo/trips", number: 482 });
    expect(isPullURL("https://github.com/octo/trips/issues/3")).toBe(false);
    expect(isPullURL("https://gitlab.com/o/r/-/merge_requests/3")).toBe(false);
    expect(prFromURL("https://github.com/../x/pull/1")).toBeNull();
    expect(filesURL("https://github.com/octo/trips/pull/482/")).toBe("https://github.com/octo/trips/pull/482/files");
  });
  it("lays the files out as a tree, folders first, a closed one's rows left out", () => {
    const f = (path: string, additions = 1, deletions = 0): PRFile => ({ path, status: "modified", additions, deletions });
    const files = [f("README.md"), f("src/offline/sync.ts", 40, 4), f("src/app.ts", 2, 1), f("docs/a.md")];
    expect(fileTree(files).map((r) => (r.kind === "folder" ? `${r.depth}/${r.name}/` : `${r.depth}/${r.name}`))).toEqual(["0/docs/", "1/a.md", "0/src/", "1/offline/", "2/sync.ts", "1/app.ts", "0/README.md"]);
    expect(fileTree(files, new Set(["src"])).map((r) => r.name)).toEqual(["docs", "a.md", "src", "README.md"]);
    expect(fileTotals(files)).toEqual({ additions: 44, deletions: 5 });
  });
  it("finds the agents on the pull request: its branch on the panel's machine, or the link they named", () => {
    const pane = (id: string, over: object = {}) => ({ id, title: "", agent: "claude", cwd: "/w", focused: false, status: "idle", cols: 80, rows: 24, ...over });
    const session = (name: string, branch: string, panes: object[]) => ({ id: name, name, status: "idle", active_tab_id: "t", branch, tabs: [{ id: "t", label: "t", panes }] });
    const machines = [
      { id: "local", label: "Mac", state: "connected", sessions: [session("trips", "offline-trips", [pane("p1", { radar: { state: "working" } }), pane("p2", { agent: "" })]), session("other", "main", [pane("p3")])] },
      { id: "gog", label: "GOG", state: "connected", sessions: [session("trips", "offline-trips", [pane("p4")]), session("x", "y", [pane("p5", { pr: { url: "https://github.com/octo/trips/pull/482", label: "PR #482" } })])] },
    ] as unknown as Machine[];
    expect(agentsOn(machines, "local", { url: "https://github.com/octo/trips/pull/482", head: "offline-trips" }).map((a) => `${a.machineId}|${a.paneId}|${a.state}`)).toEqual(["local|p1|working", "gog|p5|idle"]);
  });
});
