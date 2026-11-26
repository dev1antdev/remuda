import { describe, expect, it } from "vitest";
import type { Recap } from "./api";
import { calls, fileList, numbers } from "./recap";

const empty = (): Recap => ({ prompts: 0, answers: 0, took: 0, tokens: 0, cost: 0, calls: 0, tools: {}, failed: 0, commands: [], edited: [], read: [], subagents: 0, questions: 0, asking: false });
const busy = (): Recap => ({
  ...empty(),
  prompts: 2,
  answers: 2,
  took: 97,
  tokens: 38400,
  cost: 0.423,
  calls: 7,
  tools: { Read: 3, Bash: 1, Agent: 1, AskUserQuestion: 1, Edit: 1 },
  failed: 1,
  longest: { name: "Bash", hint: "go test ./...", took: 61.5 },
  commands: ["go test ./..."],
  edited: ["/w/a/b.go"],
  read: ["/w/a/c.go", "/x/d.go"],
  subagents: 1,
  questions: 1,
  said: "Fixed",
});

describe("the lines", () => {
  it("say the numbers, the calls and the files", () => {
    expect(numbers(busy())).toBe("2 prompts · 2 answers · 1 min 37 s · 38k tokens · $0.42");
    expect(numbers(empty())).toBe("0 prompts · 0 answers");
    expect(calls(busy())).toBe("7 tool calls (Read 3, Agent 1, AskUserQuestion 1, Bash 1) · 1 failed call · longest Bash 1 min 2 s (go test ./...)");
    expect(fileList(["/w/a/b.go", "/x/d.go"], "/w")).toBe("a/b.go, /x/d.go");
    expect(fileList(["/w/1", "/w/2", "/w/3"], "/w", 2)).toBe("1, 2, +1 more");
  });
});
