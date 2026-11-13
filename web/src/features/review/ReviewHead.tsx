// The Changes lens's head: what it compares — the uncommitted work, or the branch's whole work against the branch it was
// cut from, as gitview names it — the branch, what changed since the last look, the keys, the view's switches and the
// buttons that act on the whole folder, and under them what Claude Code's settings files there want looked at.

import { useState } from "react";
import { plural } from "@/lib/text";
import { Icon, Notice, Select, Tip } from "@/ui";
import type { BaseView, Review } from "./api";
import { KEYS } from "./keys";
import { baseChoices, baseWords, pullWords, type Scope } from "./scope";
import { settingsNotes } from "./secrets";
import { t } from "@/lib/i18n";

export function ReviewHead({ review, scope, onScope, base, pinned, onPin, lastLookText, opinionWords, split, onSplit, tree, onTree, onLookAgain, opinionRunning, onAskOpinion, onUndo, marking, onMark }: {
  review: Review | null;
  scope: Scope;
  onScope: (scope: Scope) => void;
  base: BaseView | null; // the base scope's read, null while it comes
  pinned: string; // the base the person pinned, "" for the one found
  onPin: (base: string) => void;
  lastLookText: string;
  opinionWords: string; // the second opinion's line, "" without one
  split: boolean;
  onSplit: () => void;
  tree: boolean;
  onTree: () => void;
  onLookAgain: () => void;
  opinionRunning: boolean;
  onAskOpinion: () => void;
  onUndo?: () => void; // an agent's turn here taken back; none without a person's agent to undo
  marking: boolean;
  onMark: () => void;
}) {
  const files = review?.files ?? [];
  const commits = review?.commits ?? [];
  const added = files.reduce((sum, file) => sum + file.added, 0);
  const removed = files.reduce((sum, file) => sum + file.removed, 0);
  const nothing = !files.length && !commits.length;
  const notes = settingsNotes(review?.settings);
  const shown = notes.map((note) => note.text).join("\n");
  const [dismissed, setDismissed] = useState(""); // what the person set aside: the notes come back once they say something else
  const toPull = pullWords(review?.outgoing);
  return (
    <div className="review-head">
      <div className="review-title">
        <span className="review-title-line">
          <span className="review-scope" role="tablist" aria-label={t("What the lens compares")}>
            <button type="button" role="tab" aria-selected={scope === "uncommitted"} className={scope === "uncommitted" ? "on" : ""} onClick={() => onScope("uncommitted")}>
              {t("Uncommitted")}
            </button>
            <button type="button" role="tab" aria-selected={scope === "base"} className={scope === "base" ? "on" : ""} onClick={() => onScope("base")}>
              {t("vs base")}
            </button>
          </span>
          <span className="review-branch">
            <Icon name="git-branch" size={12} /> {review?.branch || t("detached")}
          </span>
        </span>
        {scope === "uncommitted" ? (
          <span className="review-sub">
            {lastLookText}
            {commits.length > 0 && ` · ${review?.commits_total ? t("the newest {n} of {total} commits", { n: commits.length, total: review.commits_total }) : plural(commits.length, "commit")}`}
            {files.length > 0 && ` · ${t("{files} uncommitted +{added} −{removed}", { files: plural(files.length, "file"), added, removed })}`}
            {toPull && ` · ${toPull}`}
            {opinionWords && ` · ${opinionWords}`}
          </span>
        ) : (
          <span className="review-sub review-base-line">
            <span>{baseWords(base)}</span>
            {base && base.refs.length > 0 && <Select className="sm review-base-pick" value={pinned} options={baseChoices(base)} onChange={onPin} />}
          </span>
        )}
      </div>
      <span className="review-keys" aria-hidden="true">
        {KEYS.map(([first, second, what]) => (
          <span key={what}>
            <kbd>{first}</kbd>
            {second && <kbd>{second}</kbd>} {t(what)}
          </span>
        ))}
      </span>
      <Tip text={tree ? t("Files as a tree of folders") : t("Files as a list")}>
        <button type="button" className={`icon-btn review-tree-toggle${tree ? " on" : ""}`} aria-pressed={tree} aria-label={t("A list or a tree")} onClick={onTree}>
          <Icon name="list-tree" size={14} />
        </button>
      </Tip>
      <Tip text={split ? t("Two columns, the old file on the left (s)") : t("One column (s)")}>
        <button type="button" className={`icon-btn review-columns${split ? " on" : ""}`} aria-pressed={split} aria-label={t("One column or two")} onClick={onSplit}>
          <Icon name="columns" size={14} />
        </button>
      </Tip>
      <Tip text={t("Look again")}>
        <button type="button" className="icon-btn review-look-again" aria-label={t("Look again")} onClick={onLookAgain}>
          <Icon name="refresh-cw" size={14} />
        </button>
      </Tip>
      {onUndo && (
        <Tip text={t("Put the files back as they were before an agent's turn: the last one, or one you pick")}>
          <button type="button" className="icon-btn review-undo" aria-label={t("Undo a turn…")} onClick={onUndo}>
            <Icon name="undo" size={14} />
          </button>
        </Tip>
      )}
      <button
        type="button"
        className="btn review-ask-opinion"
        disabled={nothing || opinionRunning}
        onClick={onAskOpinion}
        title={opinionRunning ? t("A second opinion is on its way: wait for its answer, or close its tab") : t("Another agent reviews this without changing anything; its comments appear under the lines")}
      >
        <Icon name="message" size={14} /> {t("Second opinion")}
      </button>
      <button type="button" className="btn review-seen" disabled={marking || nothing} onClick={onMark} title={t("The counts clear and the next look starts from now; the agent hears nothing")}>
        <Icon name="check" size={14} /> {t("Mark reviewed")}
      </button>
      {notes.length > 0 && dismissed !== shown && (
        <div className="review-settings">
          {notes.map((note) => (
            <Notice key={note.path} tone={note.tone} icon="alert-triangle" className="review-settings-note" onDismiss={() => setDismissed(shown)}>
              {note.text} <span className="review-settings-fix">{note.fix}</span>
            </Notice>
          ))}
        </div>
      )}
    </div>
  );
}
