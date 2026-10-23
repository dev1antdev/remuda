// One hunk of a file's diff: its rows in one column or side by side, the words that changed between a removed
// line and its replacement marked, a comment box under the tapped row and the notes already on it. Memoised:
// a keystroke in one box or a roster poll must not redraw thousands of cells.

import { memo, useMemo, useRef, useState } from "react";
import { Icon } from "@/ui";
import type { DraftComment } from "./draft";
import { changedSpan, pairRows, partners, rowsOf, type DiffHunk, type DiffRow } from "./hunks";

export type NewComment = Omit<DraftComment, "id" | "file">;

export const rowKey = (row: DiffRow) => `${row.kind === "del" ? "-" : "+"}${row.kind === "del" ? row.oldLine : row.newLine}\u0000${row.text}`;
export const commentKey = (comment: DraftComment) => `${comment.removed ? "-" : "+"}${comment.line}\u0000${comment.code}`;

type Side = "old" | "new" | "both"; // the column a row is drawn in; both = the one-column view

export const Hunk = memo(function Hunk({ hunk, split, canComment, draft, file, onComment, onRemove }: {
  hunk: DiffHunk;
  split: boolean;
  canComment: boolean;
  draft: DraftComment[];
  file: string;
  onComment: (file: string, comment: NewComment) => void;
  onRemove: (id: number) => void;
}) {
  const box = useRef<HTMLDivElement>(null);
  const [composingAt, setComposingAt] = useState<string | null>(null);
  const rows = useMemo(() => rowsOf(hunk), [hunk]);
  const pairs = useMemo(() => pairRows(rows), [rows]);
  const partnerOf = useMemo(() => partners(pairs), [pairs]);
  const commentsOn = (row: DiffRow) => (row.kind === "meta" ? [] : draft.filter((comment) => commentKey(comment) === rowKey(row)));
  // the box closed: the focus goes back to its row, so the arrow keys carry on from there
  const closeComposer = () => {
    const key = composingAt;
    setComposingAt(null);
    if (key) requestAnimationFrame(() => box.current?.querySelector<HTMLElement>(`[data-key="${CSS.escape(key)}"]`)?.focus({ preventScroll: true }));
  };
  const toggleComposer = (key: string) => {
    const selection = window.getSelection();
    if (selection && !selection.isCollapsed && box.current?.contains(selection.anchorNode)) return; // a drag that selected text here is a copy, not a tap
    if (composingAt === key) closeComposer();
    else setComposingAt(key);
  };
  const addComment = (row: DiffRow, text: string) => {
    onComment(file, { line: (row.kind === "del" ? row.oldLine : row.newLine) ?? hunk.newStart, removed: row.kind === "del", code: row.text, text });
    closeComposer();
  };
  const cell = (row: DiffRow | null, side: Side, key: string) => {
    if (!row) return <div key={key} className="fv-line empty" aria-hidden="true" />;
    const walkable = row.kind !== "meta"; // the arrow keys stop here, with or without an agent to comment to
    const commentable = canComment && walkable;
    const line = side === "old" ? row.oldLine : (row.newLine ?? row.oldLine);
    return (
      <div
        key={key}
        data-key={key}
        className={`fv-line ${row.kind}${walkable ? " walk" : ""}${commentable ? " commentable" : ""}${composingAt === key ? " composing" : ""}`}
        role={commentable ? "button" : undefined}
        aria-expanded={commentable ? composingAt === key : undefined}
        tabIndex={walkable ? 0 : undefined}
        data-line={line ?? ""}
        onClick={commentable ? () => toggleComposer(key) : undefined}
        onKeyDown={
          commentable
            ? (event) => {
                if (event.target !== event.currentTarget) return;
                if (event.key === "Enter" || event.key === " ") {
                  event.preventDefault();
                  toggleComposer(key);
                }
              }
            : undefined
        }
        title={commentable ? "Comment on this line" : undefined}
      >
        <span className="fv-ln">{line ?? ""}</span>
        <Source row={row} partner={partnerOf.get(row) ?? null} marker={side === "both"} />
      </div>
    );
  };
  const notes = (row: DiffRow) => commentsOn(row).map((comment) => <Note key={comment.id} comment={comment} onRemove={onRemove} />);
  const composer = (key: string, row: DiffRow | null) => (composingAt === key && row ? <CommentBox key={key} row={row} onAdd={(text) => addComment(row, text)} onClose={closeComposer} /> : null);
  return (
    <div className="review-hunk" ref={box}>
      <div className="fv-line hunk">
        <span className="fv-src">{hunk.header}</span>
      </div>
      {split
        ? pairs.map((pair, index) => (
            <div key={index}>
              <div className="fv-pair">
                {cell(pair.old, "old", `${index}:old`)}
                {cell(pair.new, "new", `${index}:new`)}
              </div>
              {pair.old && pair.old !== pair.new && notes(pair.old)}
              {pair.new && notes(pair.new)}
              {composer(`${index}:old`, pair.old) ?? composer(`${index}:new`, pair.new)}
            </div>
          ))
        : rows.map((row, index) => (
            <div key={index}>
              {cell(row, "both", `${index}`)}
              {notes(row)}
              {composer(`${index}`, row)}
            </div>
          ))}
    </div>
  );
});

// CommentBox holds the text being typed, so a keystroke redraws the box and nothing else
function CommentBox({ row, onAdd, onClose }: { row: DiffRow; onAdd: (text: string) => void; onClose: () => void }) {
  const [text, setText] = useState("");
  const add = () => {
    const trimmed = text.trim();
    if (trimmed) onAdd(trimmed);
  };
  return (
    <form
      className="review-comment"
      onSubmit={(event) => {
        event.preventDefault();
        add();
      }}
    >
      <textarea
        autoFocus
        rows={3}
        value={text}
        placeholder={`What should change at line ${row.newLine ?? row.oldLine}? It joins the draft; everything goes to the agent in one message.`}
        onChange={(event) => setText(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Escape") onClose();
          if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) add();
        }}
      />
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          Cancel
        </button>
        <button type="submit" className="btn primary" disabled={!text.trim()}>
          <Icon name="plus" size={12} /> Add to the review
        </button>
      </div>
    </form>
  );
}

// Source is a row's text, the +/-/space marker kept in the one-column view, the words that changed against its partner marked
function Source({ row, partner, marker }: { row: DiffRow; partner: DiffRow | null; marker: boolean }) {
  const text = row.text.slice(1);
  const lead = marker ? row.text.slice(0, 1) : "";
  const other = partner?.text.slice(1);
  const span = other === undefined ? null : row.kind === "del" ? changedSpan(text, other) : changedSpan(other, text);
  if (!span) {
    return (
      <span className="fv-src">
        {lead}
        {text}
      </span>
    );
  }
  const end = text.length - span.tail;
  return (
    <span className="fv-src">
      {lead}
      {text.slice(0, span.head)}
      {end > span.head && <mark className="fv-mark">{text.slice(span.head, end)}</mark>}
      {text.slice(end)}
    </span>
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
