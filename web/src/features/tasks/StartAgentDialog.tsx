// Start an agent on a task: a worktree off a workspace's repository, Claude in it, the task's prompt once it is up.

import { useEffect, useMemo, useState } from "react";
import type { Machine } from "@/features/roster/api";
import { machineOptions } from "@/features/roster/options";
import { usePref } from "@/lib/prefs";
import { Dialog, Icon, Notice, Select, useSubmit } from "@/ui";
import { startTask, type Project, type Task } from "./api";
import { branchFor, taskPrompt } from "./prompt";
import { t as tr } from "@/lib/i18n";

export function StartAgentDialog({ task: t, project, moveTo, machines, currentPane, onClose, onStarted }: {
  task: Task;
  project: Project | undefined;
  moveTo?: string; // the column the task moves to, if the workflow has one
  machines: Machine[];
  currentPane: { machineId: string; paneId: string } | null;
  onClose: () => void;
  onStarted: (t: Task, where: string) => void;
}) {
  const connected = machines.filter((m) => m.state === "connected" && m.sessions.some((s) => s.dir));
  const [remembered, setRemembered] = usePref<Record<string, { machine: string; workspace: string }>>("tasks.start", {});
  const last = remembered[String(t.project_id)];
  const [machine, setMachine] = useState(() => (connected.find((m) => m.id === last?.machine) ?? connected.find((m) => m.id === currentPane?.machineId) ?? connected[0])?.id ?? "");
  const workspaces = useMemo(() => {
    const m = machines.find((x) => x.id === machine);
    // the repos themselves, not their worktrees: a worktree branches off its repo
    return (m?.sessions ?? []).filter((s) => s.dir && !s.worktree?.linked).map((s) => ({ value: s.id, label: s.name, hint: s.dir, icon: <Icon name="folder" size={14} /> }));
  }, [machines, machine]);
  const [workspace, setWorkspace] = useState(() => (workspaces.find((w) => w.value === last?.workspace) ?? workspaces[0])?.value ?? "");
  useEffect(() => {
    if (!workspaces.some((w) => w.value === workspace)) setWorkspace(workspaces[0]?.value ?? "");
  }, [workspaces, workspace]);
  const [branch, setBranch] = useState(branchFor(t));
  const [base, setBase] = useState("");
  const [groom, setGroom] = useState(false);
  const [prompt, setPrompt] = useState(() => taskPrompt(t, project, false));
  const [edited, setEdited] = useState(false);
  const toggleGroom = (on: boolean) => {
    setGroom(on);
    // an edited prompt keeps its words; only the prefix comes and goes
    setPrompt((p) => (edited ? (on ? (p.startsWith("/grill-me") ? p : `/grill-me ${p}`) : p.replace(/^\/grill-me\s*/, "")) : taskPrompt(t, project, on)));
  };
  const ws = workspaces.find((w) => w.value === workspace);
  const m = machines.find((x) => x.id === machine);
  const ready = !!workspace && !!branch.trim() && !!prompt.trim();
  const { busy, err, run } = useSubmit(async () => {
    setRemembered({ ...remembered, [String(t.project_id)]: { machine, workspace } });
    const task = await startTask(t.id, { machine, workspace, branch: branch.trim(), base: base.trim(), agent: "claude", groom, prompt });
    onStarted(task, `${ws?.label ?? tr("the workspace")}${m && m.id !== "local" ? ` on ${m.label}` : ""}`);
  });
  return (
    <Dialog
      title={<>Start an agent on {t.identifier}</>}
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
        A worktree branches off the workspace's repository, Claude starts in it and gets the task once it is up.{" "}
        {moveTo ? (
          <>
            The task moves to <b>{moveTo}</b>;
          </>
        ) : (
          "The task stays where it is;"
        )}{" "}
        {tr("every answer the agent finishes comes back here as a comment.")}
      </p>
      <div className="schedule-row">
        {connected.length > 1 && (
          <label>
            {tr("Machine")}
            <Select value={machine} onChange={setMachine} options={machineOptions(connected, { onlyConnected: false })} />
          </label>
        )}
        <label className="grow">
          {tr("Workspace")}
          <Select value={workspace} onChange={setWorkspace} placeholder={tr("Pick a repository")} options={workspaces} />
        </label>
      </div>
      <div className="schedule-row">
        <label className="grow">
          {tr("Branch")}
          <input value={branch} onChange={(e) => setBranch(e.target.value)} spellCheck={false} required />
        </label>
        <label className="grow">
          {tr("From")}
          <input value={base} onChange={(e) => setBase(e.target.value)} placeholder={tr("the current branch")} spellCheck={false} />
        </label>
      </div>
      <label className="tasks-toggle">
        <input type="checkbox" checked={groom} onChange={(e) => toggleGroom(e.target.checked)} /> groom first (<code>/grill-me</code>)
      </label>
      <label>
        {tr("Prompt")}
        <textarea
          rows={6}
          value={prompt}
          onChange={(e) => {
            setPrompt(e.target.value);
            setEdited(true);
          }}
        />
      </label>
      {err && <Notice tone="error">{err}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          {tr("Cancel")}
        </button>
        <button type="submit" className="btn primary" disabled={busy || !ready}>
          {busy ? tr("Starting…") : tr("Start")}
        </button>
      </div>
    </Dialog>
  );
}
