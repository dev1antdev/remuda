// "Resume after restart", after Orca's hibernate mod: the chats a reboot stopped on a machine, one row each with a tick —
// the agent, its workspace and machine, the exact command that brings it back — then Not now, Resume N and Resume all,
// and the switch "Resume automatically next time". Closing it any other way counts as Not now.

import { useEffect, useState } from "react";
import { useRoster } from "@/features/roster/useRoster";
import { useSharing } from "@/features/roster/useSharing";
import { folderOf } from "@/features/roster/workspaces";
import { episodeKey, remember } from "@/features/roster/stopped";
import { resumeStopped } from "@/features/updates/recover";
import { t } from "@/lib/i18n";
import { usePref } from "@/lib/prefs";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { Dialog, Logo, Notice } from "@/ui";
import { fetchSleepSettings, saveSleepSettings } from "./api";
import { chatName, chatWhere, commandLine, someLabel, stillPicked, toggled } from "./resume";

export default function ResumeDialog({ machineId, onClose }: { machineId: string; onClose: () => void }) {
  const { machines } = useRoster();
  const share = useSharing();
  const machine = machines.find((m) => m.id === machineId);
  const stopped = machine?.stopped ?? [];
  const [picked, setPicked] = useState(() => stopped.map((chat) => chat.pane));
  const [seen, setSeen] = usePref<string[]>("recover.seen", []);
  const settings = useQuery(fetchSleepSettings, []);
  const [failure, setFailure] = useState("");
  useEffect(() => setPicked((now) => stillPicked(now, stopped)), [stopped.map((chat) => chat.pane).join()]); // eslint-disable-line react-hooks/exhaustive-deps
  const label = machine ? share.machineLabel(machine) : machineId;
  // the folder a stopped chat lived in hides it under the screen-sharing mode as its workspace would be hidden
  const masked = (pane: string) => {
    const session = machine?.sessions.find((s) => s.tabs.some((tab) => tab.panes.some((p) => p.id === pane)));
    return session && machine ? share.sessionHidden(machine.id, session) && { name: share.folderName(machine.id, folderOf(session), session.name) } : share.on && share.pref.all ? { name: "" } : false;
  };
  const notNow = () => {
    if (stopped.length) setSeen(remember(seen, episodeKey(machineId, stopped)));
    onClose();
  };
  const resume = (panes: string[]) => {
    setSeen(remember(seen, episodeKey(machineId, stopped)));
    onClose();
    void resumeStopped(machineId, panes.length, panes);
  };
  const flipAuto = async (on: boolean) => {
    setFailure("");
    try {
      const saved = await saveSleepSettings({ ...(settings.data ?? { wake_on_open: false, resume_automatically: false }), resume_automatically: on });
      settings.mutate(() => saved);
    } catch (e) {
      setFailure(errMsg(e));
    }
  };
  const some = someLabel(picked.length, stopped.length);
  return (
    <Dialog title={t("Resume after restart")} icon="refresh-cw" size="md" onClose={notNow} className="resume-dialog">
      <p className="modal-hint">{t("These were running before their panes came back as bare shells — a reboot, or herdr's server started again. Each comes back in its own pane, on its own conversation.")}</p>
      {machine?.resuming ? (
        <Notice tone="info">{t("They are being resumed right now.")}</Notice>
      ) : stopped.length === 0 ? (
        <Notice tone="hint">{t("Nothing waits on {machine}: every chat stopped there is back, or was left as it was.", { machine: label })}</Notice>
      ) : (
        <ul className="pick-list resume-list" aria-label={t("Chats stopped with the reboot")}>
          {stopped.map((chat) => {
            const hidden = masked(chat.pane);
            return (
              <li key={chat.pane}>
                <label className="pick-row resume-row">
                  <input type="checkbox" checked={picked.includes(chat.pane)} onChange={() => setPicked(toggled(picked, chat.pane))} />
                  <Logo agent={chat.agent} />
                  <span className="pick-name resume-chat">
                    <span className="resume-head">
                      <strong>{chatName(chat)}</strong>
                      <span className="resume-where">{chatWhere(hidden ? { ...chat, title: hidden.name } : chat, label)}</span>
                    </span>
                    <code className="resume-command">{commandLine(chat, !!hidden)}</code>
                  </span>
                </label>
              </li>
            );
          })}
        </ul>
      )}
      <p className="modal-hint">{t("Each agent comes back with its own resume command; a model or arguments it was started with are not carried over, since herdr does not keep them.")}</p>
      <label className="check-row resume-auto">
        <input type="checkbox" checked={!!settings.data?.resume_automatically} disabled={!settings.data} onChange={(e) => void flipAuto(e.target.checked)} />
        <span>{t("Resume automatically next time")}</span>
      </label>
      {failure && <Notice tone="error">{failure}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={notNow}>
          {t("Not now")}
        </button>
        {some && !machine?.resuming && (
          <button type="button" className="btn" onClick={() => resume(picked)}>
            {some}
          </button>
        )}
        <button type="button" className="btn primary" autoFocus disabled={!stopped.length || !!machine?.resuming} onClick={() => resume(stopped.map((chat) => chat.pane))}>
          {t("Resume all")}
        </button>
      </div>
    </Dialog>
  );
}
