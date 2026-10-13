// One file of the review: its row, and its diff by hunks with a comment box under a tapped line.

import { useMemo, useState } from "react";
import { useQuery } from "@/lib/query";
import { basename, dirname, plural } from "@/lib/text";
import { Icon, Notice } from "@/ui";
import type { ReviewFile } from "./api";
import type { DraftComment } from "./draft";
import { rowsOf, splitHunks, type DiffHunk, type DiffRow } from "./hunks";

export type NewComment = Omit<DraftComment, "id" | "file">;

export function FileRow({ file, open, onToggle, load, canComment, draft, onComment, onRemove, onOpen }: {
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
  const [attempt, setAttempt] = useState(0);
  const { data: diff, error } = useQuery(() => (open ? load(file.path) : Promise.resolve(undefined)), [open, load, file.path, attempt], { enabled: open, keep: true });
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
        (error ? (
          <Notice
            tone="error"
            action={
              <button type="button" className="linkish" onClick={() => setAttempt((n) => n + 1)}>
                Try again
              </button>
            }
          >
            {error}
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

export function Note({ comment, where, onRemove }: { comment: DraftComment; where?: string; onRemove: (id: number) => void }) {
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
