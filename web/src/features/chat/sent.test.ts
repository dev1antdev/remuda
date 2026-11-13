import { describe, expect, it } from "vitest";
import { setLang } from "@/lib/i18n";
import type { Turn } from "./api";
import { doing } from "./doing";
import { conversationMarkdown } from "./export";

setLang("en");

// a message the person or another agent sent while the agent worked rides in the turn it was typed into
const turns: Turn[] = [
  { role: "user", parts: [{ type: "text", text: "Rename the flag" }] },
  {
    role: "assistant",
    parts: [
      { type: "tool", tool_name: "Bash", input: { command: "grep -rn fast" }, output: "x" },
      { type: "queued", text: "also the README", at: "2026-11-13T09:00:06Z" },
      { type: "queued", text: "pkg/x fails on main too", from: "general-purpose" },
    ],
  },
];

describe("a message sent while the agent worked", () => {
  it("is exported as the person's words, in its place", () => {
    const md = conversationMarkdown(turns, { agent: "claude", workspace: "w", machine: "m", when: new Date("2026-11-13T10:00:00Z") });
    expect(md).toMatch(/\*\*You, while it worked\*\* · [^\n]+\n\nalso the README\n/);
    expect(md).toContain("**general-purpose, while it worked**\n\npkg/x fails on main too\n");
    expect(md.indexOf("also the README")).toBeGreaterThan(md.indexOf("grep -rn fast"));
  });

  it("leaves the agent taking it in, not writing", () => {
    expect(doing(turns)).toEqual({ what: "thinking" });
  });
});
