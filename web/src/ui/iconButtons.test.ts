import { describe, expect, it } from "vitest";
import dialog from "./Dialog.tsx?raw";
import notice from "./Notice.tsx?raw";
import toasts from "./Toasts.tsx?raw";

// iconButtons are the opening tags of the icon-only buttons in a source, each with the line before it
function iconButtons(source: string): { tag: string; before: string }[] {
  const lines = source.split("\n");
  const found: { tag: string; before: string }[] = [];
  lines.forEach((line, i) => {
    if (/<button\b[^>]*className="icon-btn/.test(line)) found.push({ tag: line.trim(), before: (lines[i - 1] ?? "").trim() });
  });
  return found;
}

describe("the shared pieces' icon-only buttons", () => {
  it("say what they do through a Tip and an aria-label, never a title alone (AGENTS.md)", () => {
    for (const [name, source] of [["Dialog", dialog], ["Notice", notice], ["Toasts", toasts]] as const) {
      const buttons = iconButtons(source);
      expect(buttons.length, name).toBeGreaterThan(0);
      for (const { tag, before } of buttons) {
        expect(tag, name).not.toMatch(/\btitle=/);
        expect(tag, name).toMatch(/aria-label=\{t[r]?\(/);
        expect(before, name).toMatch(/^<Tip text=\{t[r]?\(/);
      }
    }
  });
});
