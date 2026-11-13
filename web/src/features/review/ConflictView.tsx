// A conflicted file opened in the lens: its conflict blocks alone with a few lines around each, at their real line
// numbers, the markers on a raised band, yours and incoming tinted — beside each other in two columns. Showing is all it
// does: the person or an agent resolves the file, then stages it.

import { useMemo } from "react";
import { t } from "@/lib/i18n";
import { useQuery } from "@/lib/query";
import { Notice } from "@/ui";
import { fetchConflict } from "./api";
import { conflictSentence, conflictView, isPair, type ConflictLine } from "./conflicts";

export function ConflictView({ machineId, dir, path, split }: { machineId: string; dir: string; path: string; split: boolean }) {
  const { data: read, error } = useQuery(() => fetchConflict(machineId, dir, path), [machineId, dir, path]);
  const view = useMemo(() => (read && !read.binary && !read.missing ? conflictView(read.text) : null), [read]);
  if (error) return <Notice tone="error">{error}</Notice>;
  if (!read) return <div className="empty">{t("Loading…")}</div>;
  const sentence = conflictSentence(read);
  if (sentence || !view) return <div className="empty conflict-sentence">{sentence}</div>;
  return (
    <div className={`fv-code fv-diff review-diff conflict-view${split ? " columns" : ""}`}>
      {split && (
        <div className="fv-pair conflict-sides" aria-hidden="true">
          <div className="fv-line meta">{t("Yours")}</div>
          <div className="fv-line meta">{t("Incoming")}</div>
        </div>
      )}
      {split
        ? view.split.map((row, index) =>
            isPair(row) ? (
              <div key={index} className="fv-pair">
                <Line row={row.left} />
                <Line row={row.right} />
              </div>
            ) : (
              <Line key={index} row={row} />
            ),
          )
        : view.inline.map((row, index) => <Line key={index} row={row} />)}
      {read.truncated && <Notice tone="hint">{t("The file is longer than the lens reads: open it to see the rest.")}</Notice>}
    </div>
  );
}

function Line({ row }: { row: ConflictLine | null }) {
  if (!row) return <div className="fv-line empty" aria-hidden="true" />;
  if (row.kind === "heading") return <div className="fv-line hunk conflict-heading">{row.text}</div>;
  return (
    <div className={`fv-line ${row.kind}`}>
      <span className="fv-ln">{row.line ?? ""}</span>
      <span className="fv-src">{row.text}</span>
    </div>
  );
}
