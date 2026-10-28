// A task's comments: the agent's answers, the person's notes, remuda's own; and a box for a new one.

import { useState } from "react";
import { actions } from "@/app/store";
import type { FileTarget } from "@/features/files/target";
import { errMsg } from "@/lib/text";
import { whenShort } from "@/lib/time";
import { Icon, Logo } from "@/ui";
import { Markdown } from "@/ui/Markdown";
import { addComment, deleteComment, type Run, type Task } from "./api";
import { t as tr } from "@/lib/i18n";

export function CommentsField({ task: t, run, onChange, onError, onMarkDone }: {
  task: Task;
  run: Run | null;
  onChange: (t: Task) => void;
  onError: (msg: string) => void;
  onMarkDone: () => void;
}) {
  const [comment, setComment] = useState("");
  const [posting, setPosting] = useState(false);
  const comments = t.comments ?? [];
  // a path in a comment opens where the run worked: its machine, its pane while it lives, its worktree
  const openFile = (target: FileTarget) => actions.openFile(run?.machine ?? "local", run?.pane ?? "", target, run?.dir ?? "");
  const post = async () => {
    const body = comment.trim();
    if (!body || posting) return;
    setPosting(true);
    try {
      const c = await addComment(t.id, body);
      onChange({ ...t, comments: [...comments, c] });
      setComment("");
    } catch (e) {
      onError(errMsg(e));
    } finally {
      setPosting(false);
    }
  };
  return (
    <div className="tasks-comments">
      <div className="tasks-field-name">{tr("Comments")}</div>
      {comments.map((c) => (
        <div key={c.id} className={`tasks-comment by-${c.author}`}>
          <div className="tasks-comment-head">
            <span className="tasks-comment-who">
              {c.author === "agent" ? (
                <>
                  <Logo agent={run?.agent ?? "claude"} /> {run?.agent ?? "agent"}
                </>
              ) : c.author === "you" ? (
                <>
                  <Icon name="user" size={12} /> you
                </>
              ) : (
                <>
                  <Icon name="info" size={12} /> remuda
                </>
              )}
            </span>
            <span className="tasks-comment-when" title={new Date(c.created).toLocaleString()}>
              {whenShort(c.created)}
            </span>
            <button
              type="button"
              className="icon-btn sm"
              title={tr("Delete comment")}
              aria-label={tr("Delete comment")}
              onClick={() =>
                void deleteComment(c.id).then(
                  () => onChange({ ...t, comments: comments.filter((x) => x.id !== c.id) }),
                  (e) => onError(errMsg(e)),
                )
              }
            >
              <Icon name="x" size={12} />
            </button>
          </div>
          <div className="tasks-comment-body">
            <Markdown text={c.body} onOpenFile={openFile} />
          </div>
        </div>
      ))}
      {run?.state === "finished" && !t.done && (
        <button type="button" className="btn primary tasks-comment-done" onClick={onMarkDone}>
          <Icon name="check" size={14} /> {tr("Mark done")}
        </button>
      )}
      <div className="tasks-comment-add">
        <textarea
          rows={2}
          value={comment}
          placeholder={tr("A note on this task…")}
          onChange={(e) => setComment(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) void post();
          }}
        />
        <button type="button" className="btn" disabled={posting || !comment.trim()} onClick={() => void post()}>
          {tr("Comment")}
        </button>
      </div>
    </div>
  );
}
