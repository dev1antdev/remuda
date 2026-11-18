import { describe, expect, it, vi } from "vitest";
import type { Machine, Session } from "./api";
import { askRemoveWorktree } from "./prMenu";

const openDialog = vi.hoisted(() => vi.fn());
vi.mock("@/app/store", () => ({ actions: { openDialog } }));

describe("removing a merged worktree from the request's menu", () => {
  it("opens the remove dialog, where its checks are", () => {
    const session = { id: "w1", name: "task-20-x" } as Session;
    askRemoveWorktree({ id: "box" } as Machine, session);
    expect(openDialog).toHaveBeenCalledWith({ kind: "remove-worktree", machineId: "box", session });
  });
});
