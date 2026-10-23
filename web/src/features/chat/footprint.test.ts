import { describe, expect, it } from "vitest";
import type { Part, Turn } from "./api";
import { footprint, shownPath, toolLabel, toolPath } from "./footprint";

const tool = (tool_name: string, input: unknown, is_error = false): Part => ({ type: "tool", tool_name, input, is_error });
const assistant = (ts: string, ...parts: Part[]): Turn => ({ role: "assistant", ts, parts });

describe("toolPath", () => {
  it("reads the file from an object input, whichever key names it", () => {
    expect(toolPath(tool("Edit", { replace_all: false, file_path: "/r/a.go", old_string: "x" }))).toBe("/r/a.go");
    expect(toolPath(tool("NotebookEdit", { notebook_path: "/r/n.ipynb" }))).toBe("/r/n.ipynb");
    expect(toolPath(tool("Grep", { pattern: "x", path: "/r/src" }))).toBe("/r/src");
    expect(toolPath(tool("Bash", { command: "ls" }))).toBeNull();
  });
  it("still finds the file when the server cut the input to a string", () => {
    const cut = '{"file_path":"/r/web/src/a \\"b\\".tsx","content":"import { useEffect } from \\"react\\";\\n…';
    expect(toolPath(tool("Write", cut))).toBe('/r/web/src/a "b".tsx');
    expect(toolPath(tool("Write", "not json at all"))).toBeNull();
    expect(toolPath(tool("Write", undefined))).toBeNull();
  });
});

describe("footprint", () => {
  const turns: Turn[] = [
    { role: "user", parts: [{ type: "text", text: "go" }, tool("Edit", { file_path: "/r/user.go" })] },
    assistant("2026-10-21T10:00:00Z", tool("Read", { file_path: "/r/a.go" }), tool("Edit", { file_path: "/r/a.go" })),
    assistant("2026-10-21T10:05:00Z", tool("Write", { file_path: "/r/b.go" }, true), tool("Read", { file_path: "/r/c.go" }), tool("Bash", { command: "go test" })),
    assistant("2026-10-21T10:06:00Z", tool("Read", { file_path: "/r/a.go" })),
  ];
  it("groups by file, counts edits and reads, keeps the last touch", () => {
    const { edited, read } = footprint(turns);
    expect(edited.map((touch) => touch.path)).toEqual(["/r/a.go", "/r/b.go"]);
    expect(edited[0]).toMatchObject({ edits: 1, reads: 2, failed: false, at: Date.parse("2026-10-21T10:06:00Z") });
    expect(edited[1]).toMatchObject({ edits: 1, reads: 0, failed: true, at: Date.parse("2026-10-21T10:05:00Z") });
    expect(read.map((touch) => touch.path)).toEqual(["/r/c.go"]);
  });
  it("ignores the person's turns and tools without a file", () => {
    const { edited, read } = footprint(turns);
    expect([...edited, ...read].some((touch) => touch.path === "/r/user.go")).toBe(false);
    expect(footprint([assistant("", tool("Bash", { command: "ls" }))])).toEqual({ edited: [], read: [] });
  });
  it("orders the newest touch first and clears a failure once a later call succeeds", () => {
    const later = [...turns, assistant("2026-10-21T10:09:00Z", tool("Edit", { file_path: "/r/b.go" }))];
    const { edited } = footprint(later);
    expect(edited.map((touch) => touch.path)).toEqual(["/r/b.go", "/r/a.go"]);
    expect(edited[0]).toMatchObject({ edits: 2, failed: false });
  });
  it("takes a turn without a time as unknown", () => {
    expect(footprint([assistant("", tool("Edit", { file_path: "/r/a.go" }))]).edited[0]?.at).toBe(0);
  });
});

describe("shownPath", () => {
  it("is relative under the pane's folder, whole elsewhere", () => {
    expect(shownPath("/r/web/src/a.ts", "/r")).toBe("web/src/a.ts");
    expect(shownPath("/home/x/.claude/plans/p.md", "/r")).toBe("/home/x/.claude/plans/p.md");
    expect(shownPath("/r2/a.ts", "/r")).toBe("/r2/a.ts");
    expect(shownPath("/r/a.ts", "")).toBe("/r/a.ts");
  });
});

describe("toolLabel", () => {
  it("names the tool and what it was about, a file linked only when read or written", () => {
    expect(toolLabel({ type: "tool", tool_name: "Edit", input: { file_path: "/w/a.go" } })).toEqual({ name: "Edit", hint: "/w/a.go", file: "/w/a.go" });
    expect(toolLabel({ type: "tool", tool_name: "Grep", input: { pattern: "x", path: "/w" } })).toEqual({ name: "Grep", hint: "/w", file: null });
    expect(toolLabel({ type: "tool", tool_name: "Bash", input: { command: "go test ./..." } })).toEqual({ name: "Bash", hint: "go test ./...", file: null });
    expect(toolLabel({ type: "tool", tool_name: "Bash", input: { command: "x".repeat(70) } }).hint).toBe("x".repeat(60) + "…");
    expect(toolLabel({ type: "tool", input: "cut" })).toEqual({ name: "tool", hint: "", file: null });
  });
});
