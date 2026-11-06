import { describe, expect, it } from "vitest";
import overlays from "@/app/Overlays.tsx?raw";
import arena from "@/features/arena/ArenaPanel.tsx?raw";
import teleport from "@/features/teleport/TeleportDialog.tsx?raw";
import everywhere from "@/features/updates/EverywhereDialog.tsx?raw";
import updates from "@/features/updates/Updates.tsx?raw";
import dialogs from "./dialogs.tsx?raw";
import picker from "./FolderPicker.tsx?raw";
import leftovers from "./LeftoversDialog.tsx?raw";
import mac from "./MacDialog.tsx?raw";

// the "make something" dialogs; a rename's field holds the name it changes, so the renames are not among them
const making = dialogs.slice(dialogs.indexOf("export function NewWorkspaceDialog"), dialogs.indexOf("export function RenameMachineDialog"));

// the dialogs and the confirm the sidebar's menus, the ⓘ and the palette open — the menus already saying the neutral
// names under the screen-sharing mode — and what in them would say a real one straight off the roster or the server
const SOURCES = { overlays, arena, teleport, everywhere, updates, making, picker, leftovers, mac };
const RAW_NAMES = [
  /(?<!redact\()\b(?:machine|target|group|chosen\?|x)\.label\b/, // a machine's own label, not through the mode
  /\{ workspace \}\)|workspace: session\.name\b/, // a workspace's own name in the words
  /(?<!redact\()session\.worktree\?\.repo_name/, // a repository's own name
  /(?<!key=)\{left\.(?:repo|name|path)\}/, // a leftover worktree's folder, said rather than a key
];

describe("the dialogs the screen-sharing mode reaches", () => {
  it("read the mode and say no machine's, workspace's or folder's real name past it", () => {
    for (const [name, source] of Object.entries(SOURCES)) {
      expect(source, name).toMatch(/useSharing\(\)/);
      for (const raw of RAW_NAMES) expect(source, `${name}: ${raw}`).not.toMatch(raw);
    }
    expect(overlays).toMatch(/share\.redact\(confirm\.text\)/);
  });

  it("still catches a real name said straight", () => {
    expect('t("MAC address of {machine}", { machine: machine.label })').toMatch(RAW_NAMES[0]!);
    expect('t("Reading the work in {workspace}…", { workspace })').toMatch(RAW_NAMES[1]!);
    expect("<h3>{redact(group.label)}</h3>").not.toMatch(RAW_NAMES[0]!);
  });
});
