// "Resume after restart", after Orca's hibernate mod: the chats a reboot stopped on a machine, one row each with a tick —
// the agent, its workspace and machine, the exact command that brings it back, "from herdr" for an agent herdr ran
// before the machine moved to remuda-core — then Not now, Resume N and Resume all, and the switch "Resume automatically
// next time". Closing it any other way counts as Not now. While herdr's server still runs there, Resume of herdr's
// agents is refused in place, with its stop offered behind a confirm.

import { useEffect, useState } from "react";
import { useRoster } from "@/features/roster/useRoster";
import { useSharing } from "@/features/roster/useSharing";
import { folderOf } from "@/features/roster/workspaces";
import { episodeKey, fromHerdr, remember } from "@/features/roster/stopped";
import { recoverChats } from "@/features/updates/api";
import { askStopHerdr, followResume } from "@/features/updates/recover";
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
  const [held, setHeld] = useState(false); // refused: herdr's server still runs there
  const [sending, setSending] = useState(false);
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
  const resume = async (panes: string[]) => {
    setSending(true);
    setFailure("");
    try {
      const answer = await recoverChats(machineId, panes);
      setHeld(!!answer.herdr);
      if (answer.job) {
        setSeen(remember(seen, episodeKey(machineId, stopped)));
        onClose();
        void followResume(answer.job, panes.length);
      }
    } catch (e) {
      setFailure(errMsg(e));
    } finally {
      setSending(false);
    }
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
      {stopped.some(fromHerdr) && <p className="modal-hint">{t("The ones from herdr ran there before the machine moved to remuda-core: each comes back in a tab of a workspace on its folder.")}</p>}
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
                      {fromHerdr(chat) && <span className="badge quiet">{t("from herdr")}</span>}
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
      {held && (
        <Notice
          tone="warn"
          action={
            <button type="button" className="btn sm" onClick={() => askStopHerdr(machineId, label, () => setHeld(false))}>
              {t("Stop herdr's server on {machine}…", { machine: label })}
            </button>
          }
        >
          {t("herdr's server still runs on {machine}, and its agents may still be on these conversations: stop it first, or each would run twice.", { machine: label })}
        </Notice>
      )}
      {failure && <Notice tone="error">{failure}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={notNow}>
          {t("Not now")}
        </button>
        {some && !machine?.resuming && (
          <button type="button" className="btn" disabled={sending} onClick={() => void resume(picked)}>
            {some}
          </button>
        )}
        <button type="button" className="btn primary" autoFocus disabled={!stopped.length || !!machine?.resuming || sending} onClick={() => void resume(stopped.map((chat) => chat.pane))}>
          {t("Resume all")}
        </button>
      </div>
    </Dialog>
  );
}
