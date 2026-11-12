import { describe, expect, it } from "vitest";
import overlays from "@/app/Overlays.tsx?raw";
import broadcast from "@/features/agents/BroadcastDialog.tsx?raw";
import send from "@/features/agents/SendToAgent.tsx?raw";
import arena from "@/features/arena/ArenaPanel.tsx?raw";
import teleport from "@/features/teleport/TeleportDialog.tsx?raw";
import triggerForm from "@/features/triggers/TriggerForm.tsx?raw";
import triggers from "@/features/triggers/TriggersDialog.tsx?raw";
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
const SOURCES = { overlays, broadcast, send, arena, teleport, everywhere, updates, making, picker, leftovers, mac, triggers, triggerForm };
const RAW_NAMES = [
  /(?<!redact\()\b(?:machine|target|group|chosen\?|x)\.label\b/, // a machine's own label, not through the mode
  /\{ workspace \}\)|workspace: session\.name\b/, // a workspace's own name in the words
  /(?<!redact\()session\.worktree\?\.repo_name/, // a repository's own name
  /(?<!key=)\{left\.(?:repo|name|path)\}/, // a leftover worktree's folder, said rather than a key
  /sessionName\}? › \$?\{\w+\.paneTitle/, // an agent by its workspace's own name and its conversation's title
  /group: a\.machineLabel|>\{label\}</, // a picker's machine group by its own label
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
    expect("label: `${a.sessionName} › ${a.paneTitle}`,").toMatch(RAW_NAMES[4]!);
    expect('<span className="pick-machine">{label}</span>').toMatch(RAW_NAMES[5]!);
  });
});
