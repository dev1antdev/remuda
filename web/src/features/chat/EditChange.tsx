// The change an Edit or MultiEdit call made, drawn under its summary in the chat lens: a removed line on red, an added
// one on green, the words that differ marked as the Changes lens marks them, the rows past a couple dozen folded.

import { useMemo, useState } from "react";
import type { Part } from "./api";
import { editRows, editsOf, foldRows, type EditRow } from "./editDiff";

export const FOLD_ROWS = 24;

export const EDIT_CHANGE_TOOLS = new Set(["Edit", "MultiEdit"]);

export function EditChange({ part }: { part: Part }) {
  const rows = useMemo(() => rowsOf(part), [part.tool_name, part.input]);
  const [unfolded, setUnfolded] = useState(false);
  if (!rows.length) return null;
  const { shown, more } = unfolded ? { shown: rows, more: 0 } : foldRows(rows, FOLD_ROWS);
  return (
    <div className="edit-diff fv-diff" aria-label="the change this call made">
      {shown.map((row, i) => (
        <Row key={i} row={row} />
      ))}
      {more > 0 && (
        <button type="button" className="linkish edit-more" onClick={() => setUnfolded(true)}>
          {more} more lines
        </button>
      )}
    </div>
  );
}

// rowsOf is every edit of the call as rows, a MultiEdit's edits each headed by its place
function rowsOf(part: Part): EditRow[] {
  const edits = editsOf(part.tool_name, part.input);
  if (edits.length <= 1) return edits.flatMap((edit) => editRows(edit.old, edit.new));
  return edits.flatMap((edit, i) => [{ kind: "hunk" as const, text: `edit ${i + 1} of ${edits.length}${edit.replaceAll ? ", every occurrence" : ""}` }, ...editRows(edit.old, edit.new)]);
}

function Row({ row }: { row: EditRow }) {
  if (row.kind === "meta" || row.kind === "hunk") {
    return (
      <div className={`fv-line ${row.kind}`}>
        <span className="fv-src">{row.text}</span>
      </div>
    );
  }
  const sign = row.kind === "del" ? "-" : row.kind === "add" ? "+" : " ";
  return (
    <div className={row.kind === "ctx" ? "fv-line" : `fv-line ${row.kind}`}>
      <span className="edit-sign">{sign}</span>
      <span className="fv-src">{marked(row)}</span>
    </div>
  );
}

// marked is the row's text with the span that differs from its partner wrapped, or the text whole; an empty line keeps its height
function marked(row: EditRow) {
  const text = row.text;
  if (!text) return " ";
  if (!row.mark) return text;
  const end = text.length - row.mark.tail;
  if (end <= row.mark.head) return text;
  return (
    <>
      {text.slice(0, row.mark.head)}
      <mark className="fv-mark">{text.slice(row.mark.head, end)}</mark>
      {text.slice(end)}
    </>
  );
}
