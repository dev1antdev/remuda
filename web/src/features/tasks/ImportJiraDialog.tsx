// A task from a Jira issue: its key or address and the project it lands in; read from the person's Jira site, the task
// names the issue and can get an agent at once.

import { useState } from "react";
import { importJira } from "@/features/jira/api";
import { keyOf } from "@/features/jira/jira";
import { useRoster } from "@/features/roster/useRoster";
import { useQuery } from "@/lib/query";
import { Dialog, Notice, Select, useSubmit } from "@/ui";
import { t } from "@/lib/i18n";
import { listProjects, type Task } from "./api";
import { projectOption } from "./pieces";

export default function ImportJiraDialog({ projectId, onClose, onCreated }: {
  projectId?: number; // the project picked on screen, if any
  onClose: () => void;
  onCreated: (task: Task, start: boolean) => void; // start: the person wants the agent dialog on it next
}) {
  const { jiraSite } = useRoster();
  const projects = useQuery(() => listProjects().then((all) => all.filter((p) => !p.is_archived)), []);
  const list = projects.data ?? [];
  const [picked, setPicked] = useState(projectId ?? 0);
  const project = list.some((p) => p.id === picked) ? picked : ((list.find((p) => p.title === "Inbox") ?? list[0])?.id ?? 0);
  const [typed, setTyped] = useState("");
  const key = keyOf(typed);
  const ready = !!jiraSite && !!key && project > 0;
  const { busy, err, run } = useSubmit(async (start: boolean) => {
    onCreated(await importJira(project, key), start);
  });
  return (
    <Dialog
      title={t("New task from a Jira issue")}
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
      {jiraSite ? (
        <p className="modal-hint">{t("The issue is read from {site} with the token you set in Settings; its summary, description and labels become a task that names it. Nothing is written to Jira.", { site: jiraSite })}</p>
      ) : (
        <Notice tone="hint">{t("Set your Jira site in Settings first.")}</Notice>
      )}
      <label>
        {t("Issue")}
        <input value={typed} onChange={(e) => setTyped(e.target.value)} placeholder={t("CRMOPS-6279, or the issue's address")} spellCheck={false} autoFocus required />
      </label>
      <label>
        {t("Project")}
        <Select value={project} onChange={setPicked} placeholder={projects.loading ? t("Loading…") : t("Pick a project")} options={list.map(projectOption)} />
      </label>
      {err && <Notice tone="error">{err}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button type="button" className="btn" disabled={busy || !ready} onClick={() => void run(true)}>
          {t("Create & start an agent…")}
        </button>
        <button type="submit" className="btn primary" disabled={busy || !ready}>
          {busy ? t("Reading…") : t("Create")}
        </button>
      </div>
    </Dialog>
  );
}
