import { describe, expect, it } from "vitest";
import type { PullRequest, Session } from "./api";
import { cardIssues, detailsOf, freshnessOf, freshWords, noteBytes, noteDir, noteTip, portsWords } from "./cardFacts";
import type { CardPR } from "./cardPR";

const at = (minutesAgo: number, now: number) => new Date(now - minutesAgo * 60_000).toISOString();
const NOW = Date.parse("2026-11-13T12:00:00Z");
const read = (minutesAgo: number, over: Partial<PullRequest> = {}): CardPR => ({
  paneId: "p1",
  pr: { url: "https://github.com/acme/storefront/pull/212", label: "PR #212", status: { state: "OPEN", draft: false, mergeable: "MERGEABLE", review: "", checks: { total: 0, pass: 0, fail: 0, pending: 0, skipped: 0 }, at: at(minutesAgo, NOW) }, ...over },
});

describe("how fresh the card's requests are", () => {
  it("says a read is under way first, then a failed one with how old the state shown is, else when the site answered", () => {
    expect(freshWords(freshnessOf([read(3, { reading: true })]), NOW)).toBe("Refreshing from GitHub…");
    expect(freshWords(freshnessOf([read(5, { error: "error connecting to api.github.com" })]), NOW)).toBe("Couldn't reach GitHub · from 5 min ago");
    expect(freshWords(freshnessOf([{ paneId: "p1", pr: { url: "https://github.com/a/b/pull/1", label: "PR #1", error: "gh: not found" } }]), NOW)).toBe("Couldn't reach GitHub");
    expect(freshWords(freshnessOf([read(3), read(9)]), NOW)).toBe("Updated 9 min ago");
    expect(freshWords(freshnessOf([read(0)]), NOW)).toBe("Updated just now");
    expect(freshWords(freshnessOf([]), NOW)).toBe("");
  });

  it("names GitLab for a merge request", () => {
    const mr: CardPR = { paneId: "p1", pr: { url: "https://gitlab.com/t/api/-/merge_requests/9", label: "MR !9", reading: true } };
    expect(freshWords(freshnessOf([mr]), NOW)).toBe("Refreshing from GitLab…");
  });
});

describe("the card's other sections", () => {
  it("lists the Jira issues the work names and the GitHub issues its requests close, each once", () => {
    const closes = [{ number: 198, url: "https://github.com/acme/storefront/issues/198", repo: "acme/storefront" }];
    const prs = [read(1, { status: { ...read(1).pr.status!, closes } }), { ...read(1, { status: { ...read(1).pr.status!, closes } }), paneId: "p2" }];
    const issues = cardIssues([{ key: "APP-12", summary: "Draw every screen", status: "In Review", category: "indeterminate", url: "https://example.atlassian.net/browse/APP-12" }], prs);
    expect(issues.map((i) => [i.key, i.where, i.tone])).toEqual([
      ["APP-12", "Jira", "blue"],
      ["#198", "acme/storefront", ""],
    ]);
    expect(issues[1]?.title).toBe("Closed by PR #212 once it merges");
  });

  it("says the ports a checkout serves", () => {
    const port = (n: number) => ({ port: n, process: "node", pid: 10 + n, address: `localhost:${n}` });
    expect(portsWords([port(3000), port(5432)])).toBe("2 live ports: 3000, 5432");
    expect(portsWords([port(3000)])).toBe("1 live port: 3000");
  });

  it("keeps a note's tooltip to a readable length and counts its bytes as the server does", () => {
    expect(noteTip("short")).toBe("short");
    const long = "word ".repeat(200);
    expect(noteTip(long).length).toBeLessThanOrEqual(601);
    expect(noteTip(long).endsWith("…")).toBe(true);
    expect(noteBytes("  ж  ")).toBe(2);
  });

  it("opens a card only for a workspace with something to show, and never for one the screen-sharing mode hides", () => {
    const session: Session = { id: "w1", name: "app", status: "idle", active_tab_id: "t1", dir: "/w/app", tabs: [] };
    expect(detailsOf(session, undefined, false).any).toBe(false);
    expect(detailsOf({ ...session, note: "remember" }, undefined, false).any).toBe(true);
    expect(detailsOf({ ...session, ports: [{ port: 3000, process: "node", pid: 1, address: "localhost:3000" }] }, undefined, false).any).toBe(true);
    expect(detailsOf({ ...session, note: "remember" }, undefined, true)).toEqual({ note: "", prs: [], issues: [], ports: [], any: false });
    expect(noteDir(session)).toBe("/w/app");
    expect(noteDir({ ...session, dir: undefined, worktree: { repo_key: "", repo_name: "", linked: true, checkout_path: "/w/tree" } })).toBe("/w/tree");
  });
});
