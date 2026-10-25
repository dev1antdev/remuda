// One hunk of a file's diff: its rows in one column or side by side, the words that changed between a removed
// line and its replacement marked, the lines around it unfolded from the file on request, a comment box under the
// tapped row and the notes already on it. Memoised: a keystroke in one box, a roster poll or an unfold in another
// hunk must not redraw thousands of cells.

import { memo, useMemo, useRef, useState } from "react";
import { langOf, splitTokens, type Token } from "@/lib/highlight";
import { plural } from "@/lib/text";
import { Icon, Tip } from "@/ui";
import { tokenNodes } from "@/ui/Tokens";
import { gapAbove, gapBelow, gapLength, surround, UNFOLD_STEP, type Unfold } from "./context";
import type { DraftComment } from "./draft";
import { changedSpan, pairRows, partners, rowsOf, rowTokens, type DiffHunk, type DiffRow } from "./hunks";

export type NewComment = Omit<DraftComment, "id" | "file">;

export const rowKey = (row: DiffRow) => `${row.kind === "del" ? "-" : "+"}${row.kind === "del" ? row.oldLine : row.newLine}\u0000${row.text}`;
export const commentKey = (comment: DraftComment) => `${comment.removed ? "-" : "+"}${comment.line}\u0000${comment.code}`;

type Side = "old" | "new" | "both"; // the column a row is drawn in; both = the one-column view

export const Hunk = memo(function Hunk({ hunk, prev, next, index, split, canComment, draft, file, onComment, onRemove, unfold, lines, blocked, onUnfold }: {
  hunk: DiffHunk;
  prev: DiffHunk | null;
  next: DiffHunk | null;
  index: number;
  split: boolean;
  canComment: boolean;
  draft: DraftComment[];
  file: string;
  onComment: (file: string, comment: NewComment) => void;
  onRemove: (id: number) => void;
  unfold: Unfold | undefined; // the lines of the gaps this hunk has unfolded
  lines: string[] | null; // the file's lines, once the first unfold asked for them
  blocked: string | null; // why the file's lines cannot be had, when they cannot
  onUnfold: (index: number, side: "above" | "below", count: number) => void;
}) {
  const box = useRef<HTMLDivElement>(null);
  const [composingAt, setComposingAt] = useState<string | null>(null);
  const around = useMemo(() => surround(hunk, prev, next, unfold, lines), [hunk, prev, next, unfold, lines]);
  const rows = useMemo(() => [...around.above, ...rowsOf(hunk), ...around.below], [hunk, around]);
  const pairs = useMemo(() => pairRows(rows), [rows]);
  const partnerOf = useMemo(() => partners(pairs), [pairs]);
  const tokensOf = useMemo(() => rowTokens(rows, langOf(file)), [rows, file]);
  const leftAbove = gapLength(gapAbove(hunk, prev)) - around.above.length;
  const below = next ? null : gapBelow(hunk, null, lines?.length ?? null);
  const leftBelow = next ? 0 : below ? gapLength(below) - around.below.length : null; // null: the file's length is not known yet
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
        <Source row={row} partner={partnerOf.get(row) ?? null} marker={side === "both"} tokens={tokensOf.get(row)} />
      </div>
    );
  };
  const notes = (row: DiffRow) => commentsOn(row).map((comment) => <Note key={comment.id} comment={comment} onRemove={onRemove} />);
  const composer = (key: string, row: DiffRow | null) => (composingAt === key && row ? <CommentBox key={key} row={row} onAdd={(text) => addComment(row, text)} onClose={closeComposer} /> : null);
  // the key of a row stays its position in the hunk's own rows, so an unfold above does not remount them
  const keyOf = (position: number) => position - around.above.length;
  return (
    <div className="review-hunk" ref={box}>
      <div className="fv-line hunk">
        <UnfoldStep side="above" left={leftAbove} blocked={blocked} onClick={() => onUnfold(index, "above", UNFOLD_STEP)} />
        <span className="fv-src">{hunk.header}</span>
        <UnfoldAll left={leftAbove} blocked={blocked} onClick={() => onUnfold(index, "above", Infinity)} />
      </div>
      {split
        ? pairs.map((pair, position) => (
            <div key={keyOf(position)}>
              <div className="fv-pair">
                {cell(pair.old, "old", `${keyOf(position)}:old`)}
                {cell(pair.new, "new", `${keyOf(position)}:new`)}
              </div>
              {pair.old && pair.old !== pair.new && notes(pair.old)}
              {pair.new && notes(pair.new)}
              {composer(`${keyOf(position)}:old`, pair.old) ?? composer(`${keyOf(position)}:new`, pair.new)}
            </div>
          ))
        : rows.map((row, position) => (
            <div key={keyOf(position)}>
              {cell(row, "both", `${keyOf(position)}`)}
              {notes(row)}
              {composer(`${keyOf(position)}`, row)}
            </div>
          ))}
      {leftBelow !== 0 && (
        <div className="fv-line hunk hunk-foot">
          <UnfoldStep side="below" left={leftBelow} blocked={blocked} onClick={() => onUnfold(index, "below", UNFOLD_STEP)} />
          <span className="fv-src">{leftBelow === null ? "the rest of the file" : `${plural(leftBelow, "line")} to the end of the file`}</span>
          <UnfoldAll left={leftBelow} blocked={blocked} onClick={() => onUnfold(index, "below", Infinity)} />
        </div>
      )}
    </div>
  );
});

// UnfoldStep is the button at a hunk's edge for ten more lines of the gap there; it stays, disabled, when the file's lines cannot be had
function UnfoldStep({ side, left, blocked, onClick }: { side: "above" | "below"; left: number | null; blocked: string | null; onClick: () => void }) {
  if (left === 0) return <span className="hunk-unfold" />;
  const what = `${Math.min(UNFOLD_STEP, left ?? UNFOLD_STEP)} more ${left === 1 ? "line" : "lines"} ${side}`;
  return (
    <span className="hunk-unfold">
      <Tip text={blocked ?? what}>
        <button type="button" className="icon-btn sm" aria-label={what} aria-disabled={blocked ? true : undefined} onClick={blocked ? undefined : onClick}>
          <Icon name={side === "above" ? "chevron-up" : "chevron-down"} size={12} />
        </button>
      </Tip>
    </span>
  );
}

// UnfoldAll is the link at a hunk's other edge for the whole gap; without the file's length it offers the rest of the file
function UnfoldAll({ left, blocked, onClick }: { left: number | null; blocked: string | null; onClick: () => void }) {
  if (left === 0) return null;
  return (
    <button type="button" className="linkish hunk-unfold-all" aria-disabled={blocked ? true : undefined} title={blocked ?? undefined} onClick={blocked ? undefined : onClick}>
      {left === null ? "all" : `all ${plural(left, "line")}`}
    </button>
  );
}

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

// Source is a row's text, the +/-/space marker kept in the one-column view, the words that changed against its partner
// marked, the code coloured in the file's language when there are tokens for it (the mark cuts the tokens, so both hold)
function Source({ row, partner, marker, tokens }: { row: DiffRow; partner: DiffRow | null; marker: boolean; tokens: Token[] | undefined }) {
  const text = row.text.slice(1);
  const lead = marker ? row.text.slice(0, 1) : "";
  const other = partner?.text.slice(1);
  const span = other === undefined ? null : row.kind === "del" ? changedSpan(text, other) : changedSpan(other, text);
  const coloured = tokens ?? [{ kind: "plain" as const, text }];
  if (!span) {
    return (
      <span className="fv-src">
        {lead}
        {tokenNodes(coloured)}
      </span>
    );
  }
  const end = text.length - span.tail;
  const { before, inside, after } = splitTokens(coloured, span.head, end);
  return (
    <span className="fv-src">
      {lead}
      {tokenNodes(before, "b")}
      {end > span.head && <mark className="fv-mark">{tokenNodes(inside, "m")}</mark>}
      {tokenNodes(after, "a")}
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
