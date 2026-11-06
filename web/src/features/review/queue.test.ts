import { describe, expect, it } from "vitest";
import type { Machine, Pane, Session } from "@/features/roster/api";
import { reviewQueue } from "./queue";

const pane = (id: string, agent: string): Pane => ({ id, title: id, agent, cwd: `/w/${id}`, focused: false, status: agent ? "idle" : "shell", cols: 80, rows: 24 });
const session = (id: string, review: Session["review"], panes: Pane[], dir = `/w/${id}`): Session => ({ id, name: id, status: "idle", active_tab_id: `${id}-t`, dir, review, tabs: [{ id: `${id}-t`, label: "1", number: 1, named: false, status: "idle", panes }] });
const machine = (id: string, sessions: Session[], state: Machine["state"] = "connected"): Machine => ({ id, label: id, state, sessions });

describe("reviewQueue", () => {
  it("lists the workspaces with uncommitted work, ready again first, then fresh, in the roster's order otherwise", () => {
    const machines = [
      machine("local", [session("clean", { changed: 0, fresh: 0 }, [pane("a", "claude")]), session("old", { changed: 3, fresh: 0 }, [pane("b", "claude")]), session("hot", { changed: 2, fresh: 2 }, [pane("c", "")]), session("back", { changed: 1, fresh: 1, ready: true }, [pane("h", "claude")])]),
      machine("gog", [session("warm", { changed: 5, fresh: 1 }, [pane("d", ""), pane("e", "codex")]), session("nodir", { changed: 1, fresh: 1 }, [pane("f", "claude")], ""), session("fixed", { changed: 0, fresh: 0, ready: true }, [pane("i", "claude")])]),
      machine("down", [session("x", { changed: 9, fresh: 9 }, [pane("g", "claude")])], "error"),
    ];
    const queue = reviewQueue(machines);
    expect(queue.map((item) => item.key)).toEqual(["local|back", "gog|fixed", "local|hot", "gog|warm", "local|old"]);
    expect(queue.map((item) => item.ready)).toEqual([true, true, false, false, false]); // a ready one is listed even with everything committed
    expect(queue[2]?.pane?.id).toBe("c"); // a shell is still whose lens it is when no agent runs there
    expect(queue[3]?.pane?.id).toBe("e"); // the agent pane first
    expect(queue.map((item) => [item.fresh, item.changed])).toEqual([
      [1, 1],
      [0, 0],
      [2, 2],
      [1, 5],
      [0, 3],
    ]);
  });

  it("is empty when nothing is dirty anywhere", () => {
    expect(reviewQueue([machine("local", [session("clean", { changed: 0, fresh: 0 }, [pane("a", "claude")]), session("unknown", undefined, [pane("b", "claude")])])])).toEqual([]);
  });
});
