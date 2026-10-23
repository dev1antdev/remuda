import { describe, expect, it } from "vitest";
import type { Turn } from "./api";
import { conversationMarkdown, exportFileName, type ExportMeta } from "./export";

const meta: ExportMeta = { agent: "claude", workspace: "remuda", machine: "studio", sessionId: "0123456789abcdef", when: new Date("2026-10-23T14:05:00Z") };

describe("conversationMarkdown", () => {
  it("heads the file with where it ran and writes each turn with who said it", () => {
    const turns: Turn[] = [
      { role: "user", parts: [{ type: "text", text: "Fix the tests" }] },
      { role: "assistant", parts: [{ type: "text", text: "Done: **two** files." }] },
    ];
    const md = conversationMarkdown(turns, meta);
    expect(md.startsWith("# remuda · Claude Code on studio\n\nExported from remuda on ")).toBe(true);
    expect(md).toContain(" · session 0123456789abcdef · 2 turns\n\n---\n\n**You**\n\nFix the tests\n\n**Claude Code**\n\nDone: **two** files.\n");
    expect(md.endsWith("\n")).toBe(true);
    expect(md).not.toContain("\n\n\n");
  });

  it("folds thinking and tool calls, fencing an output past any backticks in it", () => {
    const turns: Turn[] = [
      {
        role: "assistant",
        parts: [
          { type: "thinking", text: "Let me look." },
          { type: "tool", tool_name: "Bash", input: { command: "go test ./..." }, output: "ok\n```\n````inside\n" },
          { type: "tool", tool_name: "Read", input: { file_path: "/w/a.go" }, output: "", is_error: true },
          { type: "tool", tool_name: "Edit", input: { file_path: "/w/<b>.go" } },
        ],
      },
    ];
    const md = conversationMarkdown(turns, meta);
    expect(md).toContain("<details><summary>Thinking</summary>\n\nLet me look.\n\n</details>");
    expect(md).toContain("<details><summary>Bash · go test ./...</summary>\n\n`````\nok\n```\n````inside\n`````\n\n</details>");
    expect(md).toContain("<details><summary>Read · /w/a.go · error</summary></details>");
    expect(md).toContain("<details><summary>Edit · /w/&lt;b&gt;.go</summary></details>");
  });

  it("names an agent it does not know by its key, and a transcript on its own Agent", () => {
    expect(conversationMarkdown([], { ...meta, agent: "zed" })).toContain("# remuda · zed on studio");
    expect(conversationMarkdown([], { ...meta, agent: "", sessionId: undefined })).toContain("# remuda · Agent on studio\n\nExported from remuda on ");
    expect(conversationMarkdown([], { ...meta, sessionId: undefined })).toContain(" · 0 turns\n");
  });
});

describe("exportFileName", () => {
  it("is the workspace, the day and the session's start, file-safe", () => {
    expect(exportFileName(meta)).toBe("remuda-2026-10-23-01234567.md");
    expect(exportFileName({ ...meta, workspace: "Открытые МР / мастер", sessionId: undefined })).toBe("Открытые-МР-мастер-2026-10-23.md");
    expect(exportFileName({ ...meta, workspace: "///" })).toBe("conversation-2026-10-23-01234567.md");
  });
});
