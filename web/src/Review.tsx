// The Changes lens: what moved in the workspace's folder since the reviewer
// last looked — commits made since, files changed against that point (with
// the ones that moved since marked fresh), each file's diff on demand, a
// comment on any hunk that goes to the pane's agent as a prompt with the
// file and line, and "Mark reviewed" to move the baseline to now.

import { useEffect, useMemo, useState } from "react";
import { fetchReview, fetchReviewDiff, markReviewed, submitPrompt, type Review as ReviewData, type ReviewFile } from "./api";
import { errMsg } from "./prefs";
import Icon from "./icons";

export default function Review({ machineId, paneId, dir, agent, onOpenFile }: {
  machineId: string;
  paneId: string;
  dir: string;
  agent: string; // "" for a shell: nobody to send feedback to
  onOpenFile: (path: string, line?: number) => void;
}) {
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
      setNotice("Reviewed — the next look starts from here.");
      setTimeout(() => setNotice(""), 4000);
    } catch (e) {
      setError(errMsg(e));
    } finally {
      setBusy(false);
    }
  };

  const feedback = async (file: string, line: number | null, text: string) => {
    const where = line ? `${file}:${line}` : file;
    await submitPrompt(machineId, paneId, `Review feedback on ${where}:\n${text}`);
    setNotice(`Sent to ${agent}: ${where}`);
    setTimeout(() => setNotice(""), 4000);
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
            key={f.path}
            file={f}
            open={open === f.path}
            onToggle={() => setOpen(open === f.path ? null : f.path)}
            load={() => fetchReviewDiff(machineId, dir, f.path)}
            canComment={!!agent}
            onComment={(line, text) => feedback(f.path, line, text)}
            onOpen={(line) => onOpenFile(`${dir}/${f.path}`, line)}
          />
        ))}
      </div>
    </div>
  );
}

function FileRow({ file: f, open, onToggle, load, canComment, onComment, onOpen }: {
  file: ReviewFile;
  open: boolean;
  onToggle: () => void;
  load: () => Promise<string>;
  canComment: boolean;
  onComment: (line: number | null, text: string) => Promise<void>;
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
      {open && (err ? <div className="chat-error">{err}</div> : diff === null ? <div className="placeholder small">Loading…</div> : <Diff text={diff} canComment={canComment} onComment={onComment} />)}
    </div>
  );
}

// A unified diff, hunk by hunk, with a comment box per hunk when there is
// an agent to hear it.
function Diff({ text, canComment, onComment }: { text: string; canComment: boolean; onComment: (line: number | null, text: string) => Promise<void> }) {
  const hunks = useMemo(() => splitHunks(text), [text]);
  if (hunks.length === 0) return <div className="placeholder small">No textual diff (binary, or identical).</div>;
  return (
    <div className="fv-code fv-diff review-diff">
      {hunks.map((h, i) => (
        <Hunk key={i} hunk={h} canComment={canComment} onComment={onComment} />
      ))}
    </div>
  );
}

type HunkT = { header: string; start: number; lines: string[] };

function splitHunks(text: string): HunkT[] {
  const out: HunkT[] = [];
  let cur: HunkT | null = null;
  const lines = text.split("\n");
  if (lines[lines.length - 1] === "") lines.pop(); // the file's final newline, not a line
  for (const l of lines) {
    if (l.startsWith("@@")) {
      const m = /\+(\d+)/.exec(l);
      cur = { header: l, start: m ? Number(m[1]) : 1, lines: [] };
      out.push(cur);
    } else if (cur && !l.startsWith("+++") && !l.startsWith("---") && !l.startsWith("diff ") && !l.startsWith("index ")) {
      cur.lines.push(l);
    }
  }
  return out;
}

function Hunk({ hunk, canComment, onComment }: { hunk: HunkT; canComment: boolean; onComment: (line: number | null, text: string) => Promise<void> }) {
  const [composing, setComposing] = useState(false);
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  // the new-file line number of each row, for the feedback's address
  let n = hunk.start;
  const rows = hunk.lines.map((l) => {
    const kind = l.startsWith("+") ? "add" : l.startsWith("-") ? "del" : l.startsWith("\\") ? "meta" : "";
    const row = { l, kind, n: kind === "del" ? null : n };
    if (kind !== "del" && kind !== "meta") n++;
    return row;
  });
  return (
    <div className="review-hunk">
      <div className="fv-line hunk">
        <span className="fv-src">{hunk.header}</span>
        {canComment && !composing && (
          <button type="button" className="linkish review-comment-btn" onClick={() => setComposing(true)}>
            <Icon name="corner-down-left" size={11} /> comment
          </button>
        )}
      </div>
      {rows.map((r, i) => (
        <div key={i} className={`fv-line ${r.kind}`}>
          <span className="fv-ln">{r.n ?? ""}</span>
          <span className="fv-src">{r.l}</span>
        </div>
      ))}
      {composing && (
        <form
          className="review-comment"
          onSubmit={(e) => {
            e.preventDefault();
            if (!text.trim()) return;
            setBusy(true);
            onComment(hunk.start, text.trim()).then(
              () => {
                setComposing(false);
                setText("");
                setBusy(false);
              },
              () => setBusy(false),
            );
          }}
        >
          <textarea autoFocus rows={3} value={text} placeholder="What should change here? Goes to the agent with the file and line." onChange={(e) => setText(e.target.value)} />
          <div className="modal-actions">
            <button type="button" onClick={() => setComposing(false)}>
              Cancel
            </button>
            <button type="submit" className="primary" disabled={busy || !text.trim()}>
              <Icon name="send" size={13} /> Send to agent
            </button>
          </div>
        </form>
      )}
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
