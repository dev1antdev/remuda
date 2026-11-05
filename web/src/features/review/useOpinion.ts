// The folder's second opinion as the lens shows it: read every few seconds while the reviewer works, rarely
// otherwise; its open comments by file for the diff rows, the ones on files the lens does not list, and the adding to
// the person's draft and the dismissing, shown ahead of the server's answer.

import { useCallback, useEffect, useMemo, useState } from "react";
import { useRoster } from "@/features/roster/useRoster";
import { errMsg } from "@/lib/text";
import { useQuery } from "@/lib/query";
import { dismissOpinion, fetchOpinion } from "./api";
import type { NewComment } from "./Hunk";
import type { DiffRow } from "./hunks";
import { adoptedText, asksPerson, byFile, openComments, unlisted, type Opinion, type OpinionComment } from "./opinion";

const WHILE_RUNNING = 3000;
const OTHERWISE = 30000;

export function useOpinion({ machineId, dir, look, active, files, addComment, onError }: {
  machineId: string;
  dir: string;
  look: number; // a new look reads the opinion again
  active: boolean;
  files: string[]; // the paths the lens lists
  addComment: (file: string, comment: NewComment) => void;
  onError: (message: string) => void;
}) {
  const { agents, machines } = useRoster();
  const [polling, setPolling] = useState(OTHERWISE);
  const [asking, setAsking] = useState(false);
  const query = useQuery<Opinion | null>(() => fetchOpinion(machineId, dir), [machineId, dir, look], { every: polling, enabled: active });
  const { mutate, reload } = query;
  const opinion = query.data ?? null;
  const running = opinion?.state === "running";
  useEffect(() => setPolling(running ? WHILE_RUNNING : OTHERWISE), [running]);
  const reviewer = agents.find((agent) => agent.id === opinion?.agent)?.label ?? opinion?.agent ?? "";
  const reviewerPane = useMemo(() => {
    if (!running || !opinion) return undefined;
    const machine = machines.find((m) => m.id === opinion.machine);
    return machine?.sessions.flatMap((s) => s.tabs.flatMap((tab) => tab.panes)).find((pane) => pane.id === opinion.pane);
  }, [running, opinion, machines]);
  const askingPerson = asksPerson(reviewerPane);
  const open = useMemo(() => openComments(opinion), [opinion]);
  const perFile = useMemo(() => byFile(open), [open]);
  const apart = useMemo(() => unlisted(open, files), [open, files]);

  const dismiss = useCallback(
    (ids: number[]) => {
      mutate((op) => (op ? { ...op, comments: op.comments.map((c) => (ids.length === 0 || ids.includes(c.id) ? { ...c, dismissed: true } : c)) } : op));
      dismissOpinion(machineId, dir, ids).then(
        (op) => mutate(() => op),
        (failure) => {
          onError(errMsg(failure));
          reload();
        },
      );
    },
    [machineId, dir, mutate, reload, onError],
  );
  // adopt puts the comment in the person's draft — under its row when it has one — and takes it out of the opinion
  const adopt = useCallback(
    (comment: OpinionComment, row?: DiffRow) => {
      addComment(comment.path, { line: comment.line, removed: false, code: row?.text ?? "", text: adoptedText(comment, reviewer) });
      dismiss([comment.id]);
    },
    [addComment, dismiss, reviewer],
  );
  const adoptAll = useCallback(() => {
    for (const comment of open) addComment(comment.path, { line: comment.line, removed: false, code: "", text: adoptedText(comment, reviewer) });
    dismiss([]);
  }, [open, addComment, dismiss, reviewer]);
  const dismissOne = useCallback((id: number) => dismiss([id]), [dismiss]);
  const dismissAll = useCallback(() => dismiss([]), [dismiss]);

  return { opinion, reviewer, askingPerson, perFile, apart, adopt, adoptAll, dismissOne, dismissAll, asking, setAsking, reload, mutate };
}
