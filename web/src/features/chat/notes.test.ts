import { describe, expect, it } from "vitest";
import { isNote, noteKind, noteOf } from "./notes";

const TASK = `<task-notification>
<task-id>bv0ye6qdf</task-id>
<tool-use-id>toolu_01Frq</tool-use-id>
<output-file>/private/tmp/tasks/bv0ye6qdf.output</output-file>
<status>completed</status>
<summary>Background command "Wait for release's CI run" completed (exit code 0)</summary>
</task-notification>`;

describe("noteKind", () => {
  it("tells a background task's note, an interruption and a compaction's summary from the person's words", () => {
    expect(noteKind(TASK)).toBe("task");
    expect(noteKind("  \n" + TASK)).toBe("task");
    expect(noteKind("[Request interrupted by user]")).toBe("interrupted");
    expect(noteKind("[Request interrupted by user for tool use]")).toBe("interrupted");
    expect(noteKind("This session is being continued from a previous conversation that ran out of context.")).toBe("compacted");
  });
  it("leaves the person's own words alone, a prompt that quotes a note too", () => {
    expect(noteKind("merge and release")).toBeNull();
    expect(noteKind("why do I see <task-notification> in the chat?")).toBeNull();
    expect(isNote("[image]")).toBe(false);
    expect(isNote("")).toBe(false);
  });
});

describe("noteOf", () => {
  it("says what a background task came to in its own summary", () => {
    const note = noteOf(TASK);
    expect(note?.words).toBe('Background command "Wait for release\'s CI run" completed (exit code 0)');
    expect(note?.failed).toBe(false);
    expect(note?.icon).toBe("activity");
  });
  it("marks a task that failed or was killed", () => {
    expect(noteOf(TASK.replace("completed</status>", "failed</status>"))?.failed).toBe(true);
    expect(noteOf(TASK.replace("completed</status>", "killed</status>"))?.failed).toBe(true);
  });
  it("still names a task whose note has no summary", () => {
    expect(noteOf("<task-notification>\n<task-id>x</task-id>\n</task-notification>")?.words).toBe("A background task reported back");
    expect(noteOf("<task-notification><summary>cut short")?.words).toBe("A background task reported back");
  });
  it("says who stopped the agent, and where", () => {
    expect(noteOf("[Request interrupted by user]")?.words).toBe("You stopped the agent");
    expect(noteOf("[Request interrupted by user for tool use]")?.words).toBe("You stopped the agent at a tool call");
  });
  it("names a compaction's summary and nothing of the person's", () => {
    expect(noteOf("This session is being continued from a previous conversation. Summary: ...")?.kind).toBe("compacted");
    expect(noteOf("ship it")).toBeNull();
  });
});
