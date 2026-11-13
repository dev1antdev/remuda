// The file viewer's File tab: the file's lines coloured in its language, with what changed since HEAD at the rows' edge —
// a green bar for lines added, an amber one for lines changed, a thin red line where lines were removed. A click on a
// changed row or on the red line shows the old lines in place, in red, above the new ones; again hides them.

import { useMemo, useState } from "react";
import { highlightLines, type Token } from "@/lib/highlight";
import { tokenNodes } from "@/ui/Tokens";
import { tn } from "@/lib/i18n";
import { changeKey, codeRows, lineCountOf, opens, toggled, type Change, type CodeRow } from "./gutter";

// CodeLines splits and colours once per text, not per render: a 512KB file is 10k rows (past the highlighter's cap they stay plain)
export default function CodeLines({ text, lang, line, lineRef, changes }: { text: string; lang: string; line?: number; lineRef: React.RefObject<HTMLDivElement | null>; changes: Change[] }) {
  const tokens = useMemo<Token[][]>(() => (lang ? highlightLines(text, lang) : text.split("\n").map((l) => [{ kind: "plain" as const, text: l }])), [text, lang]);
  const count = useMemo(() => lineCountOf(text), [text]);
  const [open, setOpen] = useState<ReadonlySet<string>>(() => new Set());
  const rows = useMemo(() => codeRows(count, changes, open), [count, changes, open]);
  const toggle = (change: Change | undefined) => {
    if (change && opens(change)) setOpen((was) => toggled(was, change));
  };
  return (
    <div className="fv-code fv-gutter">
      {rows.map((row) => (
        <Row key={rowKey(row)} row={row} tokens={row.kind === "line" ? tokens[row.index] : undefined} hit={row.kind === "line" && row.index + 1 === line} lineRef={lineRef} open={!!row.change && open.has(changeKey(row.change))} onToggle={toggle} />
      ))}
    </div>
  );
}

const rowKey = (row: CodeRow): string => (row.kind === "line" ? `l${row.index}` : row.kind === "old" ? `o${changeKey(row.change)}-${row.number}` : `r${changeKey(row.change)}`);

function Row({ row, tokens, hit, lineRef, open, onToggle }: { row: CodeRow; tokens?: Token[]; hit: boolean; lineRef: React.RefObject<HTMLDivElement | null>; open: boolean; onToggle: (change: Change | undefined) => void }) {
  if (row.kind === "removed") {
    const lines = row.change.old.length;
    return (
      <button type="button" className="linkish fv-removed" aria-expanded={false} aria-label={tn(lines, "Show the line removed here", "Show the {n} lines removed here")} onClick={() => onToggle(row.change)}>
        <span className="fv-removed-mark" />
      </button>
    );
  }
  if (row.kind === "old")
    return (
      <div className="fv-line is-old" onClick={() => onToggle(row.change)}>
        <span className="fv-ln">{row.number}</span>
        <span className="fv-src">{row.text || " "}</span>
      </div>
    );
  const change = row.change;
  const tone = !change ? "" : change.kind === "added" || open ? " is-added" : " is-changed";
  const first = !!change && opens(change) && row.index === change.line;
  return (
    <div ref={hit ? lineRef : undefined} className={`fv-line${tone}${hit ? " hit" : ""}${opens(change) ? " opens" : ""}`} onClick={opens(change) ? () => onToggle(change) : undefined}>
      {first ? (
        <button
          type="button"
          className="linkish fv-ln fv-toggle"
          aria-expanded={open}
          aria-label={open ? tn(change.old.length, "Hide the line it replaced", "Hide the {n} lines it replaced") : tn(change.old.length, "Show the line it replaced", "Show the {n} lines it replaced")}
          onClick={(e) => {
            e.stopPropagation();
            onToggle(change);
          }}
        >
          {row.index + 1}
        </button>
      ) : (
        <span className="fv-ln">{row.index + 1}</span>
      )}
      <span className="fv-src">{tokens ? tokenNodes(tokens) : null}</span>
    </div>
  );
}
