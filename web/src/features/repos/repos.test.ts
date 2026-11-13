import { describe, expect, it } from "vitest";
import type { Session } from "@/features/roster/api";
import type { Listed, Repo } from "./api";
import { fetchable, groupsOf, machineWords, paneOf, replaced, scanWords, syncLabel, syncOf, syncOffers, syncTip, workspaceOn } from "./repos";

const repo = (over: Partial<Repo> = {}): Repo => ({ path: "/code/app", name: "app", parent: "~/code", ahead: 0, behind: 0, dirty: 0, remote: true, branch: "main", upstream: "origin/main", ...over });

describe("syncOf", () => {
  it("says … while a row is read, never 0", () => {
    expect(syncOf(null)).toBe("reading");
    expect(syncOf(repo({ ahead: 3 }), true)).toBe("reading");
    expect(syncLabel("reading")).toBe("…");
  });

  it("reads each of Manygit's states", () => {
    expect(syncOf(repo())).toBe("ok");
    expect(syncOf(repo({ ahead: 2 }))).toBe("ahead");
    expect(syncOf(repo({ behind: 1 }))).toBe("behind");
    expect(syncOf(repo({ ahead: 1, behind: 1 }))).toBe("diverged");
    expect(syncOf(repo({ upstream: undefined }))).toBe("unpushed");
    expect(syncOf(repo({ upstream: undefined, remote: false }))).toBe("local");
    expect(syncOf(repo({ gone: true }))).toBe("gone");
    expect(syncOf(repo({ detached: true, branch: undefined }))).toBe("detached");
    expect(syncOf(repo({ error: "fatal: detected dubious ownership" }))).toBe("error");
    expect(syncOf(repo(), false, "fatal: could not read Username")).toBe("failed");
  });

  it("words the states with no counts, and says the counts in a sentence", () => {
    expect(syncLabel("unpushed")).toBe("not pushed");
    expect(syncLabel("local")).toBe("no remote");
    expect(syncLabel("gone")).toBe("upstream gone");
    expect(syncLabel("ahead")).toBe("");
    expect(syncTip(repo({ behind: 3 }), "behind")).toBe("3 not pulled from origin/main");
    expect(syncTip(repo({ ahead: 1 }), "ahead")).toBe("1 commit not pushed to origin/main");
  });
});

describe("syncOffers", () => {
  const order = (r: Repo) => syncOffers(r).map((o) => o.action);

  it("puts first what the row's arrows suggest", () => {
    expect(order(repo({ behind: 2 }))).toEqual(["pull", "fetch", "push"]);
    expect(order(repo({ ahead: 2 }))).toEqual(["push", "fetch", "pull"]);
    expect(order(repo({ ahead: 1, behind: 1 }))).toEqual(["fetch", "pull", "push"]);
    expect(order(repo())).toEqual(["fetch", "pull", "push"]);
  });

  it("offers only what can run", () => {
    expect(order(repo({ upstream: undefined }))).toEqual(["push", "fetch"]);
    expect(syncOffers(repo({ upstream: undefined }))[0]!.label).toBe("Push and set upstream");
    expect(order(repo({ gone: true }))).toEqual(["fetch", "push"]);
    expect(order(repo({ detached: true, branch: undefined }))).toEqual(["fetch"]);
    expect(order(repo({ remote: false, upstream: undefined }))).toEqual([]);
    expect(order(repo({ error: "fatal" }))).toEqual([]);
    expect(fetchable(repo({ remote: false }))).toBe(false);
  });
});

describe("the machines' parts", () => {
  it("groups by the folder in the order given", () => {
    const groups = groupsOf([repo({ name: "a" }), repo({ name: "b" }), repo({ name: "c", parent: "~/work" })]);
    expect(groups.map((g) => [g.parent, g.repos.map((r) => r.name)])).toEqual([
      ["~/code", ["a", "b"]],
      ["~/work", ["c"]],
    ]);
  });

  it("says how a machine stands and what it scans", () => {
    const listed: Listed = { id: "gog", label: "GOG", repos: [repo(), repo({ path: "/code/b" })], scan: { folder: "/home/me/code", depth: 3 } };
    expect(machineWords(listed)).toBe("2 repositories");
    expect(machineWords(undefined)).toBe("…");
    expect(machineWords({ ...listed, away: "connecting" })).toBe("connecting…");
    expect(machineWords({ ...listed, away: "ssh: no route to host" })).toBe("away · ssh: no route to host");
    expect(scanWords(listed)).toBe("scans /home/me/code, 3 deep");
    expect(scanWords({ ...listed, more: true })).toContain("the first 500");
    expect(scanWords({ ...listed, scan_missing: true })).toContain("not there");
  });

  it("puts a row read again in its place, keeping its folder", () => {
    const machines: Listed[] = [{ id: "gog", label: "GOG", repos: [repo({ behind: 2 })] }];
    const after = replaced(machines, "gog", repo({ behind: 0, parent: "/code" }));
    expect(after?.[0]?.repos[0]?.behind).toBe(0);
    expect(after?.[0]?.repos[0]?.parent).toBe("~/code");
  });
});

describe("the workspace on a repository", () => {
  const pane = (id: string, agent = "") => ({ id, title: "", agent, cwd: "/code/app", focused: false, status: "idle" }) as Session["tabs"][number]["panes"][number];
  const session = (over: Partial<Session>): Session => ({ id: "w1", name: "app", status: "idle", active_tab_id: "t2", folder: "/code/app", tabs: [{ id: "t1", label: "", number: 1, named: false, status: "idle", panes: [pane("w1:p1", "claude")] }, { id: "t2", label: "", number: 2, named: false, status: "idle", panes: [pane("w1:p2")] }], ...over }) as Session;

  it("finds the sidebar's workspace for the folder, never the floating terminal's", () => {
    expect(workspaceOn([session({ scratch: true }), session({ id: "w2" })], "/code/app")?.id).toBe("w2");
    expect(workspaceOn([session({ folder: "/code/web" })], "/code/app")).toBeUndefined();
  });

  it("opens on an agent's pane before a shell's", () => {
    expect(paneOf(session({}))).toBe("w1:p1");
    expect(paneOf(session({ tabs: [] }))).toBeUndefined();
  });
});
