import { describe, expect, it } from "vitest";
import type { Turn } from "./api";
import { doing } from "./doing";

const user: Turn = { role: "user", parts: [{ type: "text", text: "go" }] };
const assistant = (...parts: Turn["parts"]): Turn => ({ role: "assistant", parts });

describe("doing", () => {
  it("names the tool call that has no result yet", () => {
    expect(doing([user, assistant({ type: "text", text: "Running the tests." }, { type: "tool", tool_name: "Bash", input: { command: "go test ./..." } })])).toEqual({ what: "tool", name: "Bash", hint: "go test ./..." });
    expect(doing([assistant({ type: "tool", tool_name: "Edit", input: { file_path: "/w/internal/server/chat.go" } })])).toEqual({ what: "tool", name: "Edit", hint: "/w/internal/server/chat.go" });
    expect(doing([assistant({ type: "tool", tool_name: "Read", input: '{"file_path":"/w/a.go"' })])).toEqual({ what: "tool", name: "Read", hint: "/w/a.go" });
  });

  it("is thinking once the result is in, after the prompt, or in a thinking block", () => {
    expect(doing([assistant({ type: "tool", tool_name: "Bash", input: { command: "ls" }, output: "" })])).toEqual({ what: "thinking" });
    expect(doing([assistant({ type: "tool", tool_name: "Bash", input: { command: "ls" }, output: "a\nb" })])).toEqual({ what: "thinking" });
    expect(doing([user])).toEqual({ what: "thinking" });
    expect(doing([])).toEqual({ what: "thinking" });
    expect(doing([assistant({ type: "thinking", text: "…" })])).toEqual({ what: "thinking" });
    expect(doing([assistant()])).toEqual({ what: "thinking" });
  });

  it("is writing when the last part is text", () => {
    expect(doing([user, assistant({ type: "tool", tool_name: "Bash", input: { command: "ls" }, output: "a" }, { type: "text", text: "Done: " })])).toEqual({ what: "writing" });
  });
});
