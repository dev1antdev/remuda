// The change an Edit or MultiEdit call made, drawn under its summary in the chat lens: a removed line on red, an added
// one on green, the words that differ marked as the Changes lens marks them, the code coloured in the file's language,
// the rows past a couple dozen folded.

import { useMemo, useState } from "react";
import { langOf, splitTokens, type Token } from "@/lib/highlight";
import { tokenNodes } from "@/ui/Tokens";
import type { Part } from "./api";
import { editRows, editsOf, foldRows, rowTokens, type EditRow } from "./editDiff";
import { toolPath } from "./footprint";

export const FOLD_ROWS = 24;

export const EDIT_CHANGE_TOOLS = new Set(["Edit", "MultiEdit"]);

export function EditChange({ part }: { part: Part }) {
  const rows = useMemo(() => rowsOf(part), [part.tool_name, part.input]);
  const tokens = useMemo(() => rowTokens(rows, langOf(toolPath(part) ?? "")), [rows, part]);
  const [unfolded, setUnfolded] = useState(false);
  if (!rows.length) return null;
  const { shown, more } = unfolded ? { shown: rows, more: 0 } : foldRows(rows, FOLD_ROWS);
  return (
    <div className="edit-diff fv-diff" aria-label="the change this call made">
      {shown.map((row, i) => (
        <Row key={i} row={row} tokens={tokens.get(row)} />
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

function Row({ row, tokens }: { row: EditRow; tokens: Token[] | undefined }) {
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
      <span className="fv-src">{marked(row, tokens)}</span>
    </div>
  );
}

// marked is the row's coloured text with the span that differs from its partner wrapped, or the text whole; an empty line
// keeps its height
function marked(row: EditRow, tokens: Token[] | undefined) {
  const text = row.text;
  if (!text) return " ";
  const coloured = tokens ?? [{ kind: "plain" as const, text }];
  if (!row.mark) return tokenNodes(coloured);
  const end = text.length - row.mark.tail;
  if (end <= row.mark.head) return tokenNodes(coloured);
  const { before, inside, after } = splitTokens(coloured, row.mark.head, end);
  return (
    <>
      {tokenNodes(before, "b")}
      <mark className="fv-mark">{tokenNodes(inside, "m")}</mark>
      {tokenNodes(after, "a")}
    </>
  );
}
