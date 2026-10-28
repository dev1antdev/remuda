// The open task on the right: every field in place, the ways to hand it to an agent, the run on it, the description, the comments.

import { useCallback, useState } from "react";
import { actions } from "@/app/store";
import { useNow } from "@/lib/clock";
import { checksTone, checksWords, prWords, reviewTone, reviewWords } from "@/features/roster/prStatus";
import { openUrl } from "@/lib/host";
import { errMsg } from "@/lib/text";
import { fmtDate } from "@/lib/time";
import type { BudgetStatus } from "@/features/usage/api";
import { budgetsFor, budgetTone, ratio, tightest, toneClass } from "@/features/usage/budgets";
import { Icon, Logo, Select, useDismiss } from "@/ui";
import { patchTask, PRIORITIES, type Field, type Label, type Project, type Run, type Status, type Task } from "./api";
import { CommentsField } from "./Comments";
import { FieldInput } from "./fields";
import { Html, htmlToText, textToHtml } from "./html";
import { issueLabel, issueLink } from "./issue";
import { LabelsField } from "./Labels";
import { DueInput, PrioMark, projectOption, statusOption } from "./pieces";
import { runWords } from "./runCost";
import type { RunPR } from "./runPR";

export function TaskDetail({ task: t, projects, statuses, fields, labels, onChange, onLabelsChanged, onClose, onDelete, onError, canSend, onSend, canStart, onStart, machineLabel, liveStatus, prOf, onOpenPane, budgets }: {
  task: Task;
  projects: Project[];
  statuses: Status[];
  fields: Field[];
  labels: Label[];
  onChange: (t: Task) => void;
  onLabelsChanged: () => void;
  onClose: () => void;
  onDelete: () => void;
  onError: (msg: string) => void;
  canSend: boolean;
  onSend: (groom: boolean) => void;
  canStart: boolean;
  onStart: () => void;
  machineLabel: (id: string) => string;
  liveStatus: (r: Run) => string | null; // the pane's status from the roster, null when it is not there
  prOf: (r: Run) => RunPR | null; // the pull request the run noted, with its state while the pane is there
  onOpenPane: (machineId: string, paneId: string) => void;
  budgets?: BudgetStatus[]; // the spending limits, for the tone of what the run spent
}) {
  const run = t.run ?? null;
  const now = useNow(15000);
  const spent = run ? runWords(run, now) : "";
  const tightBudget = run ? tightest(budgetsFor(budgets, run.machine, run.dir)) : null;
  const spentTone = tightBudget ? budgetTone(ratio(tightBudget)) : "";
  const issue = issueLink(t.description);
  const paneStatus = run ? liveStatus(run) : null;
  const pr = run ? prOf(run) : null;
  const runLabel = !run ? "" : run.state === "starting" ? "starting…" : run.state === "failed" ? "failed" : run.state === "gone" ? "closed" : paneStatus === "run" ? "working" : paneStatus === "input" ? "waiting for you" : paneStatus === null ? "closed" : run.state === "finished" ? "finished" : "running";
  const [title, setTitle] = useState(t.title);
  const [sendMenu, setSendMenu] = useState(false);
  const closeSendMenu = useCallback(() => setSendMenu(false), []);
  useDismiss(sendMenu, closeSendMenu, [".split"]);
  const [editingDesc, setEditingDesc] = useState(false);
  const [desc, setDesc] = useState("");

  const patch = async (fields: Partial<Task>) => {
    try {
      onChange(await patchTask(t.id, fields));
    } catch (e) {
      onError(errMsg(e));
    }
  };
  const saveTitle = () => {
    const v = title.trim();
    if (v && v !== t.title) void patch({ title: v });
    else setTitle(t.title);
  };
  const pickSend = (fn: () => void) => () => {
    setSendMenu(false);
    fn();
  };

  return (
    <aside className="tasks-detail">
      <div className="tasks-detail-head">
        <input
          type="checkbox"
          className="tasks-check"
          checked={t.done}
          onChange={() => void patch({ done: !t.done })}
          title={t.done ? "Mark undone" : "Mark done"}
          aria-label={t.done ? "Mark undone" : "Mark done"}
        />
        <span className="tasks-ident">{t.identifier}</span>
        {t.source && (
          <span className="tasks-source" title={`Made by an agent on ${t.source}`}>
            <Icon name="bot" size={12} /> by {t.source}
          </span>
        )}
        {issue && (
          <button type="button" className="linkish tasks-issue" title={issue} onClick={() => openUrl(issue)}>
            {issueLabel(issue)} <Icon name="external-link" size={12} />
          </button>
        )}
        <button type="button" className="icon-btn tasks-close" onClick={onClose} title="Close" aria-label="Close">
          <Icon name="x" size={16} />
        </button>
      </div>
      <input
        className="tasks-title"
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        onBlur={saveTitle}
        onKeyDown={(e) => {
          if (e.key === "Enter") (e.target as HTMLInputElement).blur();
          if (e.key === "Escape") setTitle(t.title);
        }}
      />

      <div className="tasks-field">
        <span className="tasks-field-name">Status</span>
        <Select value={t.status_id} onChange={(status_id) => void patch({ status_id })} options={statuses.map(statusOption)} />
      </div>
      <div className="tasks-field">
        <span className="tasks-field-name">Project</span>
        <Select
          value={t.project_id}
          onChange={(project_id) => void patch({ project_id })}
          options={projects.filter((p) => !p.is_archived || p.id === t.project_id).map(projectOption)}
        />
      </div>
      <div className="tasks-field">
        <span className="tasks-field-name">Due</span>
        <DueInput value={t.due_date} onCommit={(iso) => void patch({ due_date: iso })} />
        {t.due_date && (
          <button type="button" className="linkish" onClick={() => void patch({ due_date: "" })}>
            clear
          </button>
        )}
      </div>
      <div className="tasks-field">
        <span className="tasks-field-name">Priority</span>
        <Select
          value={t.priority}
          onChange={(priority) => void patch({ priority })}
          options={PRIORITIES.map((p, i) => ({ value: i, label: p, icon: <PrioMark level={i} /> }))}
        />
      </div>
      <LabelsField task={t} labels={labels} onChange={onChange} onLabelsChanged={onLabelsChanged} onError={onError} />
      {fields.map((f) => (
        <div key={f.id} className={`tasks-field kind-${f.kind}`}>
          <span className="tasks-field-name" title={f.in_prompt ? "Goes to the agent with the task" : undefined}>
            {f.title}
          </span>
          <FieldInput field={f} value={t.fields?.[String(f.id)]} onCommit={(v) => void patch({ fields: { [String(f.id)]: v } })} />
        </div>
      ))}

      <div className="tasks-send">
        <span className={`split${canSend ? "" : " off"}`} title={canSend ? "" : "No agent is running in herdr"}>
          <button type="button" className="split-main" disabled={!canSend} onClick={() => onSend(false)}>
            Send to agent
          </button>
          <button type="button" className="split-arrow" disabled={!canSend} onClick={() => setSendMenu((m) => !m)} title="More ways to send" aria-haspopup="menu" aria-expanded={sendMenu}>
            <Icon name="chevron-down" size={14} />
          </button>
          {sendMenu && (
            <span className="row-menu split-menu" role="menu">
              <button type="button" role="menuitem" onClick={pickSend(() => onSend(false))}>
                Send to agent
              </button>
              <button type="button" role="menuitem" onClick={pickSend(() => onSend(true))}>
                Send to agent for grooming
                <span className="split-hint">/grill-me</span>
              </button>
              <button type="button" role="menuitem" disabled={!canStart} onClick={pickSend(onStart)} title={canStart ? "" : "No connected machine has a workspace to branch off"}>
                Start an agent in a new worktree…
              </button>
            </span>
          )}
        </span>
        {!run && canStart && (
          <button type="button" className="linkish" onClick={onStart}>
            <Icon name="git-branch" size={12} /> Start an agent on it
          </button>
        )}
      </div>

      {run && (
        <div className={`tasks-run is-${run.state}`}>
          <span className="tasks-run-logo">
            <Logo agent={run.agent} />
          </span>
          <span className="tasks-run-copy">
            <span className="tasks-run-title">
              {run.agent} on <code>{run.branch}</code>
              {run.machine !== "local" && <> · {machineLabel(run.machine)}</>}
            </span>
            <span className="tasks-run-sub">
              {runLabel}
              {run.error && <> — {run.error}</>}
            </span>
            {spent && (
              <span className={`tasks-run-spent${toneClass(spentTone)}`} title="What this run spent at API prices, how long it took, the model">
                Agent run: {spent}
              </span>
            )}
            {pr && (
              <span className="tasks-run-pr">
                <button type="button" className="linkish" title={`${pr.url}\nOpen in a new tab`} onClick={() => openUrl(pr.url)}>
                  <Icon name="external-link" size={12} /> {pr.label}
                </button>
                {pr.status && (
                  <span className={`pr-state is-${checksTone(pr.status)}`} title={`${pr.label}: ${prWords(pr.status)} · read with gh on the machine`}>
                    <span className="pr-dot" />
                    {checksWords(pr.status)}
                  </span>
                )}
                {pr.status && reviewWords(pr.status) && <span className={`pr-state pr-review is-${reviewTone(pr.status)}`}>{reviewWords(pr.status)}</span>}
              </span>
            )}
          </span>
          {paneStatus !== null && (
            <button type="button" className="linkish" onClick={() => onOpenPane(run.machine, run.pane)}>
              Open <Icon name="chevron-right" size={12} />
            </button>
          )}
          {run.dir && (
            <button type="button" className="linkish" title="The worktree's changes in the Review mode" onClick={() => actions.openReview(run.machine, run.dir)}>
              <Icon name="circle-check" size={12} /> Review
            </button>
          )}
        </div>
      )}

      <div className="tasks-desc">
        <div className="tasks-field-name">
          Description
          {!editingDesc && (
            <button
              type="button"
              className="linkish"
              onClick={() => {
                setDesc(htmlToText(t.description));
                setEditingDesc(true);
              }}
            >
              <Icon name="edit" size={12} /> edit
            </button>
          )}
        </div>
        {editingDesc ? (
          <>
            <textarea value={desc} onChange={(e) => setDesc(e.target.value)} rows={8} autoFocus />
            <div className="modal-actions">
              <button type="button" className="btn" onClick={() => setEditingDesc(false)}>
                Cancel
              </button>
              <button
                type="button"
                className="btn primary"
                onClick={() => {
                  setEditingDesc(false);
                  void patch({ description: textToHtml(desc) });
                }}
              >
                Save
              </button>
            </div>
          </>
        ) : t.description ? (
          <div className="tasks-html">
            <Html html={t.description} />
          </div>
        ) : (
          <div className="placeholder small">No description.</div>
        )}
      </div>

      <CommentsField task={t} run={run} onChange={onChange} onError={onError} onMarkDone={() => void patch({ done: true })} />

      <div className="tasks-detail-foot">
        <span className="tasks-meta">
          created {fmtDate(t.created)}
          {t.done && t.done_at && <> · done {fmtDate(t.done_at)}</>}
        </span>
        <button type="button" className="linkish danger" onClick={onDelete}>
          <Icon name="trash" size={12} /> Delete
        </button>
      </div>
    </aside>
  );
}
