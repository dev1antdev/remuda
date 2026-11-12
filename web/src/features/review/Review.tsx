// The Changes lens: a workspace folder's uncommitted work and the commits since the last look, the diffs in one
// column or two with the changed words marked, walked by keys, with comments on diff lines gathered into a draft
// that goes to the pane's agent, on its own or under a verdict.

import { useCallback, useMemo, useRef, useState } from "react";
import { actions, app } from "@/app/store";
import SendToAgent from "@/features/agents/SendToAgent";
import { useAgentTargets } from "@/features/agents/useAgentTargets";
import { fsText } from "@/features/files/api";
import { submitPrompt } from "@/features/roster/api";
import { isTouch, usePref } from "@/lib/prefs";
import { useQuery } from "@/lib/query";
import { basename, errMsg, plural } from "@/lib/text";
import { ago } from "@/lib/time";
import { Icon, Notice, useTimedNotice } from "@/ui";
import { fetchReview, fetchReviewDiff, markReviewed, type Review as ReviewData, type ReviewFile, type Verdict } from "./api";
import { FileRow, type NewComment } from "./Diff";
import { DraftBar } from "./DraftBar";
import { Note } from "./Hunk";
import { useReviewKeys } from "./keys";
import { commentsMessage, place, verdictMessage, type DraftComment, type VerdictAsk } from "./draft";
import { VerdictDialog } from "./VerdictDialog";
import { subWords, type OpinionComment } from "./opinion";
import { OpinionBlock } from "./OpinionBlock";
import { OpinionDialog } from "./OpinionDialog";
import { ReviewHead } from "./ReviewHead";
import { CollisionNotice } from "./CollisionNotice";
import type { SecretFinding } from "./secrets";
import { useOpinion } from "./useOpinion";
import { useSecretCheck } from "./useSecretCheck";
import { t } from "@/lib/i18n";

export type { DraftComment } from "./draft";

const NO_COMMENTS: DraftComment[] = []; // one empty list for every file without comments: a new array would redraw its hunks
const NO_OPINION: OpinionComment[] = [];
const NO_SECRETS: SecretFinding[] = [];

export default function Review({ machineId, paneId, dir, agent, onSeen, active = true }: {
  machineId: string;
  paneId: string;
  dir: string;
  agent: string; // "" for a shell pane
  onSeen?: () => void;
  active?: boolean; // the lens on screen: its keys listen
}) {
  const agents = useAgentTargets();
  const currentPane = { machineId, paneId };
  const onOpenFile = (path: string, line?: number) => actions.openFile(machineId, paneId, { path, line });
  const [draft, setDraft] = usePref<DraftComment[]>(`review.draft:${machineId}|${dir}`, []);
  const [split, setSplit] = usePref("review.split", !isTouch);
  const rootRef = useRef<HTMLDivElement>(null);
  const [pickingAgent, setPickingAgent] = useState(false);
  const [openVerdict, setOpenVerdict] = useState<Verdict | null>(null);
  const [actionError, setActionError] = useState("");
  const [openFile, setOpenFile] = useState<string | null>(null);
  const [showUnchanged, setShowUnchanged] = useState(false);
  const [look, setLook] = useState(0); // a new look remounts the rows, which re-read their diffs
  const undone = app.use((s) => s.undone); // an undo from the page changed a folder: this one is looked at again
  const [undoneSeen, setUndoneSeen] = useState(undone);
  if (undone !== undoneSeen) {
    setUndoneSeen(undone);
    setLook((count) => count + 1);
  }
  const [marking, setMarking] = useState(false);
  const [sending, setSending] = useState(false);
  const [notice, showNotice] = useTimedNotice(5000);

  const { data: review, error: fetchError } = useQuery<ReviewData>(() => fetchReview(machineId, dir), [machineId, dir, look]);
  const error = actionError || fetchError || review?.error || "";
  const lookAgain = () => setLook((count) => count + 1);
  const loadDiff = useCallback((path: string) => fetchReviewDiff(machineId, dir, path), [machineId, dir]); // stable: every row's query keys on it
  const loadSource = useCallback((path: string) => fsText(machineId, `${dir}/${path}`), [machineId, dir]); // the file itself, for the lines around a hunk

  // stable, so a memoised hunk is left alone by a roster poll or a keystroke elsewhere
  const addComment = useCallback(
    (file: string, comment: NewComment) => setDraft((comments) => [...comments, { ...comment, file, id: comments.reduce((max, other) => Math.max(max, other.id), 0) + 1 }]),
    [setDraft],
  );
  const removeComment = useCallback((id: number) => setDraft((comments) => comments.filter((comment) => comment.id !== id)), [setDraft]);
  const draftByFile = useMemo(() => {
    const byFile = new Map<string, DraftComment[]>();
    for (const comment of draft) byFile.set(comment.file, [...(byFile.get(comment.file) ?? []), comment]);
    return byFile;
  }, [draft]);
  const where = () => place(dir, review);
  const listed = useMemo(() => (review?.files ?? []).map((file) => file.path), [review]);
  const second = useOpinion({ machineId, dir, look, active, files: listed, addComment, onError: setActionError });
  const check = useSecretCheck(machineId, dir);

  const markReviewedSilently = async () => {
    setMarking(true);
    try {
      await markReviewed(machineId, dir);
      setOpenFile(null);
      lookAgain();
      onSeen?.();
      showNotice(t("Reviewed — the next look starts from here."));
    } catch (failure) {
      setActionError(errMsg(failure));
    } finally {
      setMarking(false);
    }
  };
  const sendComments = async (target: { machineId: string; paneId: string; agent: string; sessionName?: string }, text = commentsMessage(draft, where())) => {
    if (sending) return;
    setSending(true);
    try {
      await submitPrompt(target.machineId, target.paneId, text);
      setDraft([]);
      setPickingAgent(false);
      const comments = plural(draft.length, "comment");
      showNotice(target.sessionName ? t("Sent {comments} to {agent} in {workspace}.", { comments, agent: target.agent, workspace: target.sessionName }) : t("Sent {comments} to {agent}.", { comments, agent: target.agent }));
    } finally {
      setSending(false);
    }
  };
  // the agent first, the baseline second: a refusal leaves everything as it was
  const sendVerdict = async (verdict: Verdict, target: { machineId: string; paneId: string; agent: string; sessionName?: string }, note: string, ask: VerdictAsk) => {
    await submitPrompt(target.machineId, target.paneId, verdictMessage(verdict, note, draft, where(), ask));
    await markReviewed(machineId, dir, verdict);
    setDraft([]);
    setOpenVerdict(null);
    check.clear();
    setOpenFile(null);
    lookAgain();
    onSeen?.();
    const said = verdict === "approved" ? t("Approved") : t("Rejected");
    const told = target.sessionName ? t("{verdict} — {agent} in {workspace} has been told", { verdict: said, agent: target.agent, workspace: target.sessionName }) : t("{verdict} — {agent} has been told", { verdict: said, agent: target.agent });
    showNotice(`${told}${ask === "pr" ? t(" and asked to open a pull request") : ask === "commit" ? t(" and asked to commit") : ""}.`);
  };

  const files = review?.files ?? [];
  const commits = review?.commits ?? [];
  const lastLook = review?.baseline ? new Date(review.baseline.at) : null;
  const freshFiles = lastLook ? files.filter((file) => file.fresh) : files;
  const unchangedFiles = lastLook ? files.filter((file) => !file.fresh) : [];
  const keys = useReviewKeys({
    active,
    root: rootRef,
    files: [...freshFiles, ...(showUnchanged ? unchangedFiles : [])].map((file) => file.path),
    openFile,
    setOpenFile,
    onOpen: (path, line) => onOpenFile(`${dir}/${path}`, line),
    toggleSplit: () => setSplit((on) => !on),
  });
  // a finding clicked in the verdict dialog: the dialog gives way to its row, opened, in view and focused
  const showFinding = (finding: SecretFinding) => {
    setOpenVerdict(null);
    if (unchangedFiles.some((file) => file.path === finding.file)) setShowUnchanged(true);
    keys.reveal(finding.file, finding.line);
  };

  if (review === null && !error) return <div className="empty">{t("Looking at the folder…")}</div>;
  if (review && !review.repo) return <div className="empty">{t("{folder} is not a git checkout — nothing to review.", { folder: basename(dir) })}</div>;
  const lastVerdict = review?.baseline?.verdict;
  const lastLookText = !lastLook ? t("not reviewed yet") : lastVerdict === "approved" ? t("approved {ago}", { ago: ago(lastLook) }) : lastVerdict === "changes" ? t("rejected {ago}", { ago: ago(lastLook) }) + (review?.baseline?.ready ? t(" · ready again") : "") : t("since you looked {ago}", { ago: ago(lastLook) });
  const orphanComments = draft.filter((comment) => !files.some((file) => file.path === comment.file));
  const hasAgentToTell = !!agent || agents.length > 0;
  const fileRow = (file: ReviewFile) => (
    <FileRow
      key={`${file.path}|${look}`}
      file={file}
      open={openFile === file.path}
      split={split}
      onToggle={() => setOpenFile(openFile === file.path ? null : file.path)}
      load={loadDiff}
      loadSource={loadSource}
      onLoaded={keys.loaded}
      canComment={hasAgentToTell}
      draft={draftByFile.get(file.path) ?? NO_COMMENTS}
      onComment={addComment}
      onRemove={removeComment}
      opinion={second.perFile.get(file.path) ?? NO_OPINION}
      reviewer={second.reviewer}
      onAdopt={second.adopt}
      onDismissOpinion={second.dismissOne}
      secrets={check.marks.get(file.path) ?? NO_SECRETS}
      onOpen={(line) => onOpenFile(`${dir}/${file.path}`, line)}
      onCode={() => actions.openInCode(machineId, dir, `${dir.replace(/\/+$/, "")}/${file.path}`)}
    />
  );

  return (
    <div className="review" ref={rootRef}>
      <ReviewHead
        review={review}
        lastLookText={lastLookText}
        opinionWords={subWords(second.opinion)}
        split={split}
        onSplit={() => setSplit((on) => !on)}
        onLookAgain={lookAgain}
        opinionRunning={second.running}
        onAskOpinion={() => second.setAsking(true)}
        onUndo={() => actions.openDialog({ kind: "undo-turn", machineId, dir })}
        marking={marking}
        onMark={() => void markReviewedSilently()}
      />
      {error && (
        <Notice tone="error" onDismiss={() => setActionError("")}>
          {error}
        </Notice>
      )}
      {notice && <Notice tone="success">{notice}</Notice>}
      <div className="review-body">
        <CollisionNotice machineId={machineId} dir={dir} />
        {second.opinion && (
          <OpinionBlock
            opinion={second.opinion}
            reviewer={second.reviewer}
            asking={second.askingPerson}
            apart={second.apart}
            moved={second.moved}
            onAdopt={second.adopt}
            onAdoptAll={second.adoptAll}
            onDismiss={second.dismissOne}
            onDismissAll={second.dismissAll}
          />
        )}
        {commits.length > 0 && (
          <div className="review-commits">
            {commits.map((commit) => (
              <div key={commit.sha} className={`review-commit${commit.seen ? " seen" : ""}`} title={commit.seen ? t("A commit of work you already reviewed") : undefined}>
                <code>{commit.sha}</code>
                <span className="review-commit-subject">{commit.subject}</span>
                {commit.seen && <span className="review-tag">{t("reviewed")}</span>}
                <span className="review-commit-when">{ago(commit.when * 1000)}</span>
              </div>
            ))}
          </div>
        )}
        {files.length === 0 && <div className="empty">{commits.length > 0 ? t("Everything here is committed.") : lastLook ? t("Nothing has changed here since you looked.") : t("Nothing is uncommitted here.")}</div>}
        {freshFiles.map((file) => fileRow(file))}
        {unchangedFiles.length > 0 && (
          <div className="review-fold">
            <button type="button" className="linkish" aria-expanded={showUnchanged} onClick={() => setShowUnchanged((shown) => !shown)}>
              <Icon name={showUnchanged ? "chevron-down" : "chevron-right"} size={12} /> {t("{files} unchanged since you looked", { files: plural(unchangedFiles.length, "file") })}
            </button>
            {showUnchanged && unchangedFiles.map((file) => fileRow(file))}
          </div>
        )}
        {review && orphanComments.length > 0 && (
          <div className="review-orphans">
            <Notice tone="hint">{t("Comments on files no longer in this list — still in the draft:")}</Notice>
            {orphanComments.map((comment) => (
              <Note key={comment.id} comment={comment} where={comment.line > 0 ? `${comment.file}:${comment.line}` : comment.file} onRemove={removeComment} />
            ))}
          </div>
        )}
      </div>
      {review && hasAgentToTell && (files.length > 0 || commits.length > 0 || draft.length > 0) && (
        <DraftBar
          comments={draft.length}
          agent={agent}
          otherAgents={agents.length > 1}
          sending={sending}
          onClear={() => setDraft([])}
          onSend={() => void sendComments({ machineId, paneId, agent }).catch((failure) => setActionError(errMsg(failure)))}
          onPick={() => setPickingAgent(true)}
          onVerdict={setOpenVerdict}
        />
      )}
      {pickingAgent && (
        <SendToAgent
          title={t("Send the review to")}
          text={commentsMessage(draft, where())}
          agents={agents}
          currentPane={currentPane}
          rememberKey={`review:${machineId}|${dir}`}
          onClose={() => setPickingAgent(false)}
          onSend={(target, text) => sendComments(target, text)}
        />
      )}
      {second.asking && (
        <OpinionDialog
          machineId={machineId}
          paneId={paneId}
          dir={dir}
          paneAgent={agent}
          onClose={() => second.setAsking(false)}
          onAsked={(asked, label) => {
            second.setAsking(false);
            second.mutate(() => asked.opinion);
            if (asked.agent_error) setActionError(t("{agent} did not start: {error}", { agent: label, error: asked.agent_error }));
            else showNotice(t("{agent} is reviewing in a new tab; its comments appear here when it answers.", { agent: label }));
          }}
        />
      )}
      {openVerdict && (
        <VerdictDialog
          verdict={openVerdict}
          paneAgent={agent}
          agents={agents}
          currentPane={currentPane}
          comments={draft.length}
          check={check}
          listed={listed}
          onJump={showFinding}
          onClose={() => setOpenVerdict(null)}
          onSend={(target, note, ask) => sendVerdict(openVerdict, target, note, ask)}
        />
      )}
    </div>
  );
}
