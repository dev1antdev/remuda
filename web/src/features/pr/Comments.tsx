// The Comments tab: everything said on the pull request by thread, a thread's code drawn as diff rows, each comment's
// author and age, "New" until this device has seen it as it reads now; Reply, Resolve / Unresolve and Send to agent on
// each, a box for a new comment after the last, and the unresolved threads handed to an agent at once.

import { useMemo, useState } from "react";
import { actions, type PRAt } from "@/app/store";
import SendToAgent from "@/features/agents/SendToAgent";
import { useAgentTargets } from "@/features/agents/useAgentTargets";
import { submitPrompt } from "@/features/roster/api";
import { useRoster } from "@/features/roster/useRoster";
import { copyText } from "@/lib/host";
import { usePref } from "@/lib/prefs";
import { errMsg } from "@/lib/text";
import { ago } from "@/lib/time";
import { t, tn } from "@/lib/i18n";
import { Icon, Notice, Select, Tip } from "@/ui";
import { Markdown } from "@/ui/Markdown";
import { postComment, postReply, setResolved, type Detail } from "./api";
import { authorsOf, commentsOf, entries, hunkEnd, markSeen, unseen, whereWords, type Entry } from "./conversation";
import { commentMessage, unresolvedMessage } from "./messages";
import { unresolved } from "./model";
import { agentsOn } from "./tree";
import Patch from "./Patch";

type Handoff = { title: string; text: string };

export default function Comments({ at, pr, onlyOpen, onOnlyOpen, onChanged }: { at: PRAt; pr: Detail; onlyOpen: boolean; onOnlyOpen: (on: boolean) => void; onChanged: () => void }) {
  const [author, setAuthor] = useState("");
  const [seen, setSeen] = usePref<string[]>("pr.seen", []);
  const fresh = useMemo(() => unseen(pr, seen), [pr, seen]);
  const shown = entries(pr, { author, open: onlyOpen });
  const open = unresolved(pr);
  const [handoff, setHandoff] = useState<Handoff | null>(null);
  const targets = useAgentTargets();
  const { machines } = useRoster();
  const onPR = agentsOn(machines, at.machineId, { url: pr.url, head: pr.head });
  const first = onPR[0] ? { machineId: onPR[0].machineId, paneId: onPR[0].paneId } : at.paneId ? { machineId: at.machineId, paneId: at.paneId } : null;
  return (
    <>
      <div className="pr-comment-filters">
        <Select value={author} options={[{ value: "", label: t("Every author") }, ...authorsOf(pr).map((a) => ({ value: a, label: a }))]} onChange={setAuthor} />
        <button type="button" className={`btn sm${onlyOpen ? " on" : ""}`} aria-pressed={onlyOpen} onClick={() => onOnlyOpen(!onlyOpen)}>
          {t("Unresolved")}
        </button>
        {fresh.size > 0 && (
          <button type="button" className="linkish" onClick={() => setSeen(markSeen(pr, seen))}>
            {tn(fresh.size, "Mark {n} read", "Mark {n} read")}
          </button>
        )}
        <span className="pr-filler" />
        {open.length > 0 && (
          <button type="button" className="btn sm" onClick={() => setHandoff({ title: tn(open.length, "Send {n} unresolved thread to an agent", "Send {n} unresolved threads to an agent"), text: unresolvedMessage(pr, pr.threads) })}>
            <Icon name="send" size={12} /> {tn(open.length, "Send {n} to agent", "Send {n} to agent")}
          </button>
        )}
      </div>
      {pr.cut && <Notice tone="hint">{t("The conversation runs past what was read; the rest is on GitHub.")}</Notice>}
      {shown.length === 0 && <p className="pr-empty">{author || onlyOpen ? t("No comment matches these filters.") : t("No comments yet.")}</p>}
      {shown.map((e) => (
        <EntryBlock key={e.id} at={at} pr={pr} entry={e} fresh={fresh} author={author} onChanged={onChanged} onHand={(text) => setHandoff({ title: t("Send this comment to an agent"), text })} />
      ))}
      <NewComment at={at} pr={pr} onChanged={onChanged} />
      {handoff && (
        <SendToAgent
          title={handoff.title}
          text={handoff.text}
          agents={targets}
          currentPane={first}
          rememberKey={`pr:${pr.repository}`}
          onClose={() => setHandoff(null)}
          onSend={async (target, text) => {
            await submitPrompt(target.machineId, target.paneId, text);
            setHandoff(null);
            actions.notify(t("Sent to {agent}", { agent: target.sessionName }), { brief: true });
          }}
        />
      )}
    </>
  );
}

function EntryBlock({ at, pr, entry, fresh, author, onChanged, onHand }: { at: PRAt; pr: Detail; entry: Entry; fresh: Set<string>; author: string; onChanged: () => void; onHand: (text: string) => void }) {
  const thread = entry.thread;
  const [open, setOpen] = useState(!thread?.resolved);
  const [replying, setReplying] = useState(false);
  const [busy, setBusy] = useState(false);
  const comments = commentsOf(entry);
  const resolve = async () => {
    if (!thread) return;
    setBusy(true);
    try {
      await setResolved(at.machineId, thread.id, !thread.resolved);
      onChanged();
    } catch (e) {
      actions.notify(errMsg(e), { error: true });
    } finally {
      setBusy(false);
    }
  };
  return (
    <section className={`pr-entry${thread && !thread.resolved ? " is-open" : ""}`}>
      <button type="button" className="pr-entry-head" aria-expanded={open} onClick={() => setOpen(!open)}>
        <Icon name={open ? "chevron-down" : "chevron-right"} size={12} />
        <span className="pr-entry-kind">
          {entry.kind}
          {thread?.outdated ? ` · ${t("Outdated")}` : ""}
        </span>
        {thread && <code className="pr-entry-where">{whereWords(thread)}</code>}
        {!open && <span className="pr-entry-who">{comments[0]?.author}</span>}
      </button>
      {open && (
        <div className="pr-entry-body">
          {thread?.comments[0]?.hunk && <Patch patch={hunkEnd(thread.comments[0].hunk)} path={thread.path} />}
          {comments.map((c) => (
            <div key={c.id} className="pr-comment">
              <div className="pr-comment-head">
                <b className={c.author === author ? "is-picked" : ""}>{c.author}</b>
                {fresh.has(c.id) && <span className="pr-new">{t("New")}</span>}
                <span className="pr-when">{ago(c.at)}</span>
                {c.url.startsWith("https://") && (
                  <Tip text={t("Copy the link")}>
                    <button type="button" className="icon-btn sm" aria-label={t("Copy the link")} onClick={() => void copyText(c.url).then(() => actions.notify(t("Link copied"), { brief: true }))}>
                      <Icon name="link" size={12} />
                    </button>
                  </Tip>
                )}
              </div>
              <div className="pr-markdown">{c.body.trim() ? <Markdown text={c.body} onOpenFile={(file) => actions.openFile(at.machineId, at.paneId ?? "", file, at.dir)} /> : <p className="pr-empty">{t("No text.")}</p>}</div>
            </div>
          ))}
          {thread && thread.more_replies ? <p className="pr-empty">{tn(thread.more_replies, "{n} more reply on GitHub", "{n} more replies on GitHub")}</p> : null}
          <div className="pr-entry-actions">
            <button type="button" className="linkish" disabled={busy || (thread ? !thread.can_reply : false)} onClick={() => setReplying(true)}>
              {t("Reply")}
            </button>
            {thread?.can_resolve && (
              <button type="button" className="linkish" disabled={busy} onClick={() => void resolve()}>
                {thread.resolved ? t("Unresolve") : t("Resolve")}
              </button>
            )}
            <span className="pr-filler" />
            <button type="button" className="btn sm" onClick={() => onHand(commentMessage(pr, entry))}>
              <Icon name="send" size={12} /> {t("Send to agent")}
            </button>
          </div>
          {replying && <ReplyBox at={at} pr={pr} entry={entry} onDone={() => {
                setReplying(false);
                onChanged();
              }} onCancel={() => setReplying(false)} />}
        </div>
      )}
    </section>
  );
}

// ReplyBox answers in a thread, or, for a comment outside one, adds a comment to the conversation addressed to its author
function ReplyBox({ at, pr, entry, onDone, onCancel }: { at: PRAt; pr: Detail; entry: Entry; onDone: () => void; onCancel: () => void }) {
  const last = commentsOf(entry).at(-1);
  const [text, setText] = useState(entry.thread || !last ? "" : `@${last.author} `);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const send = async () => {
    setBusy(true);
    setError("");
    try {
      if (entry.thread) await postReply(at.machineId, entry.thread.id, text);
      else await postComment({ machine: at.machineId, repo: pr.repository, number: pr.number }, text);
      actions.notify(t("Saved to GitHub"), { brief: true });
      onDone();
    } catch (e) {
      setError(t("{error} If it may have gone through, read GitHub again before sending it once more.", { error: errMsg(e) }));
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="pr-compose">
      <textarea rows={3} autoFocus value={text} onChange={(e) => setText(e.target.value)} aria-label={t("Reply")} />
      {error && <Notice tone="error">{error}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn sm" disabled={busy} onClick={onCancel}>
          {t("Cancel")}
        </button>
        <button type="button" className="btn sm primary" disabled={busy || !text.trim()} onClick={() => void send()}>
          {busy ? t("Sending…") : t("Reply")}
        </button>
      </div>
    </div>
  );
}

// NewComment is always after the last entry, its draft kept on this device by pull request
function NewComment({ at, pr, onChanged }: { at: PRAt; pr: Detail; onChanged: () => void }) {
  const [draft, setDraft] = usePref<string>(`pr.draft:${pr.repository}#${pr.number}`, "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const send = async () => {
    setBusy(true);
    setError("");
    try {
      await postComment({ machine: at.machineId, repo: pr.repository, number: pr.number }, draft);
      setDraft("");
      actions.notify(t("Saved to GitHub"), { brief: true });
      onChanged();
    } catch (e) {
      setError(t("{error} If it may have gone through, read GitHub again before sending it once more.", { error: errMsg(e) }));
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="pr-compose pr-new-comment">
      <textarea rows={3} placeholder={t("Write a comment…")} value={draft} onChange={(e) => setDraft(e.target.value)} aria-label={t("New comment")} />
      {error && <Notice tone="error">{error}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn sm primary" disabled={busy || !draft.trim()} onClick={() => void send()}>
          {busy ? t("Sending…") : t("Comment")}
        </button>
      </div>
    </div>
  );
}
