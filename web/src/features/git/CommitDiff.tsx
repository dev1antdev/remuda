// One file's patch in a commit, drawn as the Changes lens draws its hunks — the review's own rows and their Source
// (features/review): the words that changed between a removed line and its replacement marked, the code coloured in the
// file's language, the lines numbered — in one column, read-only.

import { memo, useMemo } from "react";
import { langOf } from "@/lib/highlight";
import { Source } from "@/features/review/Hunk";
import { pairRows, partners, rowsOf, rowTokens, splitHunks, type DiffHunk } from "@/features/review/hunks";
import { t } from "@/lib/i18n";

export function CommitDiff({ patch, path, binary }: { patch: string; path: string; binary?: boolean }) {
  const hunks = useMemo(() => splitHunks(patch), [patch]);
  if (!hunks.length) {
    const note = binary || /^Binary files /m.test(patch) ? t("A binary file changed.") : /^rename from /m.test(patch) ? t("Renamed, its text unchanged.") : t("No text changes.");
    return <p className="git-empty-note">{note}</p>;
  }
  return (
    <div className="fv-code fv-diff git-diff">
      {hunks.map((hunk, i) => (
        <PatchHunk key={i} hunk={hunk} path={path} />
      ))}
    </div>
  );
}

const PatchHunk = memo(function PatchHunk({ hunk, path }: { hunk: DiffHunk; path: string }) {
  const rows = useMemo(() => rowsOf(hunk), [hunk]);
  const partnerOf = useMemo(() => partners(pairRows(rows)), [rows]);
  const tokensOf = useMemo(() => rowTokens(rows, langOf(path)), [rows, path]);
  return (
    <>
      <div className="fv-line hunk">
        <span className="fv-src">{hunk.header}</span>
      </div>
      {rows.map((row, i) => (
        <div key={i} className={`fv-line ${row.kind}`}>
          <span className="fv-ln">{(row.kind === "del" ? row.oldLine : row.newLine) ?? ""}</span>
          <Source row={row} partner={partnerOf.get(row) ?? null} marker tokens={tokensOf.get(row)} />
        </div>
      ))}
    </>
  );
});
