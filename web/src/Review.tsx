// The Changes lens: the workspace folder's uncommitted work — what git diff
// shows against HEAD, plus untracked files, with what moved since the
// reviewer last looked marked fresh — and the commits made since that look.
// Each file's diff on demand; a comment on any hunk joins a draft, and the
// draft goes to the pane's agent as a prompt with the files and lines — on
// its own, or under a verdict: Approve tells the agent the work is reviewed
// and it can carry on, Reject tells it what to fix, each with a note. Every
// verdict moves the baseline to now; "Mark reviewed" moves it and tells
// nobody.

import { useCallback, useEffect, useMemo, useState } from "react";
import { fetchReview, fetchReviewDiff, markReviewed, submitPrompt, type Review as ReviewData, type ReviewFile } from "./api";
import type { AgentTarget } from "./Tasks";
import SendToAgent, { targetKey } from "./SendToAgent";
import Select from "./Select";
import Logo from "./Logo";
import { basename, dirname, errMsg, usePref } from "./prefs";
import { ago } from "./time";
import { Dialog, Notice, useSubmit, useTimedNotice } from "./ui";
import Icon from "./icons";

// A review comment: a line of a file's diff and what should change there.
// Comments gather in a draft (kept per folder on this device) and go to the
// agent as one message.
export type DraftComment = {
  id: number;
  file: string;
  line: number; // in the new file; the old one for a removed line
  removed: boolean;
  code: string; // the diff row, prefix included
  text: string;
};

// what a review ends with, when it ends with a word to the agent
export type Verdict = "approved" | "changes";

const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? "" : "s"}`;

export default function Review({ machineId, paneId, dir, agent, agents, currentPane, onOpenFile, onSeen }: {
  machineId: string;
  paneId: string;
  dir: string;
  agent: string; // "" for a shell: nobody in this pane to send feedback to
  agents: AgentTarget[]; // any other agent the review can go to
  currentPane: { machineId: string; paneId: string } | null;
  onOpenFile: (path: string, line?: number) => void;
  onSeen?: () => void; // the roster's badges follow at once
}) {
  const [draft, setDraft] = usePref<DraftComment[]>(`review.draft:${machineId}|${dir}`, []);
  const [pick, setPick] = useState(false); // sending to another agent
  const [verdict, setVerdict] = useState<Verdict | null>(null); // the verdict dialog, open for one
  const addComment = (c: Omit<DraftComment, "id">) => setDraft((d) => [...d, { ...c, id: d.reduce((m, x) => Math.max(m, x.id), 0) + 1 }]);
  const removeComment = (id: number) => setDraft((d) => d.filter((c) => c.id !== id));
  const [data, setData] = useState<ReviewData | null>(null);
  const [error, setError] = useState("");
  const [open, setOpen] = useState<string | null>(null);
  const [showOld, setShowOld] = useState(false);
  const [tick, setTick] = useState(0);
  const [busy, setBusy] = useState(false); // marking reviewed
  const [sending, setSending] = useState(false);
  const [notice, showNotice] = useTimedNotice(5000);

  useEffect(() => {
    let gone = false;
    setError("");
    fetchReview(machineId, dir).then(
      (d) => {
        if (gone) return;
        setData(d);
        if (d.error) setError(d.error);
      },
      (e) => !gone && setError(errMsg(e)),
    );
    return () => {
      gone = true;
    };
  }, [machineId, dir, tick]);
  // one loader for every file row, stable across renders: a row's effect
  // keys on it and must not fire again on each render of the list
  const loadDiff = useCallback((path: string) => fetchReviewDiff(machineId, dir, path), [machineId, dir]);

  // the next look starts from now; nothing is said to anyone
  const seen = async () => {
    setBusy(true);
    try {
      await markReviewed(machineId, dir);
      setOpen(null);
      setTick((t) => t + 1);
      onSeen?.();
      showNotice("Reviewed — the next look starts from here.");
    } catch (e) {
      setError(errMsg(e));
    } finally {
      setBusy(false);
    }
  };

  // the comments as a numbered list, each with its place and its line
  const commentLines = () => draft.map((c, i) => `${i + 1}. ${c.file}:${c.line}${c.removed ? " (removed line)" : ""}\n   > ${c.code}\n   ${c.text.replace(/\n/g, "\n   ")}`);
  const where = () => `${dir}${data?.branch ? ` (${data.branch}${data.head ? ` at ${data.head.slice(0, 7)}` : ""})` : ""}`;
  // the draft as one message, no verdict
  const reviewMessage = () => [`Review comments (${draft.length}) on ${where()}:`, ...commentLines()].join("\n\n");
  // the draft under a verdict, with the reviewer's note and what the
  // verdict means at the foot: approved work is done with, and a task it
  // belongs to can move on; rejected work is the agent's next job
  const verdictMessage = (v: Verdict, note: string) => {
    const parts = [`Review of ${where()}: ${v === "approved" ? "approved" : "changes requested"}.`];
    if (note.trim()) parts.push(note.trim());
    if (draft.length > 0) parts.push(`Comments (${draft.length}):`, ...commentLines());
    parts.push(
      v === "approved"
        ? "The changes reviewed are approved: carry on with your flow, and if this is part of a task, it can move on. Any comments above are notes to take along, not blockers."
        : "Work on what is asked above, then say when the changes are ready for another look. The work is not finished until it is approved.",
    );
    return parts.join("\n\n");
  };
  // the draft goes to an agent as one prompt; a second tap while it is on
  // its way does nothing. A failure is thrown to the caller: the picker
  // dialog shows it in place, the direct button on the panel
  const sendDraft = async (target: { machineId: string; paneId: string; agent: string; sessionName?: string }, text = reviewMessage()) => {
    if (sending) return;
    setSending(true);
    try {
      await submitPrompt(target.machineId, target.paneId, text);
      setDraft([]);
      setPick(false);
      showNotice(`Sent ${plural(draft.length, "comment")} to ${target.agent}${target.sessionName ? ` in ${target.sessionName}` : ""}.`);
    } finally {
      setSending(false);
    }
  };
  // a verdict reaches the agent first, then the baseline moves: a refusal
  // (the agent busy with a question) leaves everything as it was
  const sendVerdict = async (v: Verdict, target: AgentTarget, note: string) => {
    await submitPrompt(target.machineId, target.paneId, verdictMessage(v, note));
    await markReviewed(machineId, dir, v);
    setDraft([]);
    setVerdict(null);
    setOpen(null);
    setTick((t) => t + 1);
    onSeen?.();
    showNotice(`${v === "approved" ? "Approved" : "Rejected"} — ${target.agent}${target.sessionName ? ` in ${target.sessionName}` : ""} has been told.`);
  };

  if (data === null && !error) return <div className="empty">Looking at the folder…</div>;
  if (data && !data.repo) return <div className="empty">{basename(dir)} is not a git checkout — nothing to review.</div>;
  const files = data?.files ?? [];
  const commits = data?.commits ?? [];
  const since = data?.baseline ? new Date(data.baseline.at) : null;
  const lastVerdict = data?.baseline?.verdict;
  const sinceText = !since ? "not reviewed yet" : lastVerdict === "approved" ? `approved ${ago(since)}` : lastVerdict === "changes" ? `rejected ${ago(since)}` : `since you looked ${ago(since)}`;
  const added = files.reduce((a, f) => a + f.added, 0);
  const removed = files.reduce((a, f) => a + f.removed, 0);
  const orphans = draft.filter((c) => !files.some((f) => f.path === c.file));
  const canTell = !!agent || agents.length > 0; // somebody to send a review to
  // what moved since the look comes first; the rest waits behind a fold
  const freshFiles = since ? files.filter((f) => f.fresh) : files;
  const oldFiles = since ? files.filter((f) => !f.fresh) : [];
  const fileRow = (f: ReviewFile) => (
    <FileRow
      key={`${f.path}|${tick}`} // a new look re-reads the diff
      file={f}
      open={open === f.path}
      onToggle={() => setOpen(open === f.path ? null : f.path)}
      load={loadDiff}
      canComment={canTell}
      draft={draft.filter((c) => c.file === f.path)}
      onComment={(c) => addComment({ ...c, file: f.path })}
      onRemove={removeComment}
      onOpen={(line) => onOpenFile(`${dir}/${f.path}`, line)}
    />
  );

  return (
    <div className="review">
      <div className="review-head">
        <div className="review-title">
          <span className="review-branch">
            <Icon name="git-branch" size={12} /> {data?.branch || "detached"}
          </span>
          <span className="review-sub">
            {sinceText}
            {commits.length > 0 && ` · ${plural(commits.length, "commit")}`}
            {files.length > 0 && ` · ${plural(files.length, "file")} uncommitted +${added} −${removed}`}
          </span>
        </div>
        <button type="button" className="icon-btn" title="Look again" onClick={() => setTick((t) => t + 1)}>
          <Icon name="refresh-cw" size={14} />
        </button>
        <button type="button" className="btn review-seen" disabled={busy || (!files.length && !commits.length)} onClick={() => void seen()} title="The counts clear and the next look starts from now; the agent hears nothing">
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
            {commits.map((c) => (
              <div key={c.sha} className={`review-commit${c.seen ? " seen" : ""}`} title={c.seen ? "A commit of work you already reviewed" : undefined}>
                <code>{c.sha}</code>
                <span className="review-commit-subject">{c.subject}</span>
                {c.seen && <span className="review-tag">reviewed</span>}
                <span className="review-commit-when">{ago(c.when * 1000)}</span>
              </div>
            ))}
          </div>
        )}
        {files.length === 0 && <div className="empty">{commits.length > 0 ? "Everything here is committed." : since ? "Nothing has changed here since you looked." : "Nothing is uncommitted here."}</div>}
        {freshFiles.map((f) => fileRow(f))}
        {oldFiles.length > 0 && (
          <div className="review-fold">
            <button type="button" className="linkish" aria-expanded={showOld} onClick={() => setShowOld((v) => !v)}>
              <Icon name={showOld ? "chevron-down" : "chevron-right"} size={12} /> {plural(oldFiles.length, "file")} unchanged since you looked
            </button>
            {showOld && oldFiles.map((f) => fileRow(f))}
          </div>
        )}
        {data && orphans.length > 0 && (
          <div className="review-orphans">
            <Notice tone="hint">Comments on files no longer in this list — still in the draft:</Notice>
            {orphans.map((c) => (
              <Note key={c.id} comment={c} where={`${c.file}:${c.line}`} onRemove={removeComment} />
            ))}
          </div>
        )}
      </div>
      {data && canTell && (files.length > 0 || commits.length > 0 || draft.length > 0) && (
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
                  <button type="button" className="btn" disabled={sending} title={`Send the comments to ${agent}, with no verdict`} onClick={() => sendDraft({ machineId, paneId, agent }).catch((e) => setError(errMsg(e)))}>
                    <Icon name="send" size={12} /> Send
                  </button>
                  {agents.length > 1 && (
                    <button type="button" className="icon-btn" title="Send the comments to another agent" disabled={sending} onClick={() => setPick(true)}>
                      <Icon name="chevron-down" size={14} />
                    </button>
                  )}
                </>
              ) : (
                <button type="button" className="btn" disabled={sending} title="Send the comments to an agent, with no verdict" onClick={() => setPick(true)}>
                  <Icon name="send" size={12} /> Send…
                </button>
              )}
            </>
          )}
          <span className="review-verdicts">
            <button type="button" className="btn" onClick={() => setVerdict("changes")} title="Tell the agent what to change: the comments and a note go to it, and the next look starts from now">
              <Icon name="x" size={12} /> Reject
            </button>
            <button type="button" className="btn primary" onClick={() => setVerdict("approved")} title="Tell the agent the changes are reviewed and it can carry on; the next look starts from now">
              <Icon name="check" size={12} /> Approve
            </button>
          </span>
        </div>
      )}
      {pick && (
        <SendToAgent
          title="Send the review to"
          text={reviewMessage()}
          agents={agents}
          currentPane={currentPane}
          rememberKey={`review:${machineId}|${dir}`}
          onClose={() => setPick(false)}
          onSend={(target, text) => sendDraft(target, text)}
        />
      )}
      {verdict && (
        <VerdictDialog
          verdict={verdict}
          paneAgent={agent}
          agents={agents}
          currentPane={currentPane}
          comments={draft.length}
          onClose={() => setVerdict(null)}
          onSend={(target, note) => sendVerdict(verdict, target, note)}
        />
      )}
    </div>
  );
}

// The verdict dialog: a note for the agent, the agent it goes to (this
// pane's when it has one, else a choice among the running ones), and what
// the agent will be told.
function VerdictDialog({ verdict, paneAgent, agents, currentPane, comments, onClose, onSend }: {
  verdict: Verdict;
  paneAgent: string; // "" when this pane has none
  agents: AgentTarget[];
  currentPane: { machineId: string; paneId: string } | null;
  comments: number; // in the draft, going along
  onClose: () => void;
  onSend: (target: AgentTarget, note: string) => Promise<void>;
}) {
  const approving = verdict === "approved";
  const [note, setNote] = useState("");
  // this pane's agent is the one, when it has one (a pane whose agent the
  // roster has not listed yet is still a target); a shell pane's verdict
  // goes to an agent chosen by hand, never to whichever came first
  const own = currentPane && paneAgent ? agents.find((a) => targetKey(a) === targetKey(currentPane)) ?? { ...currentPane, agent: paneAgent, machineLabel: "", sessionName: "", paneTitle: "", status: "", cwd: "" } : null;
  const choices = own && !agents.some((a) => targetKey(a) === targetKey(own)) ? [own, ...agents] : agents;
  const [target, setTarget] = useState(own ? targetKey(own) : "");
  const chosen = choices.find((a) => targetKey(a) === target);
  // a rejection with nothing to fix says nothing: a comment or a note
  const empty = !approving && comments === 0 && !note.trim();
  const { busy, err, run } = useSubmit(async () => {
    if (!chosen || empty) return;
    await onSend(chosen, note);
  });
  return (
    <Dialog
      title={approving ? "Approve the changes" : "Reject the changes"}
      icon={approving ? "check" : "x"}
      size="md"
      as="form"
      onClose={onClose}
      onSubmit={(e) => {
        e.preventDefault();
        void run();
      }}
    >
      {choices.length === 0 ? (
        <Notice tone="hint">No agent is running to tell.</Notice>
      ) : choices.length === 1 && chosen ? (
        <p className="modal-hint">
          To {chosen.agent}
          {chosen.sessionName ? ` in ${chosen.sessionName}` : " in this pane"}.
        </p>
      ) : (
        <label>
          Agent
          <Select
            value={target}
            onChange={setTarget}
            placeholder="Choose the agent"
            options={choices.map((a) => ({
              value: targetKey(a),
              label: a.sessionName ? `${a.sessionName} › ${a.paneTitle}` : "this pane",
              hint: a.status ? `${a.agent} · ${a.status}` : a.agent,
              group: a.machineLabel || undefined,
              icon: <Logo agent={a.agent} />,
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
          onChange={(e) => setNote(e.target.value)}
        />
      </label>
      <p className="modal-hint">
        {comments > 0 ? `${plural(comments, "comment")} from the draft ${comments === 1 ? "goes" : "go"} along. ` : ""}
        {approving ? "The agent is told the changes are reviewed and it can carry on; the next look starts from now." : "The agent is told to work on this and say when it is ready for another look; the next look starts from now."}
      </p>
      {err && <Notice tone="error">{err}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          Cancel
        </button>
        <button type="submit" className={`btn ${approving ? "primary" : "danger"}`} disabled={busy || !chosen || empty}>
          {busy ? "Sending…" : approving ? "Approve" : "Reject"}
        </button>
      </div>
    </Dialog>
  );
}

type NewComment = Omit<DraftComment, "id" | "file">;

function FileRow({ file: f, open, onToggle, load, canComment, draft, onComment, onRemove, onOpen }: {
  file: ReviewFile;
  open: boolean;
  onToggle: () => void;
  load: (path: string) => Promise<string>;
  canComment: boolean;
  draft: DraftComment[]; // this file's comments
  onComment: (c: NewComment) => void;
  onRemove: (id: number) => void;
  onOpen: (line?: number) => void;
}) {
  const [diff, setDiff] = useState<string | null>(null);
  const [err, setErr] = useState("");
  // the diff is read once, when the row first opens; a failure stays on
  // the row until "Try again", it is not retried on every render
  useEffect(() => {
    if (!open || diff !== null || err) return;
    let gone = false;
    load(f.path).then(
      (d) => !gone && setDiff(d),
      (e) => !gone && setErr(errMsg(e)),
    );
    return () => {
      gone = true;
    };
  }, [open, diff, err, load, f.path]);
  return (
    <div className={`review-file${f.fresh ? " fresh" : ""}${open ? " open" : ""}`}>
      <div className="review-file-row">
        <button type="button" className="review-file-head" onClick={onToggle} aria-expanded={open}>
          <span className={`chevron${open ? " open" : ""}`}>
            <Icon name="chevron-right" size={12} />
          </span>
          {f.fresh && <span className="review-dot" title="Changed since you looked" />}
          <span className="review-path">
            <span className="review-dir">{dirname(f.path)}</span>
            {basename(f.path)}
          </span>
          {draft.length > 0 && (
            <span className="review-tag review-tag-comments" title={`${plural(draft.length, "comment")} in the draft`}>
              {draft.length}
            </span>
          )}
          {f.untracked ? (
            <span className="review-tag">new</span>
          ) : (
            <span className="review-counts">
              +{f.added} −{f.removed}
            </span>
          )}
        </button>
        <button type="button" className="icon-btn sm" title="Open the file" onClick={() => onOpen()}>
          <Icon name="external-link" size={12} />
        </button>
      </div>
      {open &&
        (err ? (
          <Notice
            tone="error"
            action={
              <button type="button" className="linkish" onClick={() => setErr("")}>
                Try again
              </button>
            }
          >
            {err}
          </Notice>
        ) : diff === null ? (
          <div className="empty">Loading…</div>
        ) : (
          <Diff text={diff} canComment={canComment} draft={draft} onComment={onComment} onRemove={onRemove} />
        ))}
    </div>
  );
}

// A unified diff, hunk by hunk. A tap on a row opens a comment box under
// it; the comments written sit under their rows until the draft is sent.
function Diff({ text, canComment, draft, onComment, onRemove }: { text: string; canComment: boolean; draft: DraftComment[]; onComment: (c: NewComment) => void; onRemove: (id: number) => void }) {
  const hunks = useMemo(() => splitHunks(text), [text]);
  // a comment belongs to a row by its number and its text; the file may
  // have moved on since it was written
  const placed = useMemo(() => {
    const keys = new Set<string>();
    for (const h of hunks) for (const r of rowsOf(h)) keys.add(rowKey(r));
    return keys;
  }, [hunks]);
  const stray = draft.filter((c) => !placed.has(commentKey(c)));
  if (hunks.length === 0 && stray.length === 0) return <div className="empty">No textual diff (binary, or identical).</div>;
  return (
    <div className={`fv-code fv-diff review-diff${canComment ? " can-comment" : ""}`}>
      {hunks.map((h, i) => (
        <Hunk key={i} hunk={h} canComment={canComment} draft={draft} onComment={onComment} onRemove={onRemove} />
      ))}
      {stray.length > 0 && (
        <div className="review-orphans">
          <Notice tone="hint">No longer on a line of this diff — still in the draft:</Notice>
          {stray.map((c) => (
            <Note key={c.id} comment={c} where={`line ${c.line}${c.removed ? " (removed)" : ""}`} onRemove={onRemove} />
          ))}
        </div>
      )}
    </div>
  );
}

// a comment card under a row, or on its own when its row is gone
function Note({ comment: c, where, onRemove }: { comment: DraftComment; where?: string; onRemove: (id: number) => void }) {
  return (
    <div className="review-note">
      <span className="review-note-text">
        {where && (
          <span className="review-note-where">
            {where} <code>{c.code}</code>
          </span>
        )}
        {c.text}
      </span>
      <button type="button" className="icon-btn sm" title="Remove this comment" onClick={() => onRemove(c.id)}>
        <Icon name="x" size={12} />
      </button>
    </div>
  );
}

const rowKey = (r: Row) => `${r.kind === "del" ? "-" : "+"}${r.kind === "del" ? r.o : r.n}\u0000${r.l}`;
const commentKey = (c: DraftComment) => `${c.removed ? "-" : "+"}${c.line}\u0000${c.code}`;

type HunkT = { header: string; oldStart: number; start: number; lines: string[] };

function splitHunks(text: string): HunkT[] {
  const out: HunkT[] = [];
  let cur: HunkT | null = null;
  const lines = text.split("\n");
  if (lines[lines.length - 1] === "") lines.pop(); // the file's final newline, not a line
  for (const l of lines) {
    if (l.startsWith("@@")) {
      const m = /-(\d+)(?:,\d+)? \+(\d+)/.exec(l);
      cur = { header: l, oldStart: m ? Number(m[1]) : 1, start: m ? Number(m[2]) : 1, lines: [] };
      out.push(cur);
    } else if (cur) {
      cur.lines.push(l); // the file headers come before the first @@; a row like "---x" or "+++i" is a row
    }
  }
  return out;
}

type Row = { l: string; kind: string; n: number | null; o: number | null };

// rowsOf numbers a hunk's rows in the new file and the old one, for each
// row's address
function rowsOf(hunk: HunkT): Row[] {
  let n = hunk.start;
  let o = hunk.oldStart;
  return hunk.lines.map((l) => {
    const kind = l.startsWith("+") ? "add" : l.startsWith("-") ? "del" : l.startsWith("\\") ? "meta" : "";
    const row: Row = { l, kind, n: kind === "del" || kind === "meta" ? null : n, o: kind === "add" || kind === "meta" ? null : o };
    if (kind === "add") n++;
    else if (kind === "del") o++;
    else if (kind !== "meta") {
      n++;
      o++;
    }
    return row;
  });
}

function Hunk({ hunk, canComment, draft, onComment, onRemove }: { hunk: HunkT; canComment: boolean; draft: DraftComment[]; onComment: (c: NewComment) => void; onRemove: (id: number) => void }) {
  const [at, setAt] = useState<number | null>(null); // the row a comment is being written under
  const [text, setText] = useState("");
  const rows = useMemo(() => rowsOf(hunk), [hunk]);
  const commentsAt = (r: Row) => {
    if (r.kind === "meta") return [];
    const k = rowKey(r);
    return draft.filter((c) => commentKey(c) === k);
  };
  // a tap on a row opens the box under it; a drag that selected text is
  // a copy, not a tap
  const toggle = (i: number) => {
    if (window.getSelection()?.toString()) return;
    setAt(at === i ? null : i);
    setText("");
  };
  const submit = (r: Row) => {
    const t = text.trim();
    if (!t) return;
    onComment({ line: (r.kind === "del" ? r.o : r.n) ?? hunk.start, removed: r.kind === "del", code: r.l, text: t });
    setAt(null);
    setText("");
  };
  return (
    <div className="review-hunk">
      <div className="fv-line hunk">
        <span className="fv-src">{hunk.header}</span>
      </div>
      {rows.map((r, i) => {
        const commentable = canComment && r.kind !== "meta";
        return (
          <div key={i}>
            <div
              className={`fv-line ${r.kind}${commentable ? " commentable" : ""}${at === i ? " composing" : ""}`}
              role={commentable ? "button" : undefined}
              tabIndex={commentable ? 0 : undefined}
              onClick={commentable ? () => toggle(i) : undefined}
              onKeyDown={
                commentable
                  ? (e) => {
                      if (e.target !== e.currentTarget) return;
                      if (e.key === "Enter" || e.key === " ") {
                        e.preventDefault();
                        toggle(i);
                      }
                    }
                  : undefined
              }
              title={commentable ? "Comment on this line" : undefined}
            >
              <span className="fv-ln">{r.n ?? r.o ?? ""}</span>
              <span className="fv-src">{r.l}</span>
            </div>
            {commentsAt(r).map((c) => (
              <Note key={c.id} comment={c} onRemove={onRemove} />
            ))}
            {at === i && (
              <form
                className="review-comment"
                onSubmit={(e) => {
                  e.preventDefault();
                  submit(r);
                }}
              >
                <textarea
                  autoFocus
                  rows={3}
                  value={text}
                  placeholder={`What should change at line ${r.n ?? r.o}? It joins the draft; everything goes to the agent in one message.`}
                  onChange={(e) => setText(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Escape") setAt(null);
                    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) submit(r);
                  }}
                />
                <div className="modal-actions">
                  <button type="button" className="btn" onClick={() => setAt(null)}>
                    Cancel
                  </button>
                  <button type="submit" className="btn primary" disabled={!text.trim()}>
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
