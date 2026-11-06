import { describe, expect, it } from "vitest";
import type { Machine } from "@/features/roster/api";
import { agentTargets, targetWords } from "./targets";

const machines = [
  {
    id: "gog",
    label: "GOG",
    state: "connected",
    sessions: [
      {
        id: "w1",
        name: "billing-api",
        folder: "/p/billing",
        dir: "/p/billing",
        tabs: [{ id: "t1", label: "1", named: false, panes: [{ id: "w1:p1", agent: "claude", title: "Refund the March invoices", status: "run", cwd: "/p/billing" }, { id: "w1:p2", agent: "", title: "zsh", status: "shell", cwd: "/p/billing" }] }],
      },
    ],
  },
] as unknown as Machine[];

describe("the agents a picker offers", () => {
  it("carry their workspace's folder, so the screen-sharing mode can tell a hidden one", () => {
    expect(agentTargets(machines).map((a) => [a.paneId, a.folder])).toEqual([["w1:p1", "/p/billing"]]);
  });

  it("are named by their workspace and conversation, or under the mode by the neutral name and the agent's kind", () => {
    const [a] = agentTargets(machines);
    expect(targetWords(a!, null)).toBe("billing-api › Refund the March invoices");
    expect(targetWords(a!, "Project A")).toBe("Project A › Claude Code");
  });
});
