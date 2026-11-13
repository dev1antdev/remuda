// The review's draft and its ends: the comments on diff lines kept on this device per folder, sent to the agent on
// their own or under a verdict, and the silent look that only moves the baseline. The agent first, the baseline second:
// a refusal leaves everything as it was.

import { useCallback, useMemo, useState } from "react";
import { useAgentTargets } from "@/features/agents/useAgentTargets";
import { submitPrompt } from "@/features/roster/api";
import { t } from "@/lib/i18n";
import { usePref } from "@/lib/prefs";
import { errMsg, plural } from "@/lib/text";
import { markReviewed, type Review, type Verdict } from "./api";
import { commentsMessage, place, verdictMessage, type DraftComment, type VerdictAsk } from "./draft";
import type { NewComment } from "./Hunk";

const NO_COMMENTS: DraftComment[] = []; // one empty list for every file without comments: a new array would redraw its hunks

// Target is an agent a review goes to
export type Target = { machineId: string; paneId: string; agent: string; sessionName?: string };

export function useDraft({ machineId, dir, agent, review, files, onError, onDone }: { machineId: string; dir: string; agent: string; review: Review | null; files: string[]; onError: (text: string) => void; onDone: (text: string) => void }) {
  const agents = useAgentTargets();
  const [comments, setComments] = usePref<DraftComment[]>(`review.draft:${machineId}|${dir}`, []);
  const [pickingAgent, setPickingAgent] = useState(false);
  const [openVerdict, setOpenVerdict] = useState<Verdict | null>(null);
  const [marking, setMarking] = useState(false);
  const [sending, setSending] = useState(false);
  // stable, so a memoised hunk is left alone by a roster poll or a keystroke elsewhere
  const addComment = useCallback(
    (file: string, comment: NewComment) => setComments((before) => [...before, { ...comment, file, id: before.reduce((max, other) => Math.max(max, other.id), 0) + 1 }]),
    [setComments],
  );
  const removeComment = useCallback((id: number) => setComments((before) => before.filter((comment) => comment.id !== id)), [setComments]);
  const byFile = useMemo(() => {
    const out = new Map<string, DraftComment[]>();
    for (const comment of comments) out.set(comment.file, [...(out.get(comment.file) ?? []), comment]);
    return out;
  }, [comments]);
  const where = () => place(dir, review);
  const message = () => commentsMessage(comments, where());

  const markSilently = async (after: () => void) => {
    setMarking(true);
    try {
      await markReviewed(machineId, dir);
      after();
      onDone(t("Reviewed — the next look starts from here."));
    } catch (failure) {
      onError(errMsg(failure));
    } finally {
      setMarking(false);
    }
  };
  const sendComments = async (target: Target, text = message()) => {
    if (sending) return;
    setSending(true);
    try {
      await submitPrompt(target.machineId, target.paneId, text);
      setComments([]);
      setPickingAgent(false);
      const count = plural(comments.length, "comment");
      onDone(target.sessionName ? t("Sent {comments} to {agent} in {workspace}.", { comments: count, agent: target.agent, workspace: target.sessionName }) : t("Sent {comments} to {agent}.", { comments: count, agent: target.agent }));
    } finally {
      setSending(false);
    }
  };
  const sendVerdict = async (verdict: Verdict, target: Target, note: string, ask: VerdictAsk, after: () => void) => {
    await submitPrompt(target.machineId, target.paneId, verdictMessage(verdict, note, comments, where(), ask));
    await markReviewed(machineId, dir, verdict);
    setComments([]);
    setOpenVerdict(null);
    after();
    const said = verdict === "approved" ? t("Approved") : t("Rejected");
    const told = target.sessionName ? t("{verdict} — {agent} in {workspace} has been told", { verdict: said, agent: target.agent, workspace: target.sessionName }) : t("{verdict} — {agent} has been told", { verdict: said, agent: target.agent });
    onDone(`${told}${ask === "pr" ? t(" and asked to open a pull request") : ask === "commit" ? t(" and asked to commit") : ""}.`);
  };
  return {
    comments,
    clear: () => setComments([]),
    addComment,
    removeComment,
    forFile: (path: string) => byFile.get(path) ?? NO_COMMENTS,
    orphans: comments.filter((comment) => !files.includes(comment.file)),
    canComment: !!agent || agents.length > 0, // an agent to tell, this pane's or another
    message,
    sending,
    marking,
    markSilently,
    sendComments,
    sendVerdict,
    pickingAgent,
    setPickingAgent,
    openVerdict,
    setOpenVerdict,
    closeVerdict: () => setOpenVerdict(null),
  };
}

export type Draft = ReturnType<typeof useDraft>;
