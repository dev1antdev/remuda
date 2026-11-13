import { describe, expect, it } from "vitest";
import type { Machine, Session } from "@/features/roster/api";
import type { Repo } from "./api";
import { namedAfter, openWorkspace, paneOf, startFrom } from "./links";

const session = (id: string, dir: string, branch: string, linked = false): Session => ({
  id,
  name: id,
  status: "idle",
  active_tab_id: `${id}:t1`,
  dir,
  branch,
  ...(linked ? { worktree: { repo_key: "k", repo_name: "widgets", linked: true, checkout_path: dir } } : {}),
  tabs: [{ id: `${id}:t1`, number: 1, label: "", named: false, panes: [{ id: `${id}:p1`, agent: "", cwd: dir } as never, { id: `${id}:p2`, agent: "claude", cwd: dir } as never] } as never],
});

const repo: Repo = { repo: "acme/widgets", folder: "/w", dirs: ["/w", "/trees/a", "/trees/b"], links: [{ branch: "fix-crash", number: 41 }] };

describe("the workspace an issue has", () => {
  it("knows a branch named after the issue", () => {
    for (const branch of ["dev/feat/41", "issue-41-crash", "feat/41-crash", "41-crash", "gh-41", "fix/41"]) expect(namedAfter(branch, 41)).toBe(true);
    for (const branch of ["dev/feat/410", "task-41-crash", "v1.41", "main", "issue-141"]) expect(namedAfter(branch, 41)).toBe(false);
  });
  it("is a linked branch first, then one named after it, in the repository's checkouts only", () => {
    const machine = { id: "local", sessions: [session("main", "/w", "main"), session("a", "/trees/a", "dev/feat/41", true), session("b", "/trees/b", "fix-crash", true), session("other", "/elsewhere", "issue-41")] } as Machine;
    expect(openWorkspace(machine, repo, 41)?.id).toBe("b");
    expect(openWorkspace(machine, { ...repo, links: [] }, 41)?.id).toBe("a");
    expect(openWorkspace(machine, { ...repo, links: [], dirs: ["/w"] }, 41)).toBeNull();
    expect(openWorkspace(undefined, repo, 41)).toBeNull();
  });
  it("branches a new one off the main checkout", () => {
    const machine = { id: "local", sessions: [session("a", "/trees/a", "x", true), session("main", "/w", "main")] } as Machine;
    expect(startFrom(machine, repo)?.id).toBe("main");
    expect(startFrom({ id: "local", sessions: [session("a", "/trees/a", "x", true)] } as Machine, repo)?.id).toBe("a");
    expect(startFrom({ id: "local", sessions: [] } as unknown as Machine, repo)).toBeNull();
  });
  it("opens on the active tab's agent", () => {
    expect(paneOf(session("a", "/w", "main"))).toBe("a:p2");
  });
});
