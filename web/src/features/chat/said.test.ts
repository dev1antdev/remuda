import { describe, expect, it } from "vitest";
import type { Turn } from "./api";
import { SAID_MAX, firstLine, said } from "./said";

const user: Turn = { role: "user", parts: [{ type: "text", text: "go" }] };
const assistant = (...parts: Turn["parts"]): Turn => ({ role: "assistant", parts });

describe("said", () => {
  it("is the first line of the text the last turn ended with", () => {
    expect(said([user, assistant({ type: "thinking", text: "hmm" }, { type: "text", text: "## Done\n\nAdded tests." })])).toBe("Done");
    expect(said([user, assistant({ type: "text", text: "Looking." }, { type: "tool", tool_name: "Bash", input: {}, output: "ok" }, { type: "text", text: "All green." })])).toBe("All green.");
  });

  it("is nothing after a tool call, after the person, or with no turn", () => {
    expect(said([user, assistant({ type: "text", text: "Running." }, { type: "tool", tool_name: "Bash", input: {}, output: "" })])).toBe("");
    expect(said([assistant({ type: "text", text: "Done." }), user])).toBe("");
    expect(said([])).toBe("");
  });
});

describe("firstLine", () => {
  it("strips the marks", () => {
    expect(firstLine("\n\n- **Fixed** the `parser` in [x](http://a)\n- more")).toBe("Fixed the parser in x");
    expect(firstLine("> 1. [ ] *note* ~~old~~ <b>new</b>")).toBe("note old new");
    expect(firstLine("```go\nfunc x() {}\n```\nThe code above.")).toBe("The code above.");
    expect(firstLine("---\n\n   \nFirst   real\tline")).toBe("First real line");
    expect(firstLine("snake_case_name stays")).toBe("snake_case_name stays");
    expect(firstLine("")).toBe("");
  });

  it("cuts at a word with an ellipsis", () => {
    const got = firstLine("word ".repeat(40));
    expect(got.endsWith("…")).toBe(true);
    expect(got).not.toContain(" …");
    expect(Array.from(got).length).toBeLessThanOrEqual(SAID_MAX + 1);
    expect(Array.from(firstLine("x".repeat(200))).length).toBe(SAID_MAX + 1);
    expect(firstLine("Все тесты прошли, можно мерджить")).toBe("Все тесты прошли, можно мерджить");
  });
});
