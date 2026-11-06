import { describe, expect, it } from "vitest";
import type { Listener } from "./api";
import { hostOf, listenerWords, openOn, split, whereWords } from "./previews";

const l = (port: number, more: Partial<Listener> = {}): Listener => ({ port, address: "127.0.0.1", pid: 1, command: "node", ...more });

describe("the previews dialog", () => {
  it("names a port by its number, its process and what it is", () => {
    expect(listenerWords(l(3000, { label: "vite" }))).toBe("3000 · node · vite");
    expect(listenerWords(l(8000, { command: "python3" }))).toBe("8000 · python3");
    expect(whereWords(l(1, { dir: "/Users/me/.herdr/worktrees/app/task-1" }))).toBe("task-1");
    expect(whereWords(l(1))).toBe("");
  });
  it("sorts the ports into the workspace's, the other workspaces' and the rest, each by port", () => {
    const list = [l(8000, { workspace: "w2" }), l(5173, { workspace: "w1" }), l(3000, { workspace: "w1" }), l(9000), l(49735)];
    const { here, workspaces, others } = split(list, "w1");
    expect(here.map((x) => x.port)).toEqual([3000, 5173]);
    expect(workspaces.map((x) => x.port)).toEqual([8000]);
    expect(others.map((x) => x.port)).toEqual([9000, 49735]);
    const machine = split(list);
    expect(machine.here).toEqual([]);
    expect(machine.workspaces.map((x) => x.port)).toEqual([3000, 5173, 8000]);
    expect(machine.others.map((x) => x.port)).toEqual([9000, 49735]);
  });
  it("finds a port open already and shows its address without the scheme", () => {
    const open = [{ machine: "gog", port: 3000, local: 3000, url: "https://remuda.example.ts.net:3000", opened: "" }];
    expect(openOn(open, 3000)?.local).toBe(3000);
    expect(openOn(open, 5173)).toBeUndefined();
    expect(hostOf("https://remuda.example.ts.net:3000")).toBe("remuda.example.ts.net:3000");
    expect(hostOf("not a url")).toBe("not a url");
  });
});
