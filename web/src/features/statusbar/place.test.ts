import { describe, expect, it } from "vitest";
import type { Pane, PRStatus, Session } from "@/features/roster/api";
import { placeOf, prTip, prTone } from "./place";

const status = (over: Partial<PRStatus>): PRStatus => ({ state: "OPEN", draft: false, mergeable: "MERGEABLE", review: "", checks: { total: 2, pass: 2, fail: 0, pending: 0, skipped: 0 }, at: "", ...over });

describe("the pane's place", () => {
  it("takes the workspace's branch, the pane's request else its workspace's, and the first issue", () => {
    const pr = { url: "https://github.com/o/r/pull/15", label: "PR #15" };
    const shell = { id: "w1:p1", agent: "" } as Pane;
    const agent = { id: "w1:p2", agent: "claude", pr } as Pane;
    const session = { id: "w1", branch: "task-4", jira: [{ key: "APP-12" }], tabs: [{ panes: [shell, agent] }] } as unknown as Session;
    expect(placeOf(session, shell)).toEqual({ branch: "task-4", pr, issue: { key: "APP-12" } });
    expect(placeOf({ ...session, branch: undefined, jira: undefined, tabs: [{ panes: [shell] }] } as unknown as Session, shell)).toEqual({ branch: undefined, pr: undefined, issue: undefined });
  });

  it("colours the request by its state", () => {
    const pr = { url: "", label: "PR #15" };
    expect(prTone(pr)).toBe("dim");
    expect(prTone({ ...pr, status: status({}) })).toBe("green");
    expect(prTone({ ...pr, status: status({ draft: true }) })).toBe("dim");
    expect(prTone({ ...pr, status: status({ state: "MERGED" }) })).toBe("mauve");
    expect(prTone({ ...pr, status: status({ state: "CLOSED" }) })).toBe("red");
    expect(prTip({ ...pr, status: status({ review: "APPROVED" }) })).toBe("PR #15 · 2/2 checks · approved");
    expect(prTip(pr)).toBe("PR #15");
  });
});
