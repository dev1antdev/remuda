import { describe, expect, it } from "vitest";
import type { Item, Recap } from "./api";
import { calls, fileList, numbers, order, quiet, recapMarkdown, sinceFor, stateLabel, where } from "./standup";

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
const item = (over: Partial<Item>): Item => ({ machine: "local", label: "", workspace: "w", pane: "p", dir: "/w", agent: "claude", state: "idle", recap: empty(), ...over });

describe("sinceFor", () => {
  it("is nothing for today and the period's start otherwise", () => {
    const now = Date.UTC(2026, 9, 24, 12, 0, 0);
    expect(sinceFor("today", now)).toBeUndefined();
    expect(sinceFor("day", now)).toBe("2026-10-23T12:00:00.000Z");
    expect(sinceFor("week", now)).toBe("2026-10-17T12:00:00.000Z");
  });
});

describe("order", () => {
  it("puts the agents that need the person first, then the finished, then the working, the dearest first", () => {
    const items = [
      item({ workspace: "idle", state: "idle" }),
      item({ workspace: "cheap", state: "working", recap: { ...empty(), cost: 1 } }),
      item({ workspace: "dear", state: "working", recap: { ...empty(), cost: 5 } }),
      item({ workspace: "done", state: "done" }),
      item({ workspace: "noted", state: "working", alert: "stuck" }),
      item({ workspace: "asks", state: "blocked" }),
    ];
    expect(order(items).map((x) => x.workspace)).toEqual(["asks", "noted", "done", "dear", "cheap", "idle"]);
  });
});

describe("the lines", () => {
  it("say the numbers, the calls and the files", () => {
    expect(numbers(busy())).toBe("2 prompts · 2 answers · 1 min 37 s · 38k tokens · $0.42");
    expect(numbers(empty())).toBe("0 prompts · 0 answers");
    expect(calls(busy())).toBe("7 tool calls (Read 3, Agent 1, AskUserQuestion 1, Bash 1) · 1 failed call · longest Bash 1 min 2 s (go test ./...)");
    expect(fileList(["/w/a/b.go", "/x/d.go"], "/w")).toBe("a/b.go, /x/d.go");
    expect(fileList(["/w/1", "/w/2", "/w/3"], "/w", 2)).toBe("1, 2, +1 more");
  });

  it("name the place and the state", () => {
    expect(where(item({ machine: "gog", label: "gog" }))).toBe("w on gog");
    expect(where(item({}))).toBe("w");
    expect(stateLabel(item({ state: "blocked" }))).toBe("needs you");
    expect(stateLabel(item({ state: "working", alert: "stuck" }))).toBe("needs you");
    expect(stateLabel(item({ state: "done" }))).toBe("finished");
    expect(stateLabel(item({ state: "unknown" }))).toBe("");
    expect(quiet(empty())).toBe(true);
    expect(quiet(busy())).toBe(false);
  });
});

describe("recapMarkdown", () => {
  it("is the report's lines for one conversation", () => {
    expect(recapMarkdown("api on gog", busy(), "/w")).toBe(
      [
        "## api on gog",
        "",
        "- 2 prompts · 2 answers · 1 min 37 s · 38k tokens · $0.42",
        "- 7 tool calls (Read 3, Agent 1, AskUserQuestion 1, Bash 1) · 1 failed call · longest Bash 1 min 2 s (go test ./...)",
        "- Ran: `go test ./...`",
        "- Edited: `a/b.go`",
        "- Read: `a/c.go`, `/x/d.go`",
        "- 1 subagent · 1 question",
        "- Last answer: Fixed",
        "",
      ].join("\n"),
    );
  });
});
