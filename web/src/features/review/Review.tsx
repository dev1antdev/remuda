// The Changes lens: a workspace folder's uncommitted work and the commits
// since the last look, with comments on diff lines gathered into a draft
// that goes to the pane's agent, on its own or under a verdict.

import { useCallback, useEffect, useMemo, useState } from "react";
import { fetchReview, fetchReviewDiff, markReviewed, type Review as ReviewData, type ReviewFile, type Verdict } from "@/features/review/api";
import { submitPrompt } from "@/features/roster/api";
import type { AgentTarget } from "@/features/agents/targets";
import SendToAgent from "@/features/agents/SendToAgent";
import { targetKey } from "@/features/agents/targets";
import Select from "@/ui/Select";
import Logo from "@/ui/Logo";
import { basename, dirname, errMsg } from "@/lib/text";
import { usePref } from "@/lib/prefs";
import { ago } from "@/lib/time";
import { Dialog, Notice, useSubmit, useTimedNotice } from "@/ui";
import Icon from "@/ui/icons";
import { plural } from "@/lib/text";

export type DraftComment = {
  id: number;
  file: string;
  line: number; // in the new file; in the old one for a removed line
  removed: boolean;
  code: string; // the diff row with its prefix, part of the comment's address
  text: string;
};


export default function Review({ machineId, paneId, dir, agent, agents, currentPane, onOpenFile, onSeen }: {
  machineId: string;
  paneId: string;
  dir: string;
  agent: string; // "" for a shell pane
  agents: AgentTarget[];
  currentPane: { machineId: string; paneId: string } | null;
  onOpenFile: (path: string, line?: number) => void;
  onSeen?: () => void;
}) {
  const [draft, setDraft] = usePref<DraftComment[]>(`review.draft:${machineId}|${dir}`, []);
  const [pickingAgent, setPickingAgent] = useState(false);
  const [openVerdict, setOpenVerdict] = useState<Verdict | null>(null);
  const [review, setReview] = useState<ReviewData | null>(null);
  const [error, setError] = useState("");
  const [openFile, setOpenFile] = useState<string | null>(null);
  const [showUnchanged, setShowUnchanged] = useState(false);
  const [lookCount, setLookCount] = useState(0);
  const [marking, setMarking] = useState(false);
  const [sending, setSending] = useState(false);
  const [notice, showNotice] = useTimedNotice(5000);

  const addComment = (comment: Omit<DraftComment, "id">) => setDraft((comments) => [...comments, { ...comment, id: comments.reduce((max, other) => Math.max(max, other.id), 0) + 1 }]);
  const removeComment = (id: number) => setDraft((comments) => comments.filter((comment) => comment.id !== id));
  const lookAgain = () => setLookCount((count) => count + 1);

  useEffect(() => {
    let cancelled = false;
    setError("");
    fetchReview(machineId, dir).then(
      (summary) => {
        if (cancelled) return;
        setReview(summary);
        if (summary.error) setError(summary.error);
      },
      (failure) => !cancelled && setError(errMsg(failure)),
    );
    return () => {
      cancelled = true;
    };
  }, [machineId, dir, lookCount]);
  const loadDiff = useCallback((path: string) => fetchReviewDiff(machineId, dir, path), [machineId, dir]); // stable: every row's effect keys on it

  const markReviewedSilently = async () => {
    setMarking(true);
    try {
      await markReviewed(machineId, dir);
      setOpenFile(null);
      lookAgain();
      onSeen?.();
      showNotice("Reviewed — the next look starts from here.");
    } catch (failure) {
      setError(errMsg(failure));
    } finally {
      setMarking(false);
    }
  };

  const place = () => `${dir}${review?.branch ? ` (${review.branch}${review.head ? ` at ${review.head.slice(0, 7)}` : ""})` : ""}`;
  const commentLines = () => draft.map((comment, index) => `${index + 1}. ${comment.file}:${comment.line}${comment.removed ? " (removed line)" : ""}\n   > ${comment.code}\n   ${comment.text.replace(/\n/g, "\n   ")}`);
  const commentsMessage = () => [`Review comments (${draft.length}) on ${place()}:`, ...commentLines()].join("\n\n");
  const verdictMessage = (verdict: Verdict, note: string) => {
    const parts = [`Review of ${place()}: ${verdict === "approved" ? "approved" : "changes requested"}.`];
    if (note.trim()) parts.push(note.trim());
    if (draft.length > 0) parts.push(`Comments (${draft.length}):`, ...commentLines());
    parts.push(
      verdict === "approved"
        ? "The changes reviewed are approved: carry on with your flow, and if this is part of a task, it can move on. Any comments above are notes to take along, not blockers."
        : "Work on what is asked above, then say when the changes are ready for another look. The work is not finished until it is approved.",
    );
    return parts.join("\n\n");
  };

  const sendComments = async (target: { machineId: string; paneId: string; agent: string; sessionName?: string }, text = commentsMessage()) => {
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
  const sendVerdict = async (verdict: Verdict, target: AgentTarget, note: string) => {
    await submitPrompt(target.machineId, target.paneId, verdictMessage(verdict, note));
    await markReviewed(machineId, dir, verdict);
    setDraft([]);
    setOpenVerdict(null);
    setOpenFile(null);
    lookAgain();
    onSeen?.();
    showNotice(`${verdict === "approved" ? "Approved" : "Rejected"} — ${target.agent}${target.sessionName ? ` in ${target.sessionName}` : ""} has been told.`);
  };

  if (review === null && !error) return <div className="empty">Looking at the folder…</div>;
  if (review && !review.repo) return <div className="empty">{basename(dir)} is not a git checkout — nothing to review.</div>;
  const files = review?.files ?? [];
  const commits = review?.commits ?? [];
  const lastLook = review?.baseline ? new Date(review.baseline.at) : null;
  const lastVerdict = review?.baseline?.verdict;
  const lastLookText = !lastLook ? "not reviewed yet" : lastVerdict === "approved" ? `approved ${ago(lastLook)}` : lastVerdict === "changes" ? `rejected ${ago(lastLook)}` : `since you looked ${ago(lastLook)}`;
  const added = files.reduce((sum, file) => sum + file.added, 0);
  const removed = files.reduce((sum, file) => sum + file.removed, 0);
  const orphanComments = draft.filter((comment) => !files.some((file) => file.path === comment.file));
  const hasAgentToTell = !!agent || agents.length > 0;
  const freshFiles = lastLook ? files.filter((file) => file.fresh) : files;
  const unchangedFiles = lastLook ? files.filter((file) => !file.fresh) : [];
  const fileRow = (file: ReviewFile) => (
    <FileRow
      key={`${file.path}|${lookCount}`} // a new look remounts the row, which re-reads its diff
      file={file}
      open={openFile === file.path}
      onToggle={() => setOpenFile(openFile === file.path ? null : file.path)}
      load={loadDiff}
      canComment={hasAgentToTell}
      draft={draft.filter((comment) => comment.file === file.path)}
      onComment={(comment) => addComment({ ...comment, file: file.path })}
      onRemove={removeComment}
      onOpen={(line) => onOpenFile(`${dir}/${file.path}`, line)}
    />
  );

  return (
    <div className="review">
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
        <button type="button" className="icon-btn" title="Look again" onClick={lookAgain}>
          <Icon name="refresh-cw" size={14} />
        </button>
        <button type="button" className="btn review-seen" disabled={marking || (!files.length && !commits.length)} onClick={() => void markReviewedSilently()} title="The counts clear and the next look starts from now; the agent hears nothing">
          <Icon name="check" size={14} /> Mark reviewed
        </button>
      </div>
      {error && (
        <Notice tone="error" onDismiss={() => setError("")}>
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
                  <button type="button" className="btn" disabled={sending} title={`Send the comments to ${agent}, with no verdict`} onClick={() => sendComments({ machineId, paneId, agent }).catch((failure) => setError(errMsg(failure)))}>
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
          text={commentsMessage()}
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
          onSend={(target, note) => sendVerdict(openVerdict, target, note)}
        />
      )}
    </div>
  );
}

function VerdictDialog({ verdict, paneAgent, agents, currentPane, comments, onClose, onSend }: {
  verdict: Verdict;
  paneAgent: string; // "" when this pane has none
  agents: AgentTarget[];
  currentPane: { machineId: string; paneId: string } | null;
  comments: number;
  onClose: () => void;
  onSend: (target: AgentTarget, note: string) => Promise<void>;
}) {
  const approving = verdict === "approved";
  const [note, setNote] = useState("");
  const paneTarget = currentPane && paneAgent ? agents.find((candidate) => targetKey(candidate) === targetKey(currentPane)) ?? { ...currentPane, agent: paneAgent, machineLabel: "", sessionName: "", paneTitle: "", status: "", cwd: "" } : null;
  const targets = paneTarget && !agents.some((candidate) => targetKey(candidate) === targetKey(paneTarget)) ? [paneTarget, ...agents] : agents;
  const [chosenKey, setChosenKey] = useState(paneTarget ? targetKey(paneTarget) : "");
  const chosen = targets.find((candidate) => targetKey(candidate) === chosenKey);
  const nothingToFix = !approving && comments === 0 && !note.trim();
  const { busy, err: failure, run: submit } = useSubmit(async () => {
    if (!chosen || nothingToFix) return;
    await onSend(chosen, note);
  });
  return (
    <Dialog
      title={approving ? "Approve the changes" : "Reject the changes"}
      icon={approving ? "check" : "x"}
      size="md"
      as="form"
      onClose={onClose}
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      {targets.length === 0 ? (
        <Notice tone="hint">No agent is running to tell.</Notice>
      ) : targets.length === 1 && chosen ? (
        <p className="modal-hint">
          To {chosen.agent}
          {chosen.sessionName ? ` in ${chosen.sessionName}` : " in this pane"}.
        </p>
      ) : (
        <label>
          Agent
          <Select
            value={chosenKey}
            onChange={setChosenKey}
            placeholder="Choose the agent"
            options={targets.map((candidate) => ({
              value: targetKey(candidate),
              label: candidate.sessionName ? `${candidate.sessionName} › ${candidate.paneTitle}` : "this pane",
              hint: candidate.status ? `${candidate.agent} · ${candidate.status}` : candidate.agent,
              group: candidate.machineLabel || undefined,
              icon: <Logo agent={candidate.agent} />,
            }))}
          />
        </label>
      )}
      <label>
        {approving ? "A note for the agent (optional)" : "What should change"}
        <textarea
          autoFocus
          rows={5}
          value={note}
          placeholder={approving ? "Anything to take along; it goes with the approval." : comments > 0 ? "Goes with the comments." : "No comments in the draft: say it here."}
          onChange={(event) => setNote(event.target.value)}
        />
      </label>
      <p className="modal-hint">
        {comments > 0 ? `${plural(comments, "comment")} from the draft ${comments === 1 ? "goes" : "go"} along. ` : ""}
        {approving ? "The agent is told the changes are reviewed and it can carry on; the next look starts from now." : "The agent is told to work on this and say when it is ready for another look; the next look starts from now."}
      </p>
      {failure && <Notice tone="error">{failure}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          Cancel
        </button>
        <button type="submit" className={`btn ${approving ? "primary" : "danger"}`} disabled={busy || !chosen || nothingToFix}>
          {busy ? "Sending…" : approving ? "Approve" : "Reject"}
        </button>
      </div>
    </Dialog>
  );
}

type NewComment = Omit<DraftComment, "id" | "file">;

function FileRow({ file, open, onToggle, load, canComment, draft, onComment, onRemove, onOpen }: {
  file: ReviewFile;
  open: boolean;
  onToggle: () => void;
  load: (path: string) => Promise<string>;
  canComment: boolean;
  draft: DraftComment[];
  onComment: (comment: NewComment) => void;
  onRemove: (id: number) => void;
  onOpen: (line?: number) => void;
}) {
  const [diff, setDiff] = useState<string | null>(null);
  const [failure, setFailure] = useState("");
  useEffect(() => {
    if (!open || diff !== null || failure) return;
    let cancelled = false;
    load(file.path).then(
      (text) => !cancelled && setDiff(text),
      (reason) => !cancelled && setFailure(errMsg(reason)),
    );
    return () => {
      cancelled = true;
    };
  }, [open, diff, failure, load, file.path]);
  return (
    <div className={`review-file${file.fresh ? " fresh" : ""}${open ? " open" : ""}`}>
      <div className="review-file-row">
        <button type="button" className="review-file-head" onClick={onToggle} aria-expanded={open}>
          <span className={`chevron${open ? " open" : ""}`}>
            <Icon name="chevron-right" size={12} />
          </span>
          {file.fresh && <span className="review-dot" title="Changed since you looked" />}
          <span className="review-path">
            <span className="review-dir">{dirname(file.path)}</span>
            {basename(file.path)}
          </span>
          {draft.length > 0 && (
            <span className="review-tag review-tag-comments" title={`${plural(draft.length, "comment")} in the draft`}>
              {draft.length}
            </span>
          )}
          {file.untracked ? (
            <span className="review-tag">new</span>
          ) : (
            <span className="review-counts">
              +{file.added} −{file.removed}
            </span>
          )}
        </button>
        <button type="button" className="icon-btn sm" title="Open the file" onClick={() => onOpen()}>
          <Icon name="external-link" size={12} />
        </button>
      </div>
      {open &&
        (failure ? (
          <Notice
            tone="error"
            action={
              <button type="button" className="linkish" onClick={() => setFailure("")}>
                Try again
              </button>
            }
          >
            {failure}
          </Notice>
        ) : diff === null ? (
          <div className="empty">Loading…</div>
        ) : (
          <Diff text={diff} canComment={canComment} draft={draft} onComment={onComment} onRemove={onRemove} />
        ))}
    </div>
  );
}

function Diff({ text, canComment, draft, onComment, onRemove }: { text: string; canComment: boolean; draft: DraftComment[]; onComment: (comment: NewComment) => void; onRemove: (id: number) => void }) {
  const hunks = useMemo(() => splitHunks(text), [text]);
  const rowKeys = useMemo(() => {
    const keys = new Set<string>();
    for (const hunk of hunks) for (const row of rowsOf(hunk)) keys.add(rowKey(row));
    return keys;
  }, [hunks]);
  const strayComments = draft.filter((comment) => !rowKeys.has(commentKey(comment)));
  if (hunks.length === 0 && strayComments.length === 0) return <div className="empty">No textual diff (binary, or identical).</div>;
  return (
    <div className={`fv-code fv-diff review-diff${canComment ? " can-comment" : ""}`}>
      {hunks.map((hunk, index) => (
        <Hunk key={index} hunk={hunk} canComment={canComment} draft={draft} onComment={onComment} onRemove={onRemove} />
      ))}
      {strayComments.length > 0 && (
        <div className="review-orphans">
          <Notice tone="hint">No longer on a line of this diff — still in the draft:</Notice>
          {strayComments.map((comment) => (
            <Note key={comment.id} comment={comment} where={`line ${comment.line}${comment.removed ? " (removed)" : ""}`} onRemove={onRemove} />
          ))}
        </div>
      )}
    </div>
  );
}

function Note({ comment, where, onRemove }: { comment: DraftComment; where?: string; onRemove: (id: number) => void }) {
  return (
    <div className="review-note">
      <span className="review-note-text">
        {where && (
          <span className="review-note-where">
            {where} <code>{comment.code}</code>
          </span>
        )}
        {comment.text}
      </span>
      <button type="button" className="icon-btn sm" title="Remove this comment" onClick={() => onRemove(comment.id)}>
        <Icon name="x" size={12} />
      </button>
    </div>
  );
}

const rowKey = (row: DiffRow) => `${row.kind === "del" ? "-" : "+"}${row.kind === "del" ? row.oldLine : row.newLine}\u0000${row.text}`;
const commentKey = (comment: DraftComment) => `${comment.removed ? "-" : "+"}${comment.line}\u0000${comment.code}`;

type DiffHunk = { header: string; oldStart: number; newStart: number; lines: string[] };

function splitHunks(text: string): DiffHunk[] {
  const hunks: DiffHunk[] = [];
  let current: DiffHunk | null = null;
  const lines = text.split("\n");
  if (lines[lines.length - 1] === "") lines.pop(); // the trailing newline
  for (const line of lines) {
    if (line.startsWith("@@")) {
      const range = /-(\d+)(?:,\d+)? \+(\d+)/.exec(line);
      current = { header: line, oldStart: range ? Number(range[1]) : 1, newStart: range ? Number(range[2]) : 1, lines: [] };
      hunks.push(current);
    } else if (current) {
      current.lines.push(line); // the file headers sit before the first @@; "---x" or "+++i" inside a hunk is a row
    }
  }
  return hunks;
}

type DiffRow = { text: string; kind: string; newLine: number | null; oldLine: number | null };

function rowsOf(hunk: DiffHunk): DiffRow[] {
  let newLine = hunk.newStart;
  let oldLine = hunk.oldStart;
  return hunk.lines.map((text) => {
    const kind = text.startsWith("+") ? "add" : text.startsWith("-") ? "del" : text.startsWith("\\") ? "meta" : "";
    const row: DiffRow = { text, kind, newLine: kind === "del" || kind === "meta" ? null : newLine, oldLine: kind === "add" || kind === "meta" ? null : oldLine };
    if (kind === "add") newLine++;
    else if (kind === "del") oldLine++;
    else if (kind !== "meta") {
      newLine++;
      oldLine++;
    }
    return row;
  });
}

function Hunk({ hunk, canComment, draft, onComment, onRemove }: { hunk: DiffHunk; canComment: boolean; draft: DraftComment[]; onComment: (comment: NewComment) => void; onRemove: (id: number) => void }) {
  const [composingAt, setComposingAt] = useState<number | null>(null);
  const [draftText, setDraftText] = useState("");
  const rows = useMemo(() => rowsOf(hunk), [hunk]);
  const commentsOn = (row: DiffRow) => (row.kind === "meta" ? [] : draft.filter((comment) => commentKey(comment) === rowKey(row)));
  const toggleComposer = (index: number) => {
    if (window.getSelection()?.toString()) return; // a drag that selected text is a copy, not a tap
    setComposingAt(composingAt === index ? null : index);
    setDraftText("");
  };
  const addComment = (row: DiffRow) => {
    const text = draftText.trim();
    if (!text) return;
    onComment({ line: (row.kind === "del" ? row.oldLine : row.newLine) ?? hunk.newStart, removed: row.kind === "del", code: row.text, text });
    setComposingAt(null);
    setDraftText("");
  };
  return (
    <div className="review-hunk">
      <div className="fv-line hunk">
        <span className="fv-src">{hunk.header}</span>
      </div>
      {rows.map((row, index) => {
        const commentable = canComment && row.kind !== "meta";
        return (
          <div key={index}>
            <div
              className={`fv-line ${row.kind}${commentable ? " commentable" : ""}${composingAt === index ? " composing" : ""}`}
              role={commentable ? "button" : undefined}
              tabIndex={commentable ? 0 : undefined}
              onClick={commentable ? () => toggleComposer(index) : undefined}
              onKeyDown={
                commentable
                  ? (event) => {
                      if (event.target !== event.currentTarget) return;
                      if (event.key === "Enter" || event.key === " ") {
                        event.preventDefault();
                        toggleComposer(index);
                      }
                    }
                  : undefined
              }
              title={commentable ? "Comment on this line" : undefined}
            >
              <span className="fv-ln">{row.newLine ?? row.oldLine ?? ""}</span>
              <span className="fv-src">{row.text}</span>
            </div>
            {commentsOn(row).map((comment) => (
              <Note key={comment.id} comment={comment} onRemove={onRemove} />
            ))}
            {composingAt === index && (
              <form
                className="review-comment"
                onSubmit={(event) => {
                  event.preventDefault();
                  addComment(row);
                }}
              >
                <textarea
                  autoFocus
                  rows={3}
                  value={draftText}
                  placeholder={`What should change at line ${row.newLine ?? row.oldLine}? It joins the draft; everything goes to the agent in one message.`}
                  onChange={(event) => setDraftText(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key === "Escape") setComposingAt(null);
                    if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) addComment(row);
                  }}
                />
                <div className="modal-actions">
                  <button type="button" className="btn" onClick={() => setComposingAt(null)}>
                    Cancel
                  </button>
                  <button type="submit" className="btn primary" disabled={!draftText.trim()}>
                    <Icon name="plus" size={12} /> Add to the review
                  </button>
                </div>
              </form>
            )}
          </div>
        );
      })}
    </div>
  );
}
