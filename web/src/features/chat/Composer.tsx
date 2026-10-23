// The composer owns the draft, so typing never re-renders the transcript: attachments uploaded at once,
// quick prompts, dictation, Enter to send (Shift+Enter a new line; on a phone the button sends), ↑ in an empty
// line to bring back what was sent before. The compact one, in a wall tile, is the line and its buttons alone.

import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { sendKeys, submitPrompt } from "@/features/roster/api";
import { isTouch } from "@/lib/prefs";
import { errMsg } from "@/lib/text";
import { filesFrom } from "@/lib/transfer";
import { Icon, Notice } from "@/ui";
import { MAX_ATTACHMENTS, useAttachments, type Attachment } from "./attachments";
import { useDictation } from "./dictation";
import { QuickPromptsDialog, QuickPromptsMenu } from "./QuickPrompts";
import { useHistory } from "./useHistory";

export default function Composer({ machineId, paneId, working, dir, compact = false }: { machineId: string; paneId: string; working: boolean; dir: string; compact?: boolean }) {
  // the insertions read the draft through a ref: a state updater must stay free of side effects
  const [draft, setDraftState] = useState("");
  const draftRef = useRef("");
  const setDraft = (next: string | ((d: string) => string)) => {
    const v = typeof next === "function" ? next(draftRef.current) : next;
    draftRef.current = v;
    setDraftState(v);
  };
  const [quick, setQuick] = useState<{ x: number; y: number } | null>(null);
  const [manage, setManage] = useState(false);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [dragOver, setDragOver] = useState(false);
  const area = useRef<HTMLTextAreaElement>(null);
  const picker = useRef<HTMLInputElement>(null);
  const att = useAttachments(machineId, paneId);
  const history = useHistory(machineId, dir);

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

  const send = async () => {
    const text = draft.trim();
    if (!text || sending || att.uploading) return;
    setSending(true);
    try {
      await submitPrompt(machineId, paneId, text);
      history.add(text);
      setDraft("");
      att.clear();
      setError(null);
    } catch (e) {
      setError(errMsg(e));
    } finally {
      setSending(false);
    }
  };

  useEffect(() => {
    if (!quick) return;
    const close = () => setQuick(null);
    document.addEventListener("click", close);
    return () => document.removeEventListener("click", close);
  }, [quick]);

  return (
    <>
      {error && (
        <Notice tone="error" className="chat-notice" onDismiss={() => setError(null)}>
          {error}
        </Notice>
      )}
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
              <button type="button" className="icon-btn sm danger attachment-x" title="Remove" aria-label="Remove" onClick={() => removeAttachment(a)}>
                <Icon name="x" size={12} />
              </button>
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
          const files = filesFrom(e.dataTransfer);
          if (files.length === 0) return;
          e.preventDefault();
          setDragOver(false);
          void attach(files);
        }}
      >
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
            <button
              type="button"
              className="icon-btn lg composer-attach"
              title={`Attach files (up to ${MAX_ATTACHMENTS}) — they are uploaded into the pane's folder`}
              aria-label="Attach files"
              disabled={att.items.length >= MAX_ATTACHMENTS}
              onClick={() => picker.current?.click()}
            >
              <Icon name="paperclip" size={16} />
            </button>
            <button
              type="button"
              className={`icon-btn lg composer-attach${quick ? " on" : ""}`}
              title="Quick prompts"
              aria-label="Quick prompts"
              aria-expanded={!!quick}
              onClick={(e) => {
                e.stopPropagation();
                const r = e.currentTarget.getBoundingClientRect();
                setQuick(quick ? null : { x: r.left, y: r.top - 6 });
              }}
            >
              <Icon name="zap" size={16} />
            </button>
            {dictation.supported && (
              <button
                type="button"
                className={`icon-btn lg composer-attach composer-mic${dictation.listening ? " listening" : ""}`}
                title={dictation.listening ? "Stop dictating" : "Dictate (on this device's speech recognition)"}
                aria-label={dictation.listening ? "Stop dictating" : "Dictate"}
                aria-pressed={dictation.listening}
                onClick={dictation.toggle}
              >
                <Icon name="mic" size={16} />
              </button>
            )}
          </>
        )}
        {quick && (
          <QuickPromptsMenu
            at={quick}
            dir={dir}
            onPick={(text) => {
              insertAt(text);
              setQuick(null);
              area.current?.focus();
            }}
            onManage={() => {
              setQuick(null);
              setManage(true);
            }}
          />
        )}
        {manage && <QuickPromptsDialog dir={dir} onClose={() => setManage(false)} />}
        <textarea
          ref={area}
          value={draft}
          placeholder={compact ? "Message…" : isTouch ? "Message the agent… (Enter = new line)" : "Message the agent… (Shift+Enter = new line)"}
          rows={Math.min(compact ? 3 : 6, Math.max(1, draft.split("\n").length))}
          onChange={(e) => {
            history.reset();
            setDraft(e.target.value);
          }}
          onBlur={(e) => (caret.current = e.target.selectionStart)}
          onSelect={(e) => (caret.current = e.currentTarget.selectionStart)}
          onPaste={(e) => {
            const files = filesFrom(e.clipboardData);
            if (files.length === 0) return;
            e.preventDefault();
            void attach(files);
          }}
          onKeyDown={(e) => {
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
        <button type="button" className="btn primary composer-send" disabled={sending || att.uploading || !draft.trim()} onClick={() => void send()} title={isTouch ? "Send" : "Send (Enter)"}>
          {sending ? (
            "…"
          ) : att.uploading ? (
            "Uploading…"
          ) : (
            <>
              <span>Send</span>
              <Icon name="send" size={14} />
            </>
          )}
        </button>
        {working && (
          <button type="button" className="btn danger soft composer-stop" title="Send Esc to interrupt" onClick={() => void sendKeys(machineId, paneId, ["esc"]).catch((e) => setError(errMsg(e)))}>
            <Icon name="square" size={12} /> <span>Stop</span>
          </button>
        )}
      </div>
    </>
  );
}
