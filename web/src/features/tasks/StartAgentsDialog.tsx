// Start an agent on every selected task: one machine, one repository, one agent for all of them — a worktree each,
// the branch and the prompt from each task — started one after another, since every worktree is made in turn.

import { useEffect, useMemo, useState } from "react";
import type { AgentKind, Machine } from "@/features/roster/api";
import { machineOptions } from "@/features/roster/options";
import { usePref } from "@/lib/prefs";
import { plural } from "@/lib/text";
import { Dialog, Icon, Logo, Notice, Select } from "@/ui";
import { startTask, type Project, type Task } from "./api";
import { runInSequence, startPlan, startWords, type Outcome } from "./bulk";
import { branchFor, taskPrompt } from "./prompt";
import { t } from "@/lib/i18n";

const SHOWN = 6; // the tasks listed in the dialog before "and N more"

export function StartAgentsDialog({ tasks, projects, agents, machines, currentPane, onClose, onDone }: {
  tasks: Task[];
  projects: Map<number, Project>;
  agents: AgentKind[];
  machines: Machine[];
  currentPane: { machineId: string; paneId: string } | null;
  onClose: () => void;
  onDone: (started: Task[], outcome: Outcome, skipped: Task[]) => void;
}) {
  const { starts, skipped } = useMemo(() => startPlan(tasks), [tasks]);
  const connected = machines.filter((m) => m.state === "connected" && m.sessions.some((s) => s.dir));
  const [remembered, setRemembered] = usePref<Record<string, { machine: string; workspace: string }>>("tasks.start", {});
  const projectKey = String(starts[0]?.project_id ?? tasks[0]?.project_id ?? 0);
  const last = remembered[projectKey];
  const [machine, setMachine] = useState(() => (connected.find((m) => m.id === last?.machine) ?? connected.find((m) => m.id === currentPane?.machineId) ?? connected[0])?.id ?? "");
  const workspaces = useMemo(() => {
    const m = machines.find((x) => x.id === machine);
    return (m?.sessions ?? []).filter((s) => s.dir && !s.worktree?.linked).map((s) => ({ value: s.id, label: s.name, hint: s.dir, icon: <Icon name="folder" size={14} /> }));
  }, [machines, machine]);
  const [workspace, setWorkspace] = useState(() => (workspaces.find((w) => w.value === last?.workspace) ?? workspaces[0])?.value ?? "");
  useEffect(() => {
    if (!workspaces.some((w) => w.value === workspace)) setWorkspace(workspaces[0]?.value ?? "");
  }, [workspaces, workspace]);
  const [agent, setAgent] = useState(() => (agents.some((a) => a.id === "claude") ? "claude" : (agents[0]?.id ?? "claude")));
  const [groom, setGroom] = useState(false);
  const [at, setAt] = useState(0); // how many starts have been asked for, while they run
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const ready = !!workspace && !!agent && starts.length > 0;

  const run = async () => {
    setBusy(true);
    setErr("");
    setRemembered({ ...remembered, [projectKey]: { machine, workspace } });
    const started: Task[] = [];
    let n = 0;
    const outcome = await runInSequence(starts, async (task) => {
      setAt(++n);
      const answer = await startTask(task.id, { machine, workspace, branch: branchFor(task), base: "", agent, groom, prompt: taskPrompt(task, projects.get(task.project_id), groom) });
      started.push(answer);
    });
    setBusy(false);
    if (started.length === 0 && outcome.failed > 0) {
      setErr(outcome.error);
      return;
    }
    onDone(started, outcome, skipped);
  };

  const listed = tasks.slice(0, SHOWN);
  return (
    <Dialog
      title={t("Start agents on {tasks}", { tasks: plural(tasks.length, "task") })}
      icon="git-branch"
      size="md"
      as="form"
      className="tasks-start"
      onClose={onClose}
      onSubmit={(e) => {
        e.preventDefault();
        if (ready && !busy) void run();
      }}
    >
      <p className="modal-hint">
        {t("A worktree branches off the repository for each task, the agent starts in it and gets that task's prompt once it is up; the worktrees are made one after another.")}
        {skipped.length > 0 && <> {t("{tasks} already run and are left alone.", { tasks: skipped.map((x) => x.identifier).join(", ") })}</>}
      </p>
      <ul className="tasks-start-list">
        {listed.map((task) => (
          <li key={task.id}>
            <code>{task.identifier}</code> {task.title}
          </li>
        ))}
        {tasks.length > SHOWN && <li className="quiet">{t("and {n} more", { n: tasks.length - SHOWN })}</li>}
      </ul>
      <div className="schedule-row">
        {connected.length > 1 && (
          <label>
            {t("Machine")}
            <Select value={machine} onChange={setMachine} options={machineOptions(connected, { onlyConnected: false })} />
          </label>
        )}
        <label className="grow">
          {t("Workspace")}
          <Select value={workspace} onChange={setWorkspace} placeholder={t("Pick a repository")} options={workspaces} />
        </label>
        <label>
          {t("Agent")}
          <Select value={agent} onChange={setAgent} options={agents.map((a) => ({ value: a.id, label: a.label, icon: <Logo agent={a.id} /> }))} />
        </label>
      </div>
      <label className="tasks-toggle">
        <input type="checkbox" checked={groom} onChange={(e) => setGroom(e.target.checked)} /> {t("groom first")} (<code>/grill-me</code>)
      </label>
      {err && <Notice tone="error">{err}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose} disabled={busy}>
          {t("Cancel")}
        </button>
        <button type="submit" className="btn primary" disabled={busy || !ready}>
          {busy ? t("Starting {n} of {total}…", { n: at, total: starts.length }) : t("Start {agents}", { agents: plural(starts.length, "agent") })}
        </button>
      </div>
    </Dialog>
  );
}
