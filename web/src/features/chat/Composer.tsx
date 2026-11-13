// The composer owns the draft, so typing never re-renders the transcript: attachments uploaded at once,
// quick prompts, dictation, Enter to send (Shift+Enter a new line; on a phone the button sends), ↑ in an empty
// line to bring back what was sent before; a quote of the answer put after what is typed; Alt+Enter keeps the text as a
// note in the agent's queue, sent only from the queue's own buttons. The compact one, in a wall tile, is the line and
// its buttons alone; the bare one, for an agent that reads no file and keeps its own mode (the concierge), takes no
// attachment and shows no permission mode.

import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { sendKeys, submitPrompt } from "@/features/roster/api";
import { isTouch, usePref } from "@/lib/prefs";
import { errMsg } from "@/lib/text";
import { filesFrom } from "@/lib/transfer";
import { Icon, Notice, Tip } from "@/ui";
import { MAX_ATTACHMENTS, useAttachments, type Attachment } from "./attachments";
import { useDictation } from "./dictation";
import { expandPrompt, QuickPromptsButton, QuickPromptsDialog } from "./QuickPrompts";
import { leftAfterSend } from "./history";
import { useHistory } from "./useHistory";
import { applyPick, listFor, stillDismissed, tokenAt, type Dismissed, type Item } from "./complete";
import { CompletionList, useCompletion } from "./Completion";
import { ModeChip, type ScreenMode } from "./ModeChip";
import { addNote, numbered, queueKey, removeNote, sentAway, type QueuedNote } from "./queue";
import { QueueList } from "./QueueList";
import { withQuote } from "./quote";
import { t } from "@/lib/i18n";

const NO_NOTES: QueuedNote[] = [];
const noteId = () => `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;

export default function Composer({ machineId, paneId, working, dir, agent = "", compact = false, bare = false, quote, fill, placeholder, screen }: {
  machineId: string;
  paneId: string;
  working: boolean;
  dir: string;
  agent?: string;
  compact?: boolean;
  bare?: boolean; // no attachments, no permission mode
  quote?: { text: string; n: number }; // a quote of the answer to put in the box; n moves with every new one
  fill?: { text: string; n: number }; // a text to put in the box in place of what is there, the caret after it; n moves with every new one
  placeholder?: string;
  screen?: ScreenMode; // the chat's read of the pane's screen, for the permission mode chip of a Claude Code pane
}) {
  // the insertions read the draft through a ref: a state updater must stay free of side effects
  const [draft, setDraftState] = useState("");
  const draftRef = useRef("");
  const setDraft = (next: string | ((d: string) => string)) => {
    const v = typeof next === "function" ? next(draftRef.current) : next;
    draftRef.current = v;
    setDraftState(v);
  };
  const [manage, setManage] = useState(false);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [dragOver, setDragOver] = useState(false);
  const area = useRef<HTMLTextAreaElement>(null);
  const picker = useRef<HTMLInputElement>(null);
  const att = useAttachments(machineId, paneId);
  const history = useHistory(machineId, dir);
  const [notes, setNotes] = usePref<QueuedNote[]>(queueKey(machineId, dir, paneId), NO_NOTES);

  // the completion: the token under the caret, its list from the machine, the row the keys are on, the token Esc closed
  // (forgotten once the caret has left it, so the next slash or @ at the same place gets its list again)
  const [caretPos, setCaretPos] = useState(0);
  const [active, setActive] = useState(0);
  const [dismissed, setDismissed] = useState<Dismissed | null>(null);
  const token = useMemo(() => tokenAt(draft, caretPos), [draft, caretPos]);
  const open = listFor(token, dismissed, history.browsing());
  useEffect(() => setDismissed((was) => stillDismissed(was, token)), [token]);
  const completion = useCompletion(machineId, dir, agent, open);
  const shown = completion.items;
  const at = Math.min(active, Math.max(0, shown.length - 1));
  useEffect(() => setActive(0), [open?.kind, open?.query]);

  const mention = (rel: string) => `@${rel}`;

  // the paperclip steals focus, so the caret is remembered on every blur
  const caret = useRef<number | null>(null);
  const dictation = useDictation((text) => {
    caret.current = null;
    setDraft((d) => (d ? d.replace(/\s*$/, " ") : "") + text);
  });

  // insertAt puts text at the remembered caret (or the end), padded with spaces, and moves the caret past it once the draft has rendered
  const moveCaret = useRef<number | null>(null);
  const insertAt = (text: string) => {
    const d = draftRef.current;
    const at = Math.min(caret.current ?? d.length, d.length);
    const before = d.slice(0, at);
    const after = d.slice(at);
    const pad = before && !/\s$/.test(before) ? " " : "";
    const tail = after && !/^\s/.test(after) ? " " : "";
    const end = before.length + pad.length + text.length + tail.length;
    caret.current = end;
    moveCaret.current = end;
    setDraft(before + pad + text + tail + after);
  };
  useLayoutEffect(() => {
    const at = moveCaret.current;
    if (at === null) return;
    moveCaret.current = null;
    area.current?.setSelectionRange(at, at);
  }, [draft]);

  // a quote of the answer goes after what is typed, the caret under it, the box focused to write the reply
  const quoteN = quote?.n;
  useEffect(() => {
    if (!quote?.text) return;
    const next = withQuote(draftRef.current, quote.text);
    caret.current = next.caret;
    moveCaret.current = next.caret;
    setDraft(next.draft);
    area.current?.focus();
  }, [quoteN]); // eslint-disable-line react-hooks/exhaustive-deps

  // a text offered elsewhere (a question the concierge suggests) goes into the box, focused, ready to edit or send
  const fillN = fill?.n;
  useEffect(() => {
    if (!fill?.text) return;
    caret.current = fill.text.length;
    moveCaret.current = fill.text.length;
    setDraft(fill.text);
    area.current?.focus();
  }, [fillN]); // eslint-disable-line react-hooks/exhaustive-deps

  // the queue: the draft kept as a note for later; a send through the chat's own path takes notes away only once it went
  const queueDraft = () => {
    const text = draftRef.current;
    if (!text.trim() || att.uploading) return; // a file still uploading would land its @mention in the next draft
    setNotes((list) => addNote(list, text, noteId(), Date.now()));
    setDraft("");
    att.clear();
    history.reset();
    area.current?.focus();
  };
  const sendNotes = async (list: QueuedNote[]) => {
    if (!list.length || sending) return;
    const text = numbered(list);
    setSending(true);
    try {
      await submitPrompt(machineId, paneId, text);
      history.add(text);
      setNotes((all) => sentAway(all, list));
      setError(null);
    } catch (e) {
      setError(errMsg(e));
    } finally {
      setSending(false);
    }
  };
  const editNote = (note: QueuedNote) => {
    setNotes((list) => removeNote(list, note.id));
    const before = draftRef.current.replace(/\s+$/, "");
    const next = before ? `${before}\n\n${note.text}` : note.text;
    caret.current = next.length;
    moveCaret.current = next.length;
    setDraft(next);
    area.current?.focus();
  };

  // pick puts the row's text in place of the token and leaves the caret after it, the field still focused
  const pick = (item: Item) => {
    if (!open) return;
    const next = applyPick(draftRef.current, open, item.insert);
    caret.current = next.caret;
    moveCaret.current = next.caret;
    setCaretPos(next.caret);
    setDraft(next.draft);
    setActive(0);
    area.current?.focus();
  };

  const attach = async (files: File[]) => {
    setError(null);
    const ups = await att.add(files);
    if (ups.length) insertAt(ups.map((u) => mention(u.rel)).join(" "));
  };

  const removeAttachment = (a: Attachment) => {
    if (a.upload) {
      const m = mention(a.upload.rel);
      setDraft((d) => d.replace(new RegExp(`\\s?${m.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}(?=\\s|$)`), ""));
    }
    att.remove(a.id);
  };

  // the box stays open while the send is out: what is typed or attached meanwhile stays for the next message
  const send = async () => {
    const sent = draftRef.current;
    const text = sent.trim();
    if (!text || sending || att.uploading) return;
    const went = att.items.map((a) => a.id);
    setSending(true);
    try {
      await submitPrompt(machineId, paneId, text);
      history.add(text);
      setDraft((d) => leftAfterSend(d, sent));
      went.forEach((id) => att.remove(id));
      setError(null);
    } catch (e) {
      setError(errMsg(e));
    } finally {
      setSending(false);
    }
  };

  return (
    <>
      {error && (
        <Notice tone="error" className="chat-notice" onDismiss={() => setError(null)}>
          {error}
        </Notice>
      )}
      {!compact && notes.length > 0 && <QueueList notes={notes} busy={sending} onSend={(list) => void sendNotes(list)} onEdit={editNote} onRemove={(note) => setNotes((list) => removeNote(list, note.id))} />}
      {att.items.length > 0 && (
        <div className="attachments">
          {att.items.map((a) => (
            <div key={a.id} className={`attachment${a.error ? " failed" : ""}${a.upload ? "" : " pending"}`} title={a.error ?? a.upload?.rel ?? a.file.name}>
              {a.thumb ? (
                <img src={a.thumb} alt="" />
              ) : (
                <span className="attachment-icon">
                  <Icon name="file" size={18} />
                </span>
              )}
              <span className="attachment-name">{a.file.name}</span>
              {!a.upload && !a.error && <span className="attachment-spin" />}
              <Tip text={t("Remove")}>
                <button type="button" className="icon-btn sm danger attachment-x" aria-label={t("Remove")} onClick={() => removeAttachment(a)}>
                  <Icon name="x" size={12} />
                </button>
              </Tip>
            </div>
          ))}
        </div>
      )}
      <div
        className={`composer${compact ? " compact" : ""}${dragOver ? " drag" : ""}`}
        onDragOver={(e) => {
          if (e.dataTransfer.types.includes("Files")) {
            e.preventDefault();
            setDragOver(true);
          }
        }}
        onDragLeave={() => setDragOver(false)}
        onDrop={(e) => {
          const files = bare ? [] : filesFrom(e.dataTransfer);
          if (files.length === 0) return;
          e.preventDefault();
          setDragOver(false);
          void attach(files);
        }}
      >
        {open && <CompletionList kind={open.kind} items={shown} active={at} cut={completion.cut} error={completion.error} onPick={pick} onHover={setActive} />}
        <input
          ref={picker}
          type="file"
          multiple
          hidden
          onChange={(e) => {
            const files = Array.from(e.target.files ?? []);
            e.target.value = "";
            void attach(files);
          }}
        />
        {!compact && (
          <>
            {agent === "claude" && !bare && screen && <ModeChip machineId={machineId} paneId={paneId} screen={screen} />}
            {!bare && (
              <Tip text={t("Attach files (up to {n}): they are uploaded into the pane's folder", { n: MAX_ATTACHMENTS })}>
                <button type="button" className="icon-btn lg composer-attach" aria-label={t("Attach files")} disabled={att.items.length >= MAX_ATTACHMENTS} onClick={() => picker.current?.click()}>
                  <Icon name="paperclip" size={16} />
                </button>
              </Tip>
            )}
            <QuickPromptsButton
              dir={dir}
              onPick={(text) =>
                void expandPrompt(text, machineId, dir).then((filled) => {
                  insertAt(filled);
                  area.current?.focus();
                })
              }
              onManage={() => setManage(true)}
            />
            <Tip text={t("Queue it as a note for the agent (Alt+↵): sent only when you press Send on it")}>
              <button type="button" className="icon-btn lg composer-attach composer-queue" aria-label={t("Add to the queue")} disabled={!draft.trim() || att.uploading} onClick={queueDraft}>
                <Icon name="inbox" size={16} />
              </button>
            </Tip>
            {dictation.supported && (
              <Tip text={dictation.listening ? t("Stop dictating") : t("Dictate through this device's own speech recognition")}>
                <button
                  type="button"
                  className={`icon-btn lg composer-attach composer-mic${dictation.listening ? " listening" : ""}`}
                  aria-label={dictation.listening ? t("Stop dictating") : t("Dictate")}
                  aria-pressed={dictation.listening}
                  onClick={dictation.toggle}
                >
                  <Icon name="mic" size={16} />
                </button>
              </Tip>
            )}
          </>
        )}
        {manage && <QuickPromptsDialog dir={dir} onClose={() => setManage(false)} />}
        <textarea
          ref={area}
          value={draft}
          placeholder={placeholder ?? (compact ? t("Message…") : isTouch ? t("Message the agent… (Enter = new line)") : t("Message the agent… (Shift+Enter = new line)"))}
          rows={Math.min(compact ? 3 : 6, Math.max(1, draft.split("\n").length))}
          onChange={(e) => {
            history.reset();
            setCaretPos(e.target.selectionStart);
            setDraft(e.target.value);
          }}
          onBlur={(e) => (caret.current = e.target.selectionStart)}
          onSelect={(e) => {
            caret.current = e.currentTarget.selectionStart;
            setCaretPos(e.currentTarget.selectionStart);
          }}
          onPaste={(e) => {
            const files = bare ? [] : filesFrom(e.clipboardData);
            if (files.length === 0) return;
            e.preventDefault();
            void attach(files);
          }}
          onKeyDown={(e) => {
            // the completion takes the keys while it is open: ↑ ↓ walk its rows, ↵ or Tab picks, Esc closes it for this token
            if (open && shown.length > 0) {
              if (e.key === "ArrowDown" || e.key === "ArrowUp") {
                e.preventDefault();
                setActive((i) => (i + (e.key === "ArrowDown" ? 1 : shown.length - 1)) % shown.length);
                return;
              }
              if ((e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) || e.key === "Tab") {
                e.preventDefault();
                pick(shown[at]!);
                return;
              }
              if (e.key === "Escape") {
                e.preventDefault();
                setDismissed({ kind: open.kind, start: open.start });
                return;
              }
            }
            // Alt+Enter keeps the text as a note in the queue, in the lens
            if (!compact && e.key === "Enter" && e.altKey && !e.shiftKey && !e.metaKey && !e.ctrlKey && !e.nativeEvent.isComposing) {
              e.preventDefault();
              queueDraft();
              return;
            }
            // phone keyboards have no Shift+Enter: there Enter breaks the line and the button sends
            if (isTouch) return;
            if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
              e.preventDefault();
              void send();
              return;
            }
            // ↑ on an empty line, or while walking, brings back what was sent before; ↓ walks forward and back to the line being written
            if ((e.key === "ArrowUp" || e.key === "ArrowDown") && !e.shiftKey && !e.altKey && !e.metaKey && !e.ctrlKey && (draft === "" || history.browsing())) {
              const text = history.walk(e.key === "ArrowUp" ? "up" : "down", draft);
              if (text === null) return;
              e.preventDefault();
              caret.current = text.length;
              moveCaret.current = text.length;
              setDraft(text);
            }
          }}
        />
        <button type="button" className="btn primary composer-send" disabled={sending || att.uploading || !draft.trim()} onClick={() => void send()} title={isTouch ? t("Send") : t("Send (Enter)")}>
          {sending ? (
            "…"
          ) : att.uploading ? (
            t("Uploading…")
          ) : (
            <>
              <span>{t("Send")}</span>
              <Icon name="send" size={14} />
            </>
          )}
        </button>
        {working && (
          <Tip text={t("Send Esc: the agent stops what it is doing")}>
            <button type="button" className="btn danger soft composer-stop" aria-label={t("Stop")} onClick={() => void sendKeys(machineId, paneId, ["esc"]).catch((e) => setError(errMsg(e)))}>
              <Icon name="square" size={12} /> <span>{t("Stop")}</span>
            </button>
          </Tip>
        )}
      </div>
    </>
  );
}
