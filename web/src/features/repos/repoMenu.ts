// A repository row's menu: fetch, pull and push in the order its ↑ ↓ suggests, a new agent or terminal there, its
// workspace's Changes, the editor, and its path and branch to copy.

import { act } from "@/app/act";
import { actions } from "@/app/store";
import { createSession, type Machine } from "@/features/roster/api";
import { onWindows } from "@/features/roster/system";
import { copyText } from "@/lib/host";
import { t } from "@/lib/i18n";
import type { IconName, MenuItem } from "@/ui";
import type { Repo } from "./api";
import { paneOf, syncOffers, workspaceOn } from "./repos";
import type { SyncAction } from "./syncApi";

const ICONS: Record<SyncAction, IconName> = { fetch: "refresh-cw", pull: "arrow-down-to-line", push: "arrow-up-from-line" };

const copy = (text: string) => void copyText(text).then((ok) => actions.notify(ok ? t("Copied {text}", { text }) : t("Could not copy"), { error: !ok, brief: ok }));

// repoMenu is the row's rows; machine is the roster's (absent while the roster does not list it), onAct runs an action
export function repoMenu(machineId: string, machine: Machine | undefined, repo: Repo, onAct: (action: SyncAction) => void): MenuItem[] {
  const session = machine ? workspaceOn(machine.sessions, repo.path) : undefined;
  const pane = session ? paneOf(session) : undefined;
  const items: MenuItem[] = syncOffers(repo).map((offer) => ({ label: offer.label, icon: ICONS[offer.action], onSelect: () => onAct(offer.action) }));
  if (items.length) items.push({ sep: true });
  items.push(
    {
      label: t("New agent here…"),
      icon: "plus",
      hint: t("a workspace on this folder"),
      onSelect: () => {
        actions.closePanel();
        actions.openDialog({ kind: "workspace", machineId, dir: repo.path });
      },
    },
    {
      label: t("New terminal here"),
      icon: "square-terminal",
      onSelect: () =>
        void act(
          createSession({ machine: machineId, name: repo.name, dir: repo.path, agent: "" }).then((made) => {
            actions.closePanel();
            actions.selectPane(machineId, made.pane);
          }),
        ),
    },
  );
  if (pane || (machine && !onWindows(machine))) items.push({ sep: true });
  if (pane)
    items.push({
      label: t("Open Changes"),
      icon: "columns",
      hint: t("its workspace's uncommitted work"),
      onSelect: () => {
        actions.closePanel();
        actions.selectPane(machineId, pane);
        actions.setLens("changes");
      },
    });
  if (machine && !onWindows(machine))
    items.push({
      label: t("Open in the editor"),
      icon: "code",
      onSelect: () => {
        actions.closePanel();
        actions.openInCode(machineId, repo.path, repo.path);
      },
    });
  items.push({ sep: true }, { label: t("Copy path"), icon: "copy", onSelect: () => copy(repo.path) });
  if (repo.branch) items.push({ label: t("Copy branch"), icon: "copy", onSelect: () => copy(repo.branch!) });
  return items;
}
