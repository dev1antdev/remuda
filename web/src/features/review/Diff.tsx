// One file of the review: its row, and its diff by hunks, in one column or two.

import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@/lib/query";
import { basename, dirname, plural } from "@/lib/text";
import { Icon, Notice, Tip } from "@/ui";
import type { ReviewFile } from "./api";
import type { DraftComment } from "./draft";
import { commentKey, Hunk, Note, rowKey, type NewComment } from "./Hunk";
import { rowsOf, splitHunks } from "./hunks";

export type { NewComment } from "./Hunk";

export function FileRow({ file, open, split, onToggle, load, onLoaded, canComment, draft, onComment, onRemove, onOpen, onCode }: {
  file: ReviewFile;
  open: boolean;
  split: boolean;
  onToggle: () => void;
  load: (path: string) => Promise<string>;
  onLoaded?: (path: string) => void; // the diff is on screen
  canComment: boolean;
  draft: DraftComment[];
  onComment: (file: string, comment: NewComment) => void;
  onRemove: (id: number) => void;
  onOpen: (line?: number) => void;
  onCode?: () => void; // the file in the editor, when Code mode is there to take it
}) {
  const [attempt, setAttempt] = useState(0);
  const { data: diff, error } = useQuery(() => (open ? load(file.path) : Promise.resolve(undefined)), [open, load, file.path, attempt], { enabled: open, keep: true });
  useEffect(() => {
    if (open && diff !== null) onLoaded?.(file.path);
  }, [open, diff]); // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <div className={`review-file${file.fresh ? " fresh" : ""}${open ? " open" : ""}`}>
      <div className="review-file-row">
        <button type="button" className="review-file-head" onClick={onToggle} aria-expanded={open}>
          <span className={`chevron${open ? " open" : ""}`}>
            <Icon name="chevron-right" size={12} />
          </span>
          {file.fresh && <span className="review-dot" title="Changed since you looked" />}
          <span className="review-path">
            <span className="review-dir">{dirname(file.path)}</span>
            {basename(file.path)}
          </span>
          {draft.length > 0 && (
            <span className="review-tag review-tag-comments" title={`${plural(draft.length, "comment")} in the draft`}>
              {draft.length}
            </span>
          )}
          {file.untracked ? (
            <span className="review-tag">new</span>
          ) : (
            <span className="review-counts">
              +{file.added} −{file.removed}
            </span>
          )}
        </button>
        <Tip text="Open the file in the viewer">
          <button type="button" className="icon-btn sm" aria-label="Open the file" onClick={() => onOpen()}>
            <Icon name="external-link" size={12} />
          </button>
        </Tip>
        {onCode && (
          <Tip text="Open the file in Code (VS Code on this machine)">
            <button type="button" className="icon-btn sm" aria-label="Open in Code" onClick={onCode}>
              <Icon name="code" size={12} />
            </button>
          </Tip>
        )}
      </div>
      {open &&
        (error ? (
          <Notice
            tone="error"
            action={
              <button type="button" className="linkish" onClick={() => setAttempt((n) => n + 1)}>
                Try again
              </button>
            }
          >
            {error}
          </Notice>
        ) : diff === null ? (
          <div className="empty">Loading…</div>
        ) : (
          <Diff text={diff} split={split} canComment={canComment} draft={draft} file={file.path} onComment={onComment} onRemove={onRemove} />
        ))}
    </div>
  );
}

function Diff({ text, split, canComment, draft, file, onComment, onRemove }: {
  text: string;
  split: boolean;
  canComment: boolean;
  draft: DraftComment[];
  file: string;
  onComment: (file: string, comment: NewComment) => void;
  onRemove: (id: number) => void;
}) {
  const hunks = useMemo(() => splitHunks(text), [text]);
  const rowKeys = useMemo(() => {
    const keys = new Set<string>();
    for (const hunk of hunks) for (const row of rowsOf(hunk)) keys.add(rowKey(row));
    return keys;
  }, [hunks]);
  const strayComments = draft.filter((comment) => !rowKeys.has(commentKey(comment)));
  if (hunks.length === 0 && strayComments.length === 0) return <div className="empty">No textual diff (binary, or identical).</div>;
  return (
    <div className={`fv-code fv-diff review-diff${split ? " columns" : ""}${canComment ? " can-comment" : ""}`}>
      {hunks.map((hunk, index) => (
        <Hunk key={index} hunk={hunk} split={split} canComment={canComment} draft={draft} file={file} onComment={onComment} onRemove={onRemove} />
      ))}
      {strayComments.length > 0 && (
        <div className="review-orphans">
          <Notice tone="hint">No longer on a line of this diff — still in the draft:</Notice>
          {strayComments.map((comment) => (
            <Note key={comment.id} comment={comment} where={`line ${comment.line}${comment.removed ? " (removed)" : ""}`} onRemove={onRemove} />
          ))}
        </div>
      )}
    </div>
  );
}
