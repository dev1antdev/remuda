import { describe, expect, it } from "vitest";
import composer from "./Composer.tsx?raw";
import findBar from "./FindBar.tsx?raw";
import pinsDialog from "./PinsDialog.tsx?raw";
import promptsList from "./PromptsList.tsx?raw";
import queueList from "./QueueList.tsx?raw";
import quickPrompts from "./QuickPrompts.tsx?raw";
import timelineBar from "./TimelineBar.tsx?raw";
import turnView from "./TurnView.tsx?raw";

// openingTag is the `<button …>` starting at `from`, across lines, its braces (an arrow's `>` among them) skipped
function openingTag(source: string, from: number): string {
  let depth = 0;
  for (let i = from; i < source.length; i++) {
    const ch = source[i];
    if (ch === "{") depth++;
    else if (ch === "}") depth--;
    else if (ch === ">" && depth === 0) return source.slice(from, i + 1).replace(/\s+/g, " ");
  }
  return source.slice(from);
}

// iconButtons are the opening tags of the icon-only buttons in a source, each with the lines before it
function iconButtons(source: string): { tag: string; before: string[] }[] {
  const found: { tag: string; before: string[] }[] = [];
  for (let at = source.indexOf("<button"); at >= 0; at = source.indexOf("<button", at + 1)) {
    const tag = openingTag(source, at);
    if (!/className=\{?[`"]icon-btn/.test(tag)) continue;
    const before = source.slice(0, at).split("\n").slice(-10).map((line) => line.trim()).filter(Boolean);
    found.push({ tag, before });
  }
  return found;
}

const SOURCES = { composer, findBar, pinsDialog, promptsList, queueList, quickPrompts, timelineBar, turnView };

describe("the chat's icon-only buttons", () => {
  it("say what they do through a Tip — their own, or their menu's — and an aria-label, never a title (AGENTS.md)", () => {
    for (const [name, source] of Object.entries(SOURCES)) {
      const buttons = iconButtons(source);
      expect(buttons.length, name).toBeGreaterThan(0);
      for (const { tag, before } of buttons) {
        expect(tag, name).not.toMatch(/\btitle=/);
        expect(tag, name).toMatch(/aria-label=/);
        const last = before[before.length - 1] ?? "";
        const own = /<Tip text=/.test(last);
        const opener = last === "trigger={" && before.some((line) => /\btip=/.test(line));
        expect(own || opener, `${name}: ${tag}`).toBe(true);
      }
    }
  });
});
