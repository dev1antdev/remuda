// The Changes lens: a workspace folder's uncommitted work and the commits since the last look, the diffs in one
// column or two with the changed words marked, walked by keys, with comments on diff lines gathered into a draft
// that goes to the pane's agent, on its own or under a verdict.

import { useCallback, useMemo, useRef, useState } from "react";
import { actions } from "@/app/store";
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
import { Note } from "./Hunk";
import { KEYS, useReviewKeys } from "./keys";
import { commentsMessage, place, verdictMessage, type DraftComment, type VerdictAsk } from "./draft";
import { VerdictDialog } from "./VerdictDialog";

export type { DraftComment } from "./draft";

const NO_COMMENTS: DraftComment[] = []; // one empty list for every file without comments: a new array would redraw its hunks

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

  const markReviewedSilently = async () => {
    setMarking(true);
    try {
      await markReviewed(machineId, dir);
      setOpenFile(null);
      lookAgain();
      onSeen?.();
      showNotice("Reviewed — the next look starts from here.");
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
      showNotice(`Sent ${plural(draft.length, "comment")} to ${target.agent}${target.sessionName ? ` in ${target.sessionName}` : ""}.`);
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
    setOpenFile(null);
    lookAgain();
    onSeen?.();
    showNotice(`${verdict === "approved" ? "Approved" : "Rejected"} — ${target.agent}${target.sessionName ? ` in ${target.sessionName}` : ""} has been told${ask === "pr" ? " and asked to open a pull request" : ask === "commit" ? " and asked to commit" : ""}.`);
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

  if (review === null && !error) return <div className="empty">Looking at the folder…</div>;
  if (review && !review.repo) return <div className="empty">{basename(dir)} is not a git checkout — nothing to review.</div>;
  const lastVerdict = review?.baseline?.verdict;
  const lastLookText = !lastLook ? "not reviewed yet" : lastVerdict === "approved" ? `approved ${ago(lastLook)}` : lastVerdict === "changes" ? `rejected ${ago(lastLook)}` : `since you looked ${ago(lastLook)}`;
  const added = files.reduce((sum, file) => sum + file.added, 0);
  const removed = files.reduce((sum, file) => sum + file.removed, 0);
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
      onOpen={(line) => onOpenFile(`${dir}/${file.path}`, line)}
      onCode={() => actions.openInCode(machineId, dir, `${dir.replace(/\/+$/, "")}/${file.path}`)}
    />
  );

  return (
    <div className="review" ref={rootRef}>
      <div className="review-head">
        <div className="review-title">
          <span className="review-branch">
            <Icon name="git-branch" size={12} /> {review?.branch || "detached"}
          </span>
          <span className="review-sub">
            {lastLookText}
            {commits.length > 0 && ` · ${plural(commits.length, "commit")}`}
            {files.length > 0 && ` · ${plural(files.length, "file")} uncommitted +${added} −${removed}`}
          </span>
        </div>
        <span className="review-keys" aria-hidden="true">
          {KEYS.map(([first, second, what]) => (
            <span key={what}>
              <kbd>{first}</kbd>
              {second && <kbd>{second}</kbd>} {what}
            </span>
          ))}
        </span>
        <button type="button" className={`icon-btn${split ? " on" : ""}`} aria-pressed={split} title={split ? "Two columns, the old file on the left (s)" : "One column (s)"} onClick={() => setSplit((on) => !on)}>
          <Icon name="columns" size={14} />
        </button>
        <button type="button" className="icon-btn" title="Look again" onClick={lookAgain}>
          <Icon name="refresh-cw" size={14} />
        </button>
        <button
          type="button"
          className="btn review-seen"
          disabled={marking || (!files.length && !commits.length)}
          onClick={() => void markReviewedSilently()}
          title="The counts clear and the next look starts from now; the agent hears nothing"
        >
          <Icon name="check" size={14} /> Mark reviewed
        </button>
      </div>
      {error && (
        <Notice tone="error" onDismiss={() => setActionError("")}>
          {error}
        </Notice>
      )}
      {notice && <Notice tone="success">{notice}</Notice>}
      <div className="review-body">
        {commits.length > 0 && (
          <div className="review-commits">
            {commits.map((commit) => (
              <div key={commit.sha} className={`review-commit${commit.seen ? " seen" : ""}`} title={commit.seen ? "A commit of work you already reviewed" : undefined}>
                <code>{commit.sha}</code>
                <span className="review-commit-subject">{commit.subject}</span>
                {commit.seen && <span className="review-tag">reviewed</span>}
                <span className="review-commit-when">{ago(commit.when * 1000)}</span>
              </div>
            ))}
          </div>
        )}
        {files.length === 0 && <div className="empty">{commits.length > 0 ? "Everything here is committed." : lastLook ? "Nothing has changed here since you looked." : "Nothing is uncommitted here."}</div>}
        {freshFiles.map((file) => fileRow(file))}
        {unchangedFiles.length > 0 && (
          <div className="review-fold">
            <button type="button" className="linkish" aria-expanded={showUnchanged} onClick={() => setShowUnchanged((shown) => !shown)}>
              <Icon name={showUnchanged ? "chevron-down" : "chevron-right"} size={12} /> {plural(unchangedFiles.length, "file")} unchanged since you looked
            </button>
            {showUnchanged && unchangedFiles.map((file) => fileRow(file))}
          </div>
        )}
        {review && orphanComments.length > 0 && (
          <div className="review-orphans">
            <Notice tone="hint">Comments on files no longer in this list — still in the draft:</Notice>
            {orphanComments.map((comment) => (
              <Note key={comment.id} comment={comment} where={`${comment.file}:${comment.line}`} onRemove={removeComment} />
            ))}
          </div>
        )}
      </div>
      {review && hasAgentToTell && (files.length > 0 || commits.length > 0 || draft.length > 0) && (
        <div className="review-draft-bar">
          <span className="review-draft-count">
            {draft.length > 0 && (
              <>
                <Icon name="message" size={14} /> {plural(draft.length, "comment")}
              </>
            )}
          </span>
          {draft.length > 0 && (
            <>
              <button type="button" className="linkish" onClick={() => setDraft([])}>
                Clear
              </button>
              {agent ? (
                <>
                  <button type="button" className="btn" disabled={sending} title={`Send the comments to ${agent}, with no verdict`} onClick={() => sendComments({ machineId, paneId, agent }).catch((failure) => setActionError(errMsg(failure)))}>
                    <Icon name="send" size={12} /> Send
                  </button>
                  {agents.length > 1 && (
                    <button type="button" className="icon-btn" title="Send the comments to another agent" disabled={sending} onClick={() => setPickingAgent(true)}>
                      <Icon name="chevron-down" size={14} />
                    </button>
                  )}
                </>
              ) : (
                <button type="button" className="btn" disabled={sending} title="Send the comments to an agent, with no verdict" onClick={() => setPickingAgent(true)}>
                  <Icon name="send" size={12} /> Send…
                </button>
              )}
            </>
          )}
          <span className="review-verdicts">
            <button type="button" className="btn" onClick={() => setOpenVerdict("changes")} title="Tell the agent what to change: the comments and a note go to it, and the next look starts from now">
              <Icon name="x" size={12} /> Reject
            </button>
            <button type="button" className="btn primary" onClick={() => setOpenVerdict("approved")} title="Tell the agent the changes are reviewed and it can carry on; the next look starts from now">
              <Icon name="check" size={12} /> Approve
            </button>
          </span>
        </div>
      )}
      {pickingAgent && (
        <SendToAgent
          title="Send the review to"
          text={commentsMessage(draft, where())}
          agents={agents}
          currentPane={currentPane}
          rememberKey={`review:${machineId}|${dir}`}
          onClose={() => setPickingAgent(false)}
          onSend={(target, text) => sendComments(target, text)}
        />
      )}
      {openVerdict && (
        <VerdictDialog
          verdict={openVerdict}
          paneAgent={agent}
          agents={agents}
          currentPane={currentPane}
          comments={draft.length}
          onClose={() => setOpenVerdict(null)}
          onSend={(target, note, ask) => sendVerdict(openVerdict, target, note, ask)}
        />
      )}
    </div>
  );
}
