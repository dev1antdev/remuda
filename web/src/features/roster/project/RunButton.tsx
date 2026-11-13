// The run button, after T3 Code's: "▶ Dev | ▾" in the tab strip — the main part runs the action used last in a shell
// herdr makes for it, the menu lists every action of the repository, Add action… and Edit actions…; "▶ Add action" while
// there is none. On a phone the words go and the two icons keep their room.

import { useState } from "react";
import { act } from "@/app/act";
import { actions } from "@/app/store";
import { usePref } from "@/lib/prefs";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { Icon, Menu, Tip, type MenuItem } from "@/ui";
import { t } from "@/lib/i18n";
import { fetchProject, runAction, saved, saveProject } from "./api";
import ActionDialog from "./ActionDialog";
import { actionHint, lastKey, primary, withAction, type Action } from "./project";

export default function RunButton({ machineId, workspace, pane, root, masked = false }: { machineId: string; workspace: string; pane: string; root: string; masked?: boolean }) {
  const n = saved.use((s) => s.n);
  const project = useQuery(() => fetchProject(machineId, root, true), [machineId, root, n], { keep: true });
  const list = project.data?.settings.actions ?? [];
  const [last, setLast] = usePref(lastKey(machineId, root), "");
  const [adding, setAdding] = useState(false);
  const main = primary(list, last);

  const run = async (action: Action) => {
    setLast(action.id);
    try {
      const made = await runAction({ machine: machineId, workspace, pane, root, action: action.id });
      actions.selectPane(machineId, made.pane);
      actions.notify(t("Running “{name}”", { name: action.name }), { brief: true });
    } catch (e) {
      actions.notify(errMsg(e), { error: true });
    }
  };
  const add = async (action: Action) => {
    setAdding(false);
    const settings = project.data?.settings ?? {};
    await act(saveProject(machineId, root, { ...settings, actions: withAction(settings.actions, action) }));
  };
  const rows = (): MenuItem[] => [
    ...list.map((a): MenuItem => ({ label: a.name, icon: a.icon ?? "play", hint: actionHint(a), checked: a.id === main?.id, onSelect: () => void run(a) })),
    ...(list.length ? [{ sep: true } as MenuItem] : []),
    { label: t("Add action…"), icon: "plus", onSelect: () => setAdding(true) },
    { label: t("Edit actions…"), icon: "settings", hint: t("in the project's settings"), onSelect: () => actions.openDialog({ kind: "project-settings", machineId, root }) },
  ];
  const label = main ? t("Run {name}", { name: main.name }) : t("Add a run action");
  return (
    <div className="run-split">
      <Tip text={main ? `${label} · ${actionHint(main)}` : label}>
        <button type="button" className="btn sm run-main" aria-label={label} onClick={() => (main ? void run(main) : setAdding(true))}>
          <Icon name={main?.icon ?? "play"} size={14} />
          <span className="run-name">{main ? (masked ? t("Run") : main.name) : t("Add action")}</span>
        </button>
      </Tip>
      <Menu
        label={t("Run actions")}
        tip={t("Run actions")}
        items={rows}
        trigger={
          <button type="button" className="btn sm run-more" aria-label={t("Run actions")}>
            <Icon name="chevron-down" size={12} />
          </button>
        }
      />
      {adding && <ActionDialog action={null} onClose={() => setAdding(false)} onSave={(a) => void add(a)} />}
    </div>
  );
}
