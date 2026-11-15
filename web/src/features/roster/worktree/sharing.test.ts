import { describe, expect, it } from "vitest";
import create from "./NewWorktreeDialog.tsx?raw";
import settings from "../project/ProjectSettingsDialog.tsx?raw";

// the New worktree dialog's words carry the repository's folder (~/code/app, ~/.herdr/worktrees/app/x) and gh's
// sentence the machine's name: under the screen-sharing mode they go through its redact like the project pick does
describe("the worktree dialogs under the screen-sharing mode", () => {
  it("say the note, the rows, the empty list and the errors through the mode", () => {
    expect(create).toMatch(/note=\{share\.redact\(note\.note\)\}/);
    expect(create).toMatch(/empty=\{share\.redact\(emptyWords\(/);
    expect(create).toMatch(/rows=\{shownRows\}/);
    expect(create).toMatch(/\{share\.redact\(answer\.error\)\}/);
    expect(create).toMatch(/\{share\.redact\(error\)\}/);
    expect(settings).not.toMatch(/worktrees\/\$\{p\?\.name/);
  });
});
