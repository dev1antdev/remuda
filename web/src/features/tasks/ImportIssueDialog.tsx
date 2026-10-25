// A task from a GitHub issue: its address, the project it lands in, the machine whose gh reads it as you; the task can get an agent at once.

import { useState } from "react";
import { machineOptions } from "@/features/roster/options";
import { useRoster } from "@/features/roster/useRoster";
import { useQuery } from "@/lib/query";
import { Dialog, Notice, Select, useSubmit } from "@/ui";
import { importIssue, listProjects, type Task } from "./api";
import { isIssueURL } from "./issue";
import { projectOption } from "./pieces";

export default function ImportIssueDialog({ projectId, onClose, onCreated }: {
  projectId?: number; // the project picked on screen, if any
  onClose: () => void;
  onCreated: (task: Task, start: boolean) => void; // start: the person wants the agent dialog on it next
}) {
  const { machines } = useRoster();
  const connected = machines.filter((m) => m.state === "connected");
  const [machine, setMachine] = useState(() => (connected.find((m) => m.id === "local") ?? connected[0])?.id ?? "local");
  const projects = useQuery(() => listProjects().then((all) => all.filter((p) => !p.is_archived)), []);
  const list = projects.data ?? [];
  const [picked, setPicked] = useState(projectId ?? 0);
  const project = list.some((p) => p.id === picked) ? picked : (list.find((p) => p.title === "Inbox") ?? list[0])?.id ?? 0;
  const [url, setUrl] = useState("");
  const ready = isIssueURL(url) && project > 0 && !!machine;
  const { busy, err, run } = useSubmit(async (start: boolean) => {
    const task = await importIssue(machine, project, url.trim());
    onCreated(task, start);
  });
  const where = machines.find((m) => m.id === machine);
  return (
    <Dialog
      title="New task from a GitHub issue"
      icon="link"
      size="md"
      as="form"
      className="tasks-import"
      onClose={onClose}
      onSubmit={(e) => {
        e.preventDefault();
        if (ready && !busy) void run(false);
      }}
    >
      <p className="modal-hint">
        The issue is read with <code>gh</code> on {where && where.id !== "local" ? where.label : "this machine"}, as you; its title, text and labels become a task
        that names it, so an agent started on the task knows where it came from. Nothing is written to GitHub.
      </p>
      <label>
        Issue
        <input
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder="https://github.com/owner/repo/issues/123"
          spellCheck={false}
          autoFocus
          required
          inputMode="url"
        />
      </label>
      <div className="schedule-row">
        <label className="grow">
          Project
          <Select value={project} onChange={setPicked} placeholder={projects.loading ? "Loading…" : "Pick a project"} options={list.map(projectOption)} />
        </label>
        {connected.length > 1 && (
          <label>
            Machine
            <Select value={machine} onChange={setMachine} options={machineOptions(connected)} />
          </label>
        )}
      </div>
      {err && <Notice tone="error">{err}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          Cancel
        </button>
        <button type="button" className="btn" disabled={busy || !ready} onClick={() => void run(true)}>
          Create &amp; start an agent…
        </button>
        <button type="submit" className="btn primary" disabled={busy || !ready}>
          {busy ? "Reading…" : "Create"}
        </button>
      </div>
    </Dialog>
  );
}
