// The triage of what agents proposed: the line over the list with Accept all and Decline all, the two controls on a row,
// and the hook that does the moves — an accepted task goes to its project's new-task column, a declined one is deleted.

import { useState } from "react";
import { actions } from "@/app/store";
import { errMsg, plural } from "@/lib/text";
import { deleteTask, patchTask, type Status, type Task } from "./api";
import { proposedWords } from "./proposed";
import { t as tr } from "@/lib/i18n";

export function useTriage({ columnFor, drop, refresh, onError }: {
  columnFor: (t: Task) => Status | undefined; // where an accepted task goes
  drop: (ids: number[]) => void; // the rows leave the list at once
  refresh: () => void;
  onError: (text: string) => void;
}) {
  const [busy, setBusy] = useState(false);
  const acceptOne = async (t: Task) => {
    const to = columnFor(t);
    if (!to) {
      onError(tr("“{title}”: its project has no column to accept it into", { title: t.title }));
      return false;
    }
    drop([t.id]);
    try {
      await patchTask(t.id, { status_id: to.id });
      return true;
    } catch (e) {
      onError(errMsg(e));
      return false;
    }
  };
  const accept = async (t: Task) => {
    await acceptOne(t);
    refresh();
  };
  const decline = (t: Task) =>
    actions.ask(tr("Decline “{title}”? The task is deleted.", { title: t.title }), tr("Decline"), () => {
      drop([t.id]);
      void deleteTask(t.id).then(refresh, (e) => {
        onError(errMsg(e));
        refresh();
      });
    });
  // all takes the tasks listed one after another and stops at the first refusal
  const all = (ts: Task[], what: "accept" | "decline") => {
    const run = async () => {
      setBusy(true);
      try {
        for (const t of ts) {
          if (what === "accept") {
            if (!(await acceptOne(t))) break;
          } else {
            drop([t.id]);
            await deleteTask(t.id);
          }
        }
      } catch (e) {
        onError(errMsg(e));
      } finally {
        setBusy(false);
        refresh();
      }
    };
    if (what === "decline") actions.ask(tr("Decline {proposals}? The tasks are deleted.", { proposals: plural(ts.length, "proposal") }), tr("Decline all"), () => void run());
    else void run();
  };
  return { busy, accept, decline, all };
}

// ProposedBar sits where the quick-add line would: how many wait, and the two moves for every row shown
export function ProposedBar({ count, busy, onAcceptAll, onDeclineAll }: { count: number; busy: boolean; onAcceptAll: () => void; onDeclineAll: () => void }) {
  return (
    <div className="proposed-bar">
      <span>{proposedWords(count)}</span>
      <button type="button" className="btn sm" disabled={count === 0 || busy} onClick={onAcceptAll}>
        {busy ? tr("Working…") : tr("Accept all")}
      </button>
      <button type="button" className="btn sm danger soft" disabled={count === 0 || busy} onClick={onDeclineAll}>
        {tr("Decline all…")}
      </button>
    </div>
  );
}

// ProposedActions are a row's own Accept and Decline, at its end
export function ProposedActions({ onAccept, onDecline }: { onAccept: () => void; onDecline: () => void }) {
  return (
    <span className="tasks-row-actions">
      <button
        type="button"
        className="btn sm"
        title={tr("Accept: the task goes to the project's New column")}
        onClick={(e) => {
          e.stopPropagation();
          onAccept();
        }}
      >
        {tr("Accept")}
      </button>
      <button
        type="button"
        className="btn sm danger soft"
        title={tr("Decline: the task is deleted")}
        onClick={(e) => {
          e.stopPropagation();
          onDecline();
        }}
      >
        {tr("Decline")}
      </button>
    </span>
  );
}
