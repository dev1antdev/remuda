// The card's rows in the workspace's and the folder's ⋯ menus: Note… on a checkout, and on a touch screen, which has no
// hover, Details — the hover card's note, pull requests, issues and ports as a popover beside the card.

import { actions } from "@/app/store";
import { isTouch } from "@/lib/prefs";
import { t } from "@/lib/i18n";
import type { MenuItem } from "@/ui";
import type { Machine, Session } from "./api";
import { askDetails } from "./cardAsk";
import { detailsOf, noteDir } from "./cardFacts";
import type { Folder } from "./workspaces";

// firstLine is a note's first line, for a menu row's hint
const firstLine = (text: string): string => text.split("\n", 1)[0] ?? "";

// noteRow opens a checkout's note to write or edit; none for a folder the screen-sharing mode hides
function noteRow(m: Machine, session: Session, name: string): MenuItem[] {
  const dir = noteDir(session);
  if (!dir) return [];
  const text = session.note ?? "";
  return [{ label: t("Note…"), icon: "note", hint: text ? firstLine(text) : t("anything to remember about this checkout"), onSelect: () => actions.openDialog({ kind: "note", machineId: m.id, dir, name, text }) }];
}

// cardDetailsRows is a workspace card's Details on a touch screen, when its card has anything to show
export function cardDetailsRows(m: Machine, session: Session, masked: boolean, leadPaneId?: string): MenuItem[] {
  if (masked || !isTouch || !detailsOf(session, leadPaneId, false).any) return [];
  return [{ label: t("Details"), icon: "info", hint: t("its note, pull requests, issues and ports"), onSelect: () => askDetails(m.id, session.id) }];
}

// cardNoteRows is a workspace card's Note…
export const cardNoteRows = (m: Machine, session: Session, masked: boolean): MenuItem[] => (masked ? [] : noteRow(m, session, session.name));

// folderNoteRows is a folder's Note…: the note of its main checkout, while a workspace stands on it
export function folderNoteRows(m: Machine, folder: Folder, masked: boolean): MenuItem[] {
  if (masked) return [];
  const main = folder.items.find(({ session }) => !session.worktree?.linked && (session.dir || "") === folder.path)?.session;
  return main ? noteRow(m, main, folder.name) : [];
}
