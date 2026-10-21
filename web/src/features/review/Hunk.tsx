// One hunk of a file's diff: its rows in one column or side by side, the words that changed between a removed
// line and its replacement marked, a comment box under the tapped row and the notes already on it.

import { useMemo, useState } from "react";
import { Icon } from "@/ui";
import type { DraftComment } from "./draft";
import { changedSpan, pairRows, partners, rowsOf, type DiffHunk, type DiffRow } from "./hunks";

export type NewComment = Omit<DraftComment, "id" | "file">;

export const rowKey = (row: DiffRow) => `${row.kind === "del" ? "-" : "+"}${row.kind === "del" ? row.oldLine : row.newLine}\u0000${row.text}`;
export const commentKey = (comment: DraftComment) => `${comment.removed ? "-" : "+"}${comment.line}\u0000${comment.code}`;

type Side = "old" | "new" | "both"; // the column a row is drawn in; both = the one-column view

export function Hunk({ hunk, split, canComment, draft, onComment, onRemove }: {
  hunk: DiffHunk;
  split: boolean;
  canComment: boolean;
  draft: DraftComment[];
  onComment: (comment: NewComment) => void;
  onRemove: (id: number) => void;
}) {
  const [composingAt, setComposingAt] = useState<string | null>(null);
  const [draftText, setDraftText] = useState("");
  const rows = useMemo(() => rowsOf(hunk), [hunk]);
  const pairs = useMemo(() => pairRows(rows), [rows]);
  const partnerOf = useMemo(() => partners(pairs), [pairs]);
  const commentsOn = (row: DiffRow) => (row.kind === "meta" ? [] : draft.filter((comment) => commentKey(comment) === rowKey(row)));
  const toggleComposer = (key: string) => {
    if (window.getSelection()?.toString()) return; // a drag that selected text is a copy, not a tap
    setComposingAt(composingAt === key ? null : key);
    setDraftText("");
  };
  const addComment = (row: DiffRow) => {
    const text = draftText.trim();
    if (!text) return;
    onComment({ line: (row.kind === "del" ? row.oldLine : row.newLine) ?? hunk.newStart, removed: row.kind === "del", code: row.text, text });
    setComposingAt(null);
    setDraftText("");
  };
  const cell = (row: DiffRow | null, side: Side, key: string) => {
    if (!row) return <div key={key} className="fv-line empty" aria-hidden="true" />;
    const commentable = canComment && row.kind !== "meta";
    const line = side === "old" ? row.oldLine : (row.newLine ?? row.oldLine);
    return (
      <div
        key={key}
        className={`fv-line ${row.kind}${commentable ? " commentable" : ""}${composingAt === key ? " composing" : ""}`}
        role={commentable ? "button" : undefined}
        tabIndex={commentable ? 0 : undefined}
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
  const composer = (key: string, row: DiffRow | null) =>
    composingAt === key && row ? (
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
    ) : null;
  return (
    <div className="review-hunk">
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
