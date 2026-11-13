import { describe, expect, it } from "vitest";
import dialogs from "./dialogs.tsx?raw";
import folderPicker from "./FolderPicker.tsx?raw";
import healthButton from "./HealthButton.tsx?raw";
import machineMenu from "./MachineMenu.tsx?raw";
import rows from "./rows.tsx?raw";
import sidebar from "./Sidebar.tsx?raw";
import tabStrip from "./TabStrip.tsx?raw";
import sourcePicker from "./worktree/SourcePicker.tsx?raw";
import removeWorktree from "./worktree/RemoveWorktreeDialog.tsx?raw";
import actionDialog from "./project/ActionDialog.tsx?raw";
import projectSettings from "./project/ProjectSettingsDialog.tsx?raw";

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

const SOURCES = { dialogs, folderPicker, healthButton, machineMenu, rows, sidebar, tabStrip, sourcePicker, removeWorktree, actionDialog, projectSettings };

describe("the sidebar's icon-only buttons", () => {
  it("say what they do through a Tip — their own, or their menu's or popover's — and an aria-label, never a title (AGENTS.md)", () => {
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
