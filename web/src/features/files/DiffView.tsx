// The file viewer's Changes tab: git's diff of the file against HEAD, its code coloured, or why there is none.

import type { FileDiff } from "./api";
import { tokenNodes } from "@/ui/Tokens";
import type { DiffLine } from "./diffLines";
import { t } from "@/lib/i18n";

// the classes are the review's: a kept line has none, a note (no newline at the end) reads as the header does
const LINE_CLASS: Record<DiffLine["kind"], string> = { add: "add", del: "del", hunk: "hunk", meta: "meta", note: "meta", context: "" };

export default function DiffView({ diff, lines }: { diff: FileDiff | null; lines: DiffLine[] }) {
  if (!diff) return <div className="empty">{t("Could not run git here.")}</div>;
  if (!diff.repo) return <div className="empty">{t("Not inside a git repository.")}</div>;
  if (diff.untracked) return <div className="empty">{t("New file — not yet added to git, so the whole file is the change.")}</div>;
  if (!diff.diff) return <div className="empty">{t("No uncommitted changes — the file matches HEAD.")}</div>;
  return (
    <div className="fv-code fv-diff">
      {lines.map((line, i) => (
        <div key={i} className={`fv-line ${LINE_CLASS[line.kind]}`}>
          <span className="fv-src">{line.tokens ? [line.text.slice(0, 1), ...tokenNodes(line.tokens)] : line.text}</span>
        </div>
      ))}
    </div>
  );
}
