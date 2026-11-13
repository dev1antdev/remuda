import { describe, expect, it } from "vitest";
import { setLang } from "@/lib/i18n";
import { tokFine } from "@/features/usage/format";
import type { Turn } from "./api";
import { alsoWords, facts, gist, plainText, promptCards, totalOutput, walk } from "./prompts";
import { promptedAt, spinnerSeconds, spinnerWords } from "./progress";

setLang("en");

const tool = (name: string, input: unknown, output = "ok") => ({ type: "tool" as const, tool_name: name, input, output });
const text = (s: string) => ({ type: "text" as const, text: s });

const conversation: Turn[] = [
  { role: "user", ts: "2026-11-13T15:31:00Z", ids: ["u1"], parts: [text("add a **skeleton** loader to the cart")] },
  {
    role: "assistant",
    ts: "2026-11-13T15:31:04Z",
    took: 300,
    output: 7400,
    parts: [
      tool("Edit", { file_path: "/w/src/Cart.tsx", old_string: "a", new_string: "b" }),
      tool("Bash", { command: "npm test" }),
      tool("Write", { file_path: "/w/src/CartSkeleton.tsx", content: "x" }),
      tool("Edit", { file_path: "/w/src/Cart.tsx", old_string: "c", new_string: "d" }),
      text("Added CartSkeleton and wired it to the loading state. The tests pass."),
    ],
  },
  { role: "user", ts: "2026-11-13T15:58:00Z", ids: ["u2"], parts: [text("why does the payment webhook fail on staging?")] },
  {
    role: "assistant",
    ts: "2026-11-13T15:58:03Z",
    took: 120,
    output: 5000,
    parts: [tool("Bash", { command: "curl staging" }), { type: "queued", text: "check the **secret** first", at: "2026-11-13T15:59:00Z" }, text("Looking.")],
  },
  { role: "user", ts: "2026-11-13T16:00:10Z", ids: ["u3"], parts: [text("[Request interrupted by user]")] },
  { role: "assistant", ts: "2026-11-13T16:00:20Z", took: 112, output: 4100, parts: [tool("Bash", { command: "rotate" }), text("Done. Staging signs webhooks with the old secret; I rotated it and replayed the failed ones.")] },
];

describe("prompt cards", () => {
  it("reads a card per prompt, an interruption staying with the card before", () => {
    const cards = promptCards(conversation);
    expect(cards.map((c) => [c.n, c.id, c.text])).toEqual([
      [1, "u1", "add a **skeleton** loader to the cart"],
      [2, "u2", "why does the payment webhook fail on staging?"],
    ]);
    expect(cards[0]).toMatchObject({ took: 300, output: 7400, edits: ["/w/src/Cart.tsx", "/w/src/CartSkeleton.tsx"], commands: 1 });
    expect(cards[1]).toMatchObject({ output: 9100, commands: 2, also: [{ at: "2026-11-13T15:59:00Z", text: "check the **secret** first" }] });
    expect(cards[1]!.done).toBe(Date.parse("2026-11-13T16:00:10Z") + 112_000);
    expect(cards[1]!.took).toBe(242);
    expect(cards[1]!.reply).toBe("Done. Staging signs webhooks with the old secret; I rotated it and replayed the failed ones.");
  });

  it("starts with the person's words even when a mid-turn message comes first", () => {
    const cards = promptCards([{ role: "user", ts: "2026-11-13T09:00:00Z", ids: ["m0"], parts: [{ type: "queued", text: "and this" }] }]);
    expect(cards).toHaveLength(1);
    expect(cards[0]).toMatchObject({ text: "and this", also: [] });
  });

  it("says the facts with every zero left out", () => {
    const [first, second] = promptCards(conversation);
    const clock = () => "16:02";
    expect(facts(first!, null, clock)).toEqual({ lead: "", rest: ["done 16:02", "5 min", "↓ 7.4k tokens", "2 files", "1 command run"] });
    expect(facts({ ...second!, edits: [], commands: 0, done: undefined, took: 0, output: 0 }, null, clock)).toEqual({ lead: "", rest: [] });
    expect(facts(second!, { seconds: 134, tokens: 9800 }, clock)).toEqual({ lead: "working", rest: ["2 min 14 s", "↓ 9.8k tokens", "2 commands run"] });
    expect(totalOutput([first!, second!], { seconds: 1, tokens: 9800 })).toBe(7400 + 9800);
    expect(totalOutput([first!, second!], null)).toBe(7400 + 9100);
  });

  it("words the messages sent mid-turn on one line", () => {
    expect(alsoWords([])).toBe("");
    expect(alsoWords([{ text: "check the **secret**" }])).toBe("+ 1 more message: check the secret");
    expect(alsoWords([{ text: "one" }, { text: "two\nlines" }])).toBe("+ 2 more messages: one · two lines");
  });
});

describe("a reply's gist", () => {
  it("is the first sentence, two when the first is short, cut at 140", () => {
    expect(gist("Staging signs webhooks with the old secret; I rotated it. More here.")).toBe("Staging signs webhooks with the old secret; I rotated it.");
    expect(gist("Done. The tests pass now. And more.")).toBe("Done. The tests pass now.");
    expect(gist("## Fixed\n\nThe **tests** pass.\n\n```go\nx := 1\n```")).toBe("Fixed The tests pass.");
    const long = gist("word ".repeat(60));
    expect(long.length).toBe(140);
    expect(long.endsWith("…")).toBe(true);
    expect(plainText("- [x] `done` and [a link](http://x)")).toBe("done and a link");
  });
});

describe("the list's keys", () => {
  it("walks the pick, an open card moving with it", () => {
    expect(walk({ at: 0, open: null }, "ArrowDown", 3)).toEqual({ at: 1, open: null });
    expect(walk({ at: 2, open: null }, "ArrowDown", 3)).toEqual({ at: 2, open: null });
    expect(walk({ at: 1, open: 1 }, "ArrowUp", 3)).toEqual({ at: 0, open: 0 });
    expect(walk({ at: 1, open: null }, "Enter", 3)).toEqual({ at: 1, open: 1 });
    expect(walk({ at: 1, open: 1 }, "Enter", 3)).toEqual({ at: 1, open: null });
    expect(walk({ at: 1, open: 1 }, "Escape", 3)).toEqual({ at: 1, open: null });
    expect(walk({ at: 1, open: null }, "Escape", 3)).toBeNull();
    expect(walk({ at: 0, open: null }, "a", 3)).toBeNull();
    expect(walk({ at: 0, open: null }, "ArrowDown", 0)).toBeNull();
  });
});

describe("the spinner", () => {
  it("runs its clock on from the read", () => {
    expect(spinnerSeconds({ verb: "Refactoring", seconds: 134 }, 1_000, 4_500)).toBe(137);
    expect(spinnerSeconds({ verb: "Thinking" }, 1_000, 4_500, 2_000)).toBe(2);
    expect(spinnerSeconds({ verb: "Thinking" }, 1_000, 4_500)).toBeNull();
    expect(spinnerSeconds(undefined, 0, 70_000, 10_000)).toBe(60);
  });

  it("says the line as the CLI does", () => {
    expect(spinnerWords("Refactoring", 134, 6800)).toBe("Refactoring… 2 min 14 s · ↓ 6.8k");
    expect(spinnerWords("Thinking", null)).toBe("Thinking…");
    expect(spinnerWords("", 5, 0)).toBe("5 s");
  });

  it("finds when the turn under way began", () => {
    expect(promptedAt(conversation)).toBe(Date.parse("2026-11-13T16:00:10Z"));
    expect(promptedAt([])).toBe(0);
  });

  it("counts tokens as the CLI words them", () => {
    expect([840, 6800, 9100, 9999, 38000, 418_000, 999_499, 1_000_000, 1_240_000, 2e9].map(tokFine)).toEqual(["840", "6.8k", "9.1k", "10k", "38k", "418k", "999k", "1M", "1.2M", "2B"]);
  });
});
