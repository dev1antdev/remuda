// One file of the review: its row, and its diff by hunks, in one column or two.

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@/lib/query";
import { basename, dirname, errMsg, plural } from "@/lib/text";
import { Icon, Notice, Tip } from "@/ui";
import type { ReviewFile } from "./api";
import { fileLines, surround, whyNotUnfold, type Unfold } from "./context";
import { draftOnRow, type DraftComment } from "./draft";
import { Hunk, Note, type NewComment } from "./Hunk";
import { rowsOf, splitHunks, type DiffRow } from "./hunks";
import { offRows, whereWords, type OpinionComment } from "./opinion";
import { OpinionNote } from "./OpinionNote";
import type { SecretFinding } from "./secrets";
import { t, tn } from "@/lib/i18n";

export type { NewComment } from "./Hunk";

// SourceRead is the working-tree file as the viewer serves it: its text, and whether the cap cut it
export type SourceRead = { text: string; truncated: boolean; max: number };

// Opinions is a second opinion's open comments on one file, who gave them, and what a comment's buttons do
export type Opinions = {
  comments: OpinionComment[];
  reviewer: string;
  onAdopt: (comment: OpinionComment, row?: DiffRow) => void;
  onDismiss: (id: number) => void;
};

export function FileRow({ file, open, split, onToggle, load, loadSource, onLoaded, canComment, draft, onComment, onRemove, onOpen, onCode, opinion, reviewer, onAdopt, onDismissOpinion, secrets }: {
  file: ReviewFile;
  open: boolean;
  split: boolean;
  onToggle: () => void;
  load: (path: string) => Promise<string>;
  loadSource: (path: string) => Promise<SourceRead>; // the file itself, for the lines around a hunk
  onLoaded?: (path: string) => void; // the diff is on screen
  canComment: boolean;
  draft: DraftComment[];
  onComment: (file: string, comment: NewComment) => void;
  onRemove: (id: number) => void;
  onOpen: (line?: number) => void;
  onCode?: () => void; // the file in the editor, when Code mode is there to take it
  opinion: OpinionComment[]; // the second opinion's open comments on this file
  reviewer: string;
  onAdopt: (comment: OpinionComment, row?: DiffRow) => void;
  onDismissOpinion: (id: number) => void;
  secrets: SecretFinding[]; // what the secret check found on this file's lines, their rows marked
}) {
  const opinions = useMemo<Opinions>(() => ({ comments: opinion, reviewer, onAdopt, onDismiss: onDismissOpinion }), [opinion, reviewer, onAdopt, onDismissOpinion]);
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
          {file.fresh && <span className="review-dot" title={t("Changed since you looked")} />}
          <span className="review-path">
            <span className="review-dir">{dirname(file.path)}</span>
            {basename(file.path)}
          </span>
          {draft.length > 0 && (
            <span className="review-tag review-tag-comments" title={t("{comments} in the draft", { comments: plural(draft.length, "comment") })}>
              {draft.length}
            </span>
          )}
          {secrets.length > 0 && (
            <span className="review-tag review-tag-secret" title={tn(secrets.length, "{n} line looks like it holds a secret", "{n} lines look like they hold secrets")}>
              <Icon name="alert-triangle" size={12} /> {secrets.length}
            </span>
          )}
          {opinion.length > 0 && (
            <span className="review-tag review-tag-opinion" title={t("{comments} from {agent}'s second opinion", { comments: plural(opinion.length, "comment"), agent: reviewer })}>
              <Icon name="message" size={12} /> {opinion.length}
            </span>
          )}
          {file.untracked ? (
            <span className="review-tag">{t("new")}</span>
          ) : (
            <span className="review-counts">
              +{file.added} −{file.removed}
            </span>
          )}
        </button>
        <Tip text={t("Open the file in the viewer")}>
          <button type="button" className="icon-btn sm" aria-label={t("Open the file")} onClick={() => onOpen()}>
            <Icon name="external-link" size={12} />
          </button>
        </Tip>
        {onCode && (
          <Tip text={t("Open the file in Code (VS Code on this machine)")}>
            <button type="button" className="icon-btn sm" aria-label={t("Open in Code")} onClick={onCode}>
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
                {t("Try again")}
              </button>
            }
          >
            {error}
          </Notice>
        ) : diff === null ? (
          <div className="empty">{t("Loading…")}</div>
        ) : (
          <Diff text={diff} split={split} canComment={canComment} draft={draft} file={file.path} loadSource={loadSource} onComment={onComment} onRemove={onRemove} opinions={opinions} secrets={secrets} />
        ))}
    </div>
  );
}

// Unfolds is what the hunks of one diff have unfolded, with the file's lines that feed them: all of it for this very
// diff text, so a diff re-read after an edit starts folded again against the file as it is now
type Unfolds = { forText: string; by: Record<number, Unfold>; lines: string[] | null; blocked: string | null };
const folded = (text: string): Unfolds => ({ forText: text, by: {}, lines: null, blocked: null });
const FOLDED = folded("");

function Diff({ text, split, canComment, draft, file, loadSource, onComment, onRemove, opinions, secrets }: {
  text: string;
  split: boolean;
  canComment: boolean;
  draft: DraftComment[];
  file: string;
  loadSource: (path: string) => Promise<SourceRead>;
  onComment: (file: string, comment: NewComment) => void;
  onRemove: (id: number) => void;
  opinions: Opinions;
  secrets: SecretFinding[];
}) {
  const hunks = useMemo(() => splitHunks(text), [text]);
  const [unfoldState, setUnfolds] = useState<Unfolds>(() => folded(text));
  const unfolds = unfoldState.forText === text ? unfoldState : FOLDED;
  const reading = useRef<{ forText: string; promise: Promise<string[] | null> } | null>(null);
  // the file is read once per diff text, at the first unfold; a cut file or a missing one blocks every unfold with the reason
  const readLines = useCallback((): Promise<string[] | null> => {
    if (reading.current?.forText !== text) {
      reading.current = {
        forText: text,
        promise: loadSource(file)
          .then((read) => {
            const blocked = whyNotUnfold(read);
            const lines = blocked ? null : fileLines(read.text);
            setUnfolds((state) => ({ ...(state.forText === text ? state : folded(text)), lines, blocked }));
            return lines;
          })
          .catch((failure: unknown) => {
            setUnfolds((state) => ({ ...(state.forText === text ? state : folded(text)), lines: null, blocked: errMsg(failure) }));
            return null;
          }),
      };
    }
    return reading.current.promise;
  }, [text, file, loadSource]);
  const onUnfold = useCallback(
    (index: number, side: "above" | "below", count: number) => {
      void readLines().then((lines) => {
        if (!lines) return;
        setUnfolds((state) => {
          const base = state.forText === text ? state : folded(text);
          const before = base.by[index] ?? { above: 0, below: 0 };
          return { ...base, by: { ...base.by, [index]: { ...before, [side]: before[side] + count } } };
        });
      });
    },
    [readLines, text],
  );
  const shownRows = useMemo(() => {
    const rows: DiffRow[] = [];
    hunks.forEach((hunk, index) => {
      const around = surround(hunk, hunks[index - 1] ?? null, hunks[index + 1] ?? null, unfolds.by[index], unfolds.lines);
      rows.push(...around.above, ...rowsOf(hunk), ...around.below);
    });
    return rows;
  }, [hunks, unfolds]);
  const strayComments = draft.filter((comment) => !shownRows.some((row) => draftOnRow(comment, row)));
  const strayOpinions = useMemo(() => offRows(opinions.comments, shownRows), [opinions.comments, shownRows]);
  if (hunks.length === 0 && strayComments.length === 0 && strayOpinions.length === 0) return <div className="empty">{t("No textual diff (binary, or identical).")}</div>;
  return (
    <div className={`fv-code fv-diff review-diff${split ? " columns" : ""}${canComment ? " can-comment" : ""}`}>
      {hunks.map((hunk, index) => (
        <Hunk
          key={index}
          hunk={hunk}
          prev={hunks[index - 1] ?? null}
          next={hunks[index + 1] ?? null}
          index={index}
          split={split}
          canComment={canComment}
          draft={draft}
          file={file}
          onComment={onComment}
          onRemove={onRemove}
          unfold={unfolds.by[index]}
          lines={unfolds.by[index] || index === hunks.length - 1 ? unfolds.lines : null}
          blocked={unfolds.blocked}
          onUnfold={onUnfold}
          opinions={opinions}
          secrets={secrets}
        />
      ))}
      {strayOpinions.length > 0 && (
        <div className="review-orphans">
          <Notice tone="hint">{t("{agent}'s comments on lines this diff does not show:", { agent: opinions.reviewer })}</Notice>
          {strayOpinions.map((comment) => (
            <OpinionNote key={comment.id} comment={comment} reviewer={opinions.reviewer} where={whereWords(comment)} onAdopt={opinions.onAdopt} onDismiss={opinions.onDismiss} />
          ))}
        </div>
      )}
      {strayComments.length > 0 && (
        <div className="review-orphans">
          <Notice tone="hint">{t("No longer on a line of this diff — still in the draft:")}</Notice>
          {strayComments.map((comment) => (
            <Note key={comment.id} comment={comment} where={(comment.line > 0 ? t("line {n}", { n: comment.line }) : t("the whole file")) + (comment.removed ? t(" (removed)") : "")} onRemove={onRemove} />
          ))}
        </div>
      )}
    </div>
  );
}
