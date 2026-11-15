import { describe, expect, it } from "vitest";
import type { Part, Turn } from "./api";
import { runningShells, runningWords, sameShells, shellLabel, shellOfCall, shellSeconds, shellsOf } from "./shells";

const started = (id: string, toolId: string, input: unknown, at = "2026-11-15T17:00:00Z"): Part => ({
  type: "tool",
  tool_name: "Bash",
  tool_id: toolId,
  at,
  took: 0.2,
  input,
  output: `Command running in background with ID: ${id}. Output is being written to: /private/tmp/claude-501/app/s1/tasks/${id}.output. You will be notified when it completes — do not poll that file.`,
});
const notified = (id: string, toolId: string, status: string, ts: string): Turn => ({
  role: "user",
  ts,
  parts: [{ type: "text", text: `<task-notification>\n<task-id>${id}</task-id>\n<tool-use-id>${toolId}</tool-use-id>\n<output-file>/tmp/${id}.output</output-file>\n<status>${status}</status>\n<summary>Background command "x" ${status} (exit code ${status === "completed" ? 0 : 1})</summary>\n</task-notification>` }],
});
const answer = (...parts: Part[]): Turn => ({ role: "assistant", ts: "2026-11-15T17:00:00Z", parts });

describe("shellsOf", () => {
  it("reads a background Bash call as a shell at work, with its id, its command and the file its output goes to", () => {
    const shells = shellsOf([answer(started("bcw0x", "toolu_1", { command: "make dev", description: "Run the dev server", run_in_background: true }))]);
    expect(shells).toHaveLength(1);
    expect(shells[0]).toMatchObject({ id: "bcw0x", toolId: "toolu_1", command: "make dev", description: "Run the dev server", status: "running", output: "/private/tmp/claude-501/app/s1/tasks/bcw0x.output" });
    expect(runningShells(shells)).toHaveLength(1);
  });
  it("leaves every other call alone: a Bash in the foreground, a failed start, another tool", () => {
    const calls = [
      { type: "tool", tool_name: "Bash", tool_id: "t1", input: { command: "ls" }, output: "a\nb" },
      { ...started("b1", "t2", { command: "x" }), is_error: true },
      { ...started("b2", "t3", { command: "x" }), tool_name: "Agent" },
      { ...started("b3", "", { command: "x" }) },
    ] as Part[];
    expect(shellsOf([answer(...calls)])).toEqual([]);
  });
  it("ends a shell with the notification that names its call, in Claude Code's own words", () => {
    const shells = shellsOf([answer(started("b1", "toolu_1", { command: "npm test" }), started("b2", "toolu_2", { command: "make dev" })), notified("b1", "toolu_1", "failed", "2026-11-15T17:05:00Z")]);
    expect(shells.map((shell) => shell.status)).toEqual(["failed", "running"]);
    expect(shells[0]?.ended).toBe("2026-11-15T17:05:00Z");
    expect(shells[0]?.summary).toBe('Background command "x" failed (exit code 1)');
    expect(runningShells(shells).map((shell) => shell.id)).toEqual(["b2"]);
  });
  it("finds the shell by its id when the notification names no call", () => {
    const note = notified("b1", "", "completed", "2026-11-15T17:05:00Z");
    expect(shellsOf([answer(started("b1", "toolu_1", { command: "x" })), note])[0]?.status).toBe("completed");
  });
  it("marks a shell the agent stopped, and keeps it stopped when the kill is told a moment later", () => {
    const stop: Part = { type: "tool", tool_name: "TaskStop", tool_id: "toolu_9", at: "2026-11-15T17:03:00Z", input: { task_id: "b1" }, output: "stopped" };
    const shells = shellsOf([answer(started("b1", "toolu_1", { command: "make dev" })), answer(stop), notified("b1", "toolu_1", "killed", "2026-11-15T17:03:01Z")]);
    expect(shells[0]).toMatchObject({ status: "stopped", ended: "2026-11-15T17:03:00Z" });
    expect(shells[0]?.summary).toContain("killed");
    const refused: Part = { ...stop, is_error: true };
    expect(shellsOf([answer(started("b1", "toolu_1", { command: "make dev" })), answer(refused)])[0]?.status).toBe("running");
  });
  it("reads the command out of an input the server cut to a string", () => {
    const cut = '{"command":"until s=$(gh run view 1 --jq \\".status\\"); do sleep 120; done","description":"Wait for CI","run_in_bac';
    expect(shellsOf([answer(started("b1", "toolu_1", cut))])[0]).toMatchObject({ description: "Wait for CI", command: 'until s=$(gh run view 1 --jq ".status"); do sleep 120; done' });
  });
  it("ignores a notification of something the turns never showed starting, and a prompt that quotes one", () => {
    expect(shellsOf([notified("gone", "toolu_0", "completed", "2026-11-15T17:05:00Z")])).toEqual([]);
    const quoting: Turn = { role: "user", parts: [{ type: "text", text: "why do I see <task-notification> in the chat?" }] };
    expect(shellsOf([answer(started("b1", "toolu_1", { command: "x" })), quoting])[0]?.status).toBe("running");
  });
});

describe("the words", () => {
  const shell = shellsOf([answer(started("b1", "toolu_1", { command: "make dev\nand more", description: "" }))])[0]!;
  it("names a shell by what the agent said it is for, else by its command's first line, cut", () => {
    expect(shellLabel(shell)).toBe("make dev");
    expect(shellLabel({ ...shell, description: "Run the dev server" })).toBe("Run the dev server");
    expect(shellLabel({ ...shell, command: "x".repeat(200) })).toHaveLength(60);
    expect(shellLabel({ ...shell, command: "" })).toBe("b1");
  });
  it("counts how long a shell ran, or has run so far", () => {
    expect(shellSeconds(shell, Date.parse("2026-11-15T17:02:00Z"))).toBe(120);
    expect(shellSeconds({ ...shell, ended: "2026-11-15T17:00:30Z" }, Date.parse("2026-11-15T18:00:00Z"))).toBe(30);
    expect(shellSeconds({ ...shell, started: undefined }, 0)).toBeNull();
  });
  it("says how many run", () => {
    expect(runningWords(1)).toBe("1 shell running in the background");
    expect(runningWords(3)).toBe("3 shells running in the background");
  });
  it("finds a call's shell and tells two readings of one apart", () => {
    expect(shellOfCall([shell], "toolu_1")).toBe(shell);
    expect(shellOfCall([shell], "toolu_2")).toBeUndefined();
    expect(shellOfCall(undefined, "toolu_1")).toBeUndefined();
    expect(sameShells(shell, { ...shell })).toBe(true);
    expect(sameShells(shell, { ...shell, status: "completed" })).toBe(false);
    expect(sameShells(undefined, undefined)).toBe(true);
    expect(sameShells(shell, undefined)).toBe(false);
  });
});
