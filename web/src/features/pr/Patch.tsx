// A patch GitHub sent — a file's, or the end of a thread's hunk — drawn by the file viewer's diff rows, coloured in the
// file's language.

import { useMemo } from "react";
import { diffLines, type DiffLine } from "@/features/files/diffLines";
import { langOf } from "@/lib/highlight";
import { tokenNodes } from "@/ui/Tokens";

const LINE_CLASS: Record<DiffLine["kind"], string> = { add: "add", del: "del", hunk: "hunk", meta: "meta", note: "meta", context: "" };

export default function Patch({ patch, path }: { patch: string; path: string }) {
  const lines = useMemo(() => diffLines(patch, langOf(path)), [patch, path]);
  return (
    <div className="fv-code fv-diff pr-patch">
      {lines.map((line, i) => (
        <div key={i} className={`fv-line ${LINE_CLASS[line.kind]}`}>
          <span className="fv-src">{line.tokens ? [line.text.slice(0, 1), ...tokenNodes(line.tokens)] : line.text}</span>
        </div>
      ))}
    </div>
  );
}
