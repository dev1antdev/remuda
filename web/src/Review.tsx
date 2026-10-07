// The Changes lens: what moved in the workspace's folder since the reviewer
// last looked — commits made since, files changed against that point (with
// the ones that moved since marked fresh), each file's diff on demand, a
// comment on any hunk that goes to the pane's agent as a prompt with the
// file and line, and "Mark reviewed" to move the baseline to now.

import { useEffect, useMemo, useState } from "react";
import { fetchReview, fetchReviewDiff, markReviewed, submitPrompt, type Review as ReviewData, type ReviewFile } from "./api";
import type { AgentTarget } from "./Tasks";
import SendToAgent from "./SendToAgent";
import { usePref } from "./prefs";
import { errMsg } from "./prefs";
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
  const addComment = (c: Omit<DraftComment, "id">) => setDraft((d) => [...d, { ...c, id: d.reduce((m, x) => Math.max(m, x.id), 0) + 1 }]);
  const removeComment = (id: number) => setDraft((d) => d.filter((c) => c.id !== id));
  const [data, setData] = useState<ReviewData | null>(null);
  const [error, setError] = useState("");
  const [open, setOpen] = useState<string | null>(null);
  const [tick, setTick] = useState(0);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");

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

  const seen = async () => {
    setBusy(true);
    try {
      await markReviewed(machineId, dir);
      setOpen(null);
      setTick((t) => t + 1);
      onSeen?.();
      setNotice("Reviewed — the next look starts from here.");
      setTimeout(() => setNotice(""), 4000);
    } catch (e) {
      setError(errMsg(e));
    } finally {
      setBusy(false);
    }
  };

  // the draft as one message: numbered, each with its place and its line
  const reviewMessage = () =>
    [
      `Review comments (${draft.length}) on ${dir}${data?.branch ? ` (${data.branch})` : ""}:`,
      ...draft.map((c, i) => `${i + 1}. ${c.file}:${c.line}${c.removed ? " (removed line)" : ""}\n   > ${c.code}\n   ${c.text.replace(/\n/g, "\n   ")}`),
    ].join("\n\n");
  const sendDraft = async (target: { machineId: string; paneId: string; agent: string; sessionName?: string }) => {
    await submitPrompt(target.machineId, target.paneId, reviewMessage());
    setDraft([]);
    setPick(false);
    setNotice(`Sent ${draft.length} comment${draft.length === 1 ? "" : "s"} to ${target.agent}${target.sessionName ? ` in ${target.sessionName}` : ""}.`);
    setTimeout(() => setNotice(""), 5000);
  };

  if (data === null && !error) return <div className="placeholder">Looking at the folder…</div>;
  if (data && !data.repo) {
    return (
      <div className="placeholder">
        <div>{basename(dir)} is not a git checkout — nothing to review.</div>
      </div>
    );
  }
  const files = data?.files ?? [];
  const commits = data?.commits ?? [];
  const since = data?.baseline ? new Date(data.baseline.at) : null;
  const added = files.reduce((a, f) => a + f.added, 0);
  const removed = files.reduce((a, f) => a + f.removed, 0);

  return (
    <div className="review">
      <div className="review-head">
        <div className="review-title">
          <span className="review-branch">
            <Icon name="layers" size={13} /> {data?.branch || "detached"}
          </span>
          <span className="review-sub">
            {since ? `since you looked ${ago(since)}` : "no baseline yet — everything uncommitted counts"}
            {commits.length > 0 && ` · ${commits.length} commit${commits.length === 1 ? "" : "s"}`}
            {files.length > 0 && ` · ${files.length} file${files.length === 1 ? "" : "s"} +${added} −${removed}`}
          </span>
        </div>
        <button className="icon-btn" title="Look again" onClick={() => setTick((t) => t + 1)}>
          <Icon name="arrow-up-down" size={14} />
        </button>
        <button className="primary review-seen" disabled={busy || (!files.length && !commits.length)} onClick={() => void seen()} title="Everything here is reviewed: the next look starts from now">
          <Icon name="check" size={14} /> Mark reviewed
        </button>
      </div>
      {error && <div className="chat-error">{error}</div>}
      {notice && <div className="tasks-notice">{notice}</div>}
      <div className="review-body">
        {commits.length > 0 && (
          <div className="review-commits">
            {commits.map((c) => (
              <div key={c.sha} className="review-commit">
                <code>{c.sha}</code>
                <span className="review-commit-subject">{c.subject}</span>
                <span className="review-commit-when">{ago(new Date(c.when * 1000))}</span>
              </div>
            ))}
          </div>
        )}
        {files.length === 0 && commits.length === 0 && <div className="placeholder">Nothing has changed here{since ? " since you looked" : ""}.</div>}
        {files.map((f) => (
          <FileRow
            key={`${f.path}|${tick}`} // a new look re-reads the diff
            file={f}
            open={open === f.path}
            onToggle={() => setOpen(open === f.path ? null : f.path)}
            load={() => fetchReviewDiff(machineId, dir, f.path)}
            canComment={!!agent || agents.length > 0}
            draft={draft.filter((c) => c.file === f.path)}
            onComment={(c) => addComment({ ...c, file: f.path })}
            onRemove={removeComment}
            onOpen={(line) => onOpenFile(`${dir}/${f.path}`, line)}
          />
        ))}
        {data && draft.some((c) => !files.some((f) => f.path === c.file)) && (
          <div className="review-orphans">
            <div className="review-orphans-head">Comments on files no longer in this list — still in the draft:</div>
            {draft
              .filter((c) => !files.some((f) => f.path === c.file))
              .map((c) => (
                <Note key={c.id} comment={c} where={`${c.file}:${c.line}`} onRemove={removeComment} />
              ))}
          </div>
        )}
      </div>
      {draft.length > 0 && (
        <div className="review-draft-bar">
          <span className="review-draft-count">
            <Icon name="message" size={14} /> {draft.length} comment{draft.length === 1 ? "" : "s"}
          </span>
          <button className="linkish" onClick={() => setDraft([])}>
            Clear
          </button>
          {agent ? (
            <>
              <button className="primary" onClick={() => void sendDraft({ machineId, paneId, agent }).catch((e) => setError(errMsg(e)))}>
                <Icon name="send" size={13} /> Send to {agent}
              </button>
              {agents.length > 1 && (
                <button className="icon-btn" title="Send to another agent" onClick={() => setPick(true)}>
                  <Icon name="chevron-down" size={14} />
                </button>
              )}
            </>
          ) : (
            <button className="primary" disabled={agents.length === 0} onClick={() => setPick(true)}>
              <Icon name="send" size={13} /> Send to an agent…
            </button>
          )}
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
          onSend={async (target, text) => {
            await submitPrompt(target.machineId, target.paneId, text);
            setDraft([]);
            setPick(false);
            setNotice(`Sent ${draft.length} comment${draft.length === 1 ? "" : "s"} to ${target.agent} in ${target.sessionName}.`);
            setTimeout(() => setNotice(""), 5000);
          }}
        />
      )}
    </div>
  );
}

type NewComment = Omit<DraftComment, "id" | "file">;

function FileRow({ file: f, open, onToggle, load, canComment, draft, onComment, onRemove, onOpen }: {
  file: ReviewFile;
  open: boolean;
  onToggle: () => void;
  load: () => Promise<string>;
  canComment: boolean;
  draft: DraftComment[]; // this file's comments
  onComment: (c: NewComment) => void;
  onRemove: (id: number) => void;
  onOpen: (line?: number) => void;
}) {
  const [diff, setDiff] = useState<string | null>(null);
  const [err, setErr] = useState("");
  useEffect(() => {
    if (!open || diff !== null) return;
    load().then(setDiff, (e) => setErr(errMsg(e)));
  }, [open, diff, load]);
  return (
    <div className={`review-file${f.fresh ? " fresh" : ""}${open ? " open" : ""}`}>
      <button type="button" className="review-file-head" onClick={onToggle}>
        <span className={`chevron${open ? " open" : ""}`}>
          <Icon name="chevron-right" size={12} />
        </span>
        {f.fresh && <span className="review-dot" title="Changed since you looked" />}
        <span className="review-path">
          <span className="review-dir">{dirname(f.path)}</span>
          {basename(f.path)}
        </span>
        {draft.length > 0 && (
          <span className="review-tag review-tag-comments" title={`${draft.length} comment${draft.length === 1 ? "" : "s"} in the draft`}>
            {draft.length}
          </span>
        )}
        {f.untracked ? <span className="review-tag">new</span> : <span className="review-counts">+{f.added} −{f.removed}</span>}
        <span
          className="linkish"
          role="button"
          title="Open the file"
          onClick={(e) => {
            e.stopPropagation();
            onOpen();
          }}
        >
          <Icon name="external-link" size={12} />
        </span>
      </button>
      {open && (err ? <div className="chat-error">{err}</div> : diff === null ? <div className="placeholder small">Loading…</div> : <Diff text={diff} canComment={canComment} draft={draft} onComment={onComment} onRemove={onRemove} />)}
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
  if (hunks.length === 0 && stray.length === 0) return <div className="placeholder small">No textual diff (binary, or identical).</div>;
  return (
    <div className={`fv-code fv-diff review-diff${canComment ? " can-comment" : ""}`}>
      {hunks.map((h, i) => (
        <Hunk key={i} hunk={h} canComment={canComment} draft={draft} onComment={onComment} onRemove={onRemove} />
      ))}
      {stray.length > 0 && (
        <div className="review-orphans">
          <div className="review-orphans-head">No longer on a line of this diff — still in the draft:</div>
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
      <button type="button" className="icon-btn" title="Remove this comment" onClick={() => onRemove(c.id)}>
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
      {rows.map((r, i) => (
        <div key={i} className="review-row">
          <div
            className={`fv-line ${r.kind}${canComment && r.kind !== "meta" ? " commentable" : ""}${at === i ? " composing" : ""}`}
            onClick={() => {
              if (!canComment || r.kind === "meta") return;
              setAt(at === i ? null : i);
              setText("");
            }}
            title={canComment && r.kind !== "meta" ? "Comment on this line" : undefined}
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
                <button type="button" onClick={() => setAt(null)}>
                  Cancel
                </button>
                <button type="submit" className="primary" disabled={!text.trim()}>
                  <Icon name="plus" size={13} /> Add to the review
                </button>
              </div>
            </form>
          )}
        </div>
      ))}
    </div>
  );
}

const basename = (p: string) => p.split("/").filter(Boolean).pop() ?? p;
const dirname = (p: string) => {
  const i = p.lastIndexOf("/");
  return i < 0 ? "" : p.slice(0, i + 1);
};

function ago(d: Date): string {
  const m = Math.round((Date.now() - d.getTime()) / 60000);
  if (m < 1) return "just now";
  if (m < 60) return `${m} min ago`;
  const h = Math.round(m / 60);
  if (h < 48) return `${h} h ago`;
  return `${Math.round(h / 24)} days ago`;
}
