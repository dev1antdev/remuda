import { describe, expect, it } from "vitest";
import type { Session } from "@/features/roster/api";
import { portGroups, portsTip, processWords, stopAsk, workspacePorts } from "./ports";

describe("the live ports", () => {
  const sessions = [{ id: "w2", ports: [{ port: 5173, process: "node", label: "vite", pid: 41, address: "localhost:5173" }] }, { id: "w1" }, { id: "w3", ports: [] }] as unknown as Session[];
  it("counts the workspaces' ports and groups a machine's by workspace in the roster's order", () => {
    expect(workspacePorts(sessions)).toBe(1);
    const { groups, other } = portGroups(sessions, [
      { port: 8000, process: "python3", pid: 50, address: "*:8000" },
      { port: 5173, process: "node", label: "vite", pid: 41, address: "localhost:5173", workspace: "w2" },
      { port: 3000, process: "node", pid: 40, address: "localhost:3000", workspace: "w1" },
      { port: 3001, process: "node", pid: 40, address: "localhost:3001", workspace: "gone" },
    ]);
    expect(groups.map((g) => `${g.session.id}:${g.ports.map((p) => p.port).join(",")}`)).toEqual(["w2:5173", "w1:3000"]);
    expect(other.map((p) => p.port)).toEqual([3001, 8000]);
  });

  it("names what listens and asks before a stop", () => {
    expect(processWords({ port: 5173, process: "node", label: "vite", pid: 41, address: "" })).toBe("vite (node)");
    expect(processWords({ port: 8000, process: "python3", pid: 50, address: "" })).toBe("python3");
    expect(portsTip(3, 2)).toBe("3 ports in the workspaces · 2 others");
    expect(portsTip(1, null)).toBe("1 port in the workspaces");
    expect(stopAsk({ port: 5173, process: "node", label: "vite", pid: 41, address: "" }, "GOG")).toContain("Stop vite (node) (PID 41), listening on port 5173 of GOG?");
  });
});
