import { describe, expect, it } from "vitest";
import type { Machine, Pane, Session, Tab } from "./api";
import { DEFAULT_RADAR } from "./radar";
import { agentWords, branchShown, entries, folderCounts, itemTitle, itemWords, leadOf, representative, shortAge, since, type WorkspaceItem } from "./workspaces";

const now = 1_000_000_000_000;
const ago = (minutes: number) => now - minutes * 60_000;

let ids = 0;
const pane = (over: Partial<Pane> = {}): Pane => ({ id: `p${++ids}`, title: "", agent: "claude", cwd: "/x", focused: false, status: "idle", cols: 80, rows: 24, ...over }) as Pane;
const tab = (panes: Pane[], over: Partial<Tab> = {}): Tab => ({ id: `t${++ids}`, number: 1, label: "", named: false, status: "idle", panes, ...over }) as unknown as Tab;
const session = (name: string, tabs: Tab[], over: Partial<Session> = {}): Session => ({ id: `w${++ids}`, name, status: "idle", active_tab_id: tabs[0]?.id ?? "", tabs, ...over }) as Session;
const machine = (sessions: Session[]): Machine => ({ id: "gog", label: "GOG", state: "connected", sessions }) as unknown as Machine;
const repo = (name: string, linked: boolean) => ({ repo_key: `/p/${name}/.git`, repo_name: name, linked, checkout_path: `/p/${name}` });

describe("entries", () => {
  const fold = (list: ReturnType<typeof entries>) => list.map((e) => (e.kind === "folder" ? `${e.folder.name}: ${e.folder.items.map((i) => i.session.name).join(", ")}` : e.item.session.name));

  it("makes every repository a folder with its workspaces, the main checkout first, the liveliest folder first", () => {
    const main = session("user-profiles", [tab([pane({ radar: { state: "idle", last_active: ago(60) } })])], { folder: "/p/user-profiles", dir: "/p/user-profiles", worktree: repo("user-profiles", false), branch: "main" });
    const tree = session("small-improvements", [tab([pane({ radar: { state: "working", since: ago(3) } })])], { folder: "/p/user-profiles", dir: "/w/small-improvements", worktree: repo("user-profiles", true) });
    const games = session("games-db", [tab([pane({ radar: { state: "done", last_active: ago(5) } })])], { folder: "/p/games-db", dir: "/p/games-db" });
    const quiet = session("notes", [tab([pane({ agent: "" })])], { folder: "/p/notes", dir: "/p/notes" });
    expect(fold(entries(machine([quiet, games, tree, main]), DEFAULT_RADAR))).toEqual(["user-profiles: user-profiles, small-improvements", "games-db: games-db", "notes: notes"]);
  });

  it("gathers two workspaces on one folder, and keeps herdr's order when asked", () => {
    const a = session("api", [tab([pane({ radar: { state: "idle", last_active: ago(30) } })])], { folder: "/p/backend", dir: "/p/backend" });
    const b = session("backend", [tab([pane({ radar: { state: "working" } })])], { folder: "/p/backend", dir: "/p/backend" });
    const c = session("web", [tab([pane({ radar: { state: "working" } })])], { folder: "/p/web", dir: "/p/web" });
    expect(fold(entries(machine([a, c, b]), DEFAULT_RADAR))).toEqual(["backend: backend, api", "web: web"]);
    expect(fold(entries(machine([a, c, b]), { ...DEFAULT_RADAR, order: "herdr" }))).toEqual(["backend: api, backend", "web: web"]);
  });

  it("takes the folder's icon from any of its workspaces", () => {
    const a = session("api", [tab([pane()])], { folder: "/p/backend", dir: "/p/backend" });
    const b = session("backend", [tab([pane()])], { folder: "/p/backend", dir: "/p/backend", icon: { emoji: "🦊" } });
    const [entry] = entries(machine([a, b]), DEFAULT_RADAR);
    expect(entry?.kind === "folder" && entry.folder.icon).toEqual({ emoji: "🦊" });
  });

  it("lists every workspace by activity in the flat order", () => {
    const main = session("user-profiles", [tab([pane({ radar: { state: "idle", last_active: ago(60) } })])], { folder: "/p/user-profiles", worktree: repo("user-profiles", false) });
    const tree = session("small-improvements", [tab([pane({ radar: { state: "working" } })])], { folder: "/p/user-profiles", worktree: repo("user-profiles", true) });
    const games = session("games-db", [tab([pane({ radar: { state: "done", last_active: ago(5) } })])]);
    expect(fold(entries(machine([main, tree, games]), { ...DEFAULT_RADAR, order: "recent" }))).toEqual(["small-improvements", "games-db", "user-profiles"]);
  });
});

describe("folderCounts", () => {
  it("counts a folder's agents by state and its fresh files", () => {
    const a = session("a", [tab([pane({ radar: { state: "working" } }), pane({ agent: "" })])], { folder: "/p/x", review: { changed: 3, fresh: 2 } });
    const b = session("b", [tab([pane({ radar: { state: "done" } })]), tab([pane({ radar: { state: "blocked" } })])], { folder: "/p/x", review: { changed: 1, fresh: 1 } });
    const [entry] = entries(machine([a, b]), { ...DEFAULT_RADAR, shells: true });
    expect(entry?.kind === "folder" && folderCounts(entry.folder)).toEqual({ working: 1, blocked: 1, done: 1, agents: 3, fresh: 3 });
  });
});

describe("leadOf", () => {
  it("picks a question before a note, work, a finish and the rest; a shell never leads an agent", () => {
    const shell = pane({ agent: "" });
    const idle = pane({ radar: { state: "idle", last_active: ago(1) } });
    const working = pane({ radar: { state: "working" } });
    const asking = pane({ radar: { state: "blocked" } });
    const noted = pane({ radar: { state: "working", alert: "stuck for 20 min" } });
    const rows = (...panes: Pane[]) => panes.map((p, index) => ({ session: session("s", []), tab: tab([]), pane: p, last: 0, head: true, index }));
    expect(leadOf(rows(shell, idle, working))?.pane).toBe(working);
    expect(leadOf(rows(working, noted))?.pane).toBe(noted);
    expect(leadOf(rows(noted, asking))?.pane).toBe(asking);
    expect(leadOf(rows(shell, idle))?.pane).toBe(idle);
    expect(leadOf(rows(shell))?.pane).toBe(shell);
    expect(leadOf([])).toBeNull();
  });
});

describe("the line's words", () => {
  const item = (s: Session, prefs = { ...DEFAULT_RADAR, shells: true }): WorkspaceItem => entries(machine([s]), prefs).flatMap((e) => (e.kind === "workspace" ? [e.item] : e.folder.items))[0]!;

  it("names a workspace called as its folder by its branch inside the folder", () => {
    const main = item(session("user-profiles", [tab([pane()])], { branch: "msn-2448" }));
    expect(itemTitle(main)).toBe("user-profiles");
    expect(itemTitle(main, "user-profiles")).toBe("msn-2448");
    expect(itemTitle(main, "elsewhere")).toBe("user-profiles");
    expect(itemTitle(item(session("notes", [tab([pane()])])), "notes")).toBe("notes");
  });

  it("shows the branch unless the title already says it", () => {
    const tree = item(session("small-improvements", [tab([pane()])], { branch: "small-improvements" }));
    expect(branchShown(tree, "small-improvements")).toBe("");
    const main = item(session("api", [tab([pane()])], { branch: "feature/x" }));
    expect(branchShown(main, "api")).toBe("feature/x");
  });

  it("says what the lead agent does, how many run, or the shells' tabs", () => {
    const asking = item(session("a", [tab([pane({ radar: { state: "blocked" } })])]));
    expect(itemWords(asking, DEFAULT_RADAR)).toBe("waiting for your answer");
    const finished = item(session("b", [tab([pane({ title: "Fix the queue", said: "Done: the queue drains", radar: { state: "done" } })])]));
    expect(itemWords(finished, DEFAULT_RADAR)).toBe("Done: the queue drains");
    const working = item(session("c", [tab([pane({ title: "Fix the queue", said: "old words", radar: { state: "working" } })])]));
    expect(itemWords(working, DEFAULT_RADAR)).toBe("Fix the queue");
    const two = item(session("d", [tab([pane()]), tab([pane({ agent: "codex" })]), tab([pane({ agent: "" })])]));
    expect(itemWords(two, DEFAULT_RADAR)).toBe("2 agents · 1 shell");
    expect(itemWords(item(session("e", [tab([pane({ agent: "" })]), tab([pane({ agent: "" })])]), DEFAULT_RADAR), DEFAULT_RADAR)).toBe("2 tabs");
    const noted = item(session("f", [tab([pane({ radar: { state: "working", alert: "the same call six times" } })])]));
    expect(agentWords(noted.lead!, DEFAULT_RADAR)).toBe("the same call six times");
  });
});

describe("the line's age", () => {
  it("counts work and questions from their start, the rest from the last turn", () => {
    const row = (radar: Pane["radar"]) => ({ session: session("s", []), tab: tab([]), pane: pane({ radar }), last: 0, head: true, index: 0 });
    expect(since(row({ state: "working", since: ago(3), last_active: ago(1) }))).toBe(ago(3));
    expect(since(row({ state: "done", since: ago(3), last_active: ago(1) }))).toBe(ago(1));
    expect(since(row(undefined))).toBeUndefined();
    expect(since(null)).toBeUndefined();
  });

  it("reads at a glance", () => {
    expect(shortAge(undefined, now)).toBe("");
    expect(shortAge(ago(0.5), now)).toBe("now");
    expect(shortAge(ago(12), now)).toBe("12m");
    expect(shortAge(ago(60 * 5 + 10), now)).toBe("5h");
    expect(shortAge(ago(60 * 24 * 3), now)).toBe("3d");
    expect(shortAge(now + 60_000, now)).toBe("now");
  });
});

describe("representative", () => {
  it("is the active tab's focused pane, else its first", () => {
    const a = pane({ agent: "" });
    const b = pane({ agent: "", focused: true });
    const first = tab([pane()]);
    const active = tab([a, b]);
    expect(representative(session("s", [first, active], { active_tab_id: active.id }))).toBe(b);
    expect(representative(session("s", [tab([a])]))).toBe(a);
    expect(representative(session("s", []))).toBeNull();
  });
});
