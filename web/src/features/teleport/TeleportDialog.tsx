// Move an agent's work to another machine: the branch, its commits and the uncommitted changes land in a new worktree
// of a clone of the repository there, and an agent carries on with the gist of this conversation. The work here and
// its agent stay as they are; once the move is done the dialog offers to open the new agent and to close this one.

import { useEffect, useRef, useState } from "react";
import { act } from "@/app/act";
import { actions } from "@/app/store";
import { closeTab, type AgentKind } from "@/features/roster/api";
import { DEFAULT_TURNS, turnOptions } from "@/features/chat/handover";
import { fetchJob, type Job } from "@/features/updates/api";
import { errMsg } from "@/lib/text";
import { Dialog, Logo, Notice, Select } from "@/ui";
import { t } from "@/lib/i18n";
import { fetchMoved, fetchTargets, startMove, type Moved, type Targets } from "./api";
import { TYPED, firstFolder, firstTarget, folderChoices, folderOf, targetHint, whyNot } from "./teleport";

const POLL = 1000;

export default function TeleportDialog({ machineId, paneId, tabId, workspace, agent, agents, onClose }: {
  machineId: string;
  paneId: string;
  tabId: string; // the source's tab, offered for closing once the work is moved
  workspace: string;
  agent: string; // the source pane's agent: the default for the one carrying on
  agents: AgentKind[];
  onClose: () => void;
}) {
  const [found, setFound] = useState<Targets | null>(null);
  const [error, setError] = useState("");
  const [target, setTarget] = useState("");
  const [choice, setChoice] = useState(TYPED);
  const [typed, setTyped] = useState("");
  const [kind, setKind] = useState(agent || agents[0]?.id || "");
  const [turns, setTurns] = useState(DEFAULT_TURNS);
  const [job, setJob] = useState<Job | null>(null);
  const [moved, setMoved] = useState<Moved | null>(null);
  const log = useRef<HTMLPreElement>(null);

  useEffect(() => {
    let live = true;
    fetchTargets(machineId, paneId).then(
      (got) => {
        if (!live) return;
        setFound(got);
        const first = firstTarget(got.targets, machineId);
        setTarget(first);
        setChoice(firstFolder(got.targets.find((x) => x.machine === first)));
      },
      (e) => live && setError(errMsg(e)),
    );
    return () => {
      live = false;
    };
  }, [machineId, paneId]);

  useEffect(() => {
    if (!job || job.done) return;
    const id = setTimeout(() => {
      fetchJob(job.id).then(
        async (next) => {
          setJob(next);
          if (next.done) setMoved(await fetchMoved(next.id).catch(() => null));
        },
        (e) => setError(errMsg(e)),
      );
    }, POLL);
    return () => clearTimeout(id);
  }, [job]);

  useEffect(() => {
    if (log.current) log.current.scrollTop = log.current.scrollHeight;
  }, [job?.output]);

  const chosen = found?.targets.find((x) => x.machine === target);
  const folder = folderOf(choice, typed);
  const blocked = found ? whyNot(chosen, folder, kind) : "";
  const pickTarget = (next: string) => {
    setTarget(next);
    setChoice(firstFolder(found?.targets.find((x) => x.machine === next)));
  };
  const move = async () => {
    setError("");
    try {
      const { job: id } = await startMove({ machine: machineId, pane: paneId, target, folder, agent: kind, turns });
      setJob({ id, machine: target, title: "", command: "", started: new Date().toISOString(), done: false, ok: false, output: "" });
    } catch (e) {
      setError(errMsg(e));
    }
  };
  const open = () => {
    if (!moved?.pane) return;
    onClose();
    actions.openPane(moved.machine, moved.pane);
  };
  const closeSource = () =>
    actions.ask(t("Close the tab of {workspace} here? Its agent stops; the work itself stays in its folder.", { workspace }), t("Close tab"), () => void act(closeTab(machineId, tabId)));

  const label = agents.find((a) => a.id === kind)?.label ?? kind;
  return (
    <Dialog title={t("Move the work to another machine")} icon="send" size="md" onClose={onClose}>
      {!found && !error && <p className="modal-hint">{t("Reading the work in {workspace}…", { workspace })}</p>}
      {found && (
        <p className="modal-hint">
          {agent
            ? t("{words}: a new worktree of a clone there gets the branch, its commits and the uncommitted changes as they are; {agent} carries on with the gist of this conversation. Nothing here changes.", { words: found.words, agent: label })
            : t("{words}: a new worktree of a clone there gets the branch, its commits and the uncommitted changes as they are; {agent} starts there, told where the work came from. Nothing here changes.", { words: found.words, agent: label })}
        </p>
      )}
      {found && !job && (
        <>
          <label>
            {t("Machine")}
            <Select value={target} onChange={pickTarget} options={found.targets.map((x) => ({ value: x.machine, label: x.label, hint: targetHint(x) }))} />
          </label>
          <label>
            {t("Clone of the repository there")}
            <Select value={choice} onChange={setChoice} options={folderChoices(chosen)} />
          </label>
          {choice === TYPED && (
            <label>
              {t("Folder")}
              <input value={typed} placeholder="/Users/me/Projects/app" spellCheck={false} onChange={(e) => setTyped(e.target.value)} />
            </label>
          )}
          <label>
            {t("Agent")}
            <Select value={kind} onChange={setKind} options={agents.map((a) => ({ value: a.id, label: a.label, icon: <Logo agent={a.id} /> }))} />
          </label>
          <label>
            {t("In full")}
            <Select value={turns} onChange={setTurns} options={turnOptions()} />
          </label>
        </>
      )}
      {job && <pre className="updates-log" ref={log}>{job.output || t("Starting…")}</pre>}
      {job?.done && job.ok && <Notice tone="success">{t("Moved: {agent} carries on on {machine}. The agent here keeps running until you close it.", { agent: label, machine: chosen?.label ?? target })}</Notice>}
      {job?.done && !job.ok && <Notice tone="error">{job.error || t("The move stopped")}</Notice>}
      {error && <Notice tone="error">{error}</Notice>}
      {found && !job && blocked && <p className="modal-hint">{blocked}</p>}
      <div className="modal-actions">
        {job?.done && job.ok && tabId && (
          <button type="button" className="btn danger" onClick={closeSource}>
            {t("Close the agent here…")}
          </button>
        )}
        <button type="button" className="btn" onClick={onClose}>
          {job?.done || !job ? t("Close") : t("Hide")}
        </button>
        {!job && (
          <button type="button" className="btn primary" disabled={!found || !!blocked} onClick={() => void move()}>
            {t("Move to {machine}", { machine: chosen?.label ?? "…" })}
          </button>
        )}
        {job?.done && job.ok && moved?.pane && (
          <button type="button" className="btn primary" onClick={open}>
            {t("Open it there")}
          </button>
        )}
      </div>
    </Dialog>
  );
}
