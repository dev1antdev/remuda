import { memo, useEffect, useRef, useState } from "react";
import { fetchChat, sendKeys, submitPrompt, type ChatResponse, type Part, type Turn } from "./api";
import { filesFrom, MAX_ATTACHMENTS, useAttachments, type Attachment } from "./attachments";
import { INSERT_EVENT } from "./host";
import { Markdown, type OpenFile } from "./markdown";
import { errMsg, isTouch } from "./prefs";
import Select from "./Select";
import Icon from "./icons";

// Chat lens: the transcript comes from Claude Code's own session file (read
// server-side), sending types into the live pane. Polls with ETag — every
// 2s while the agent works or waits, every 10s when the pane is idle, and
// at once when the roster flips its status.
export default function Chat({
  machineId,
  paneId,
  status,
  onOpenFile,
}: {
  machineId: string;
  paneId: string;
  status: string;
  onOpenFile: OpenFile;
}) {
  const [chat, setChat] = useState<ChatResponse | null>(null);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const etagRef = useRef<string | null>(null);
  const logRef = useRef<HTMLDivElement>(null);
  const stickRef = useRef(true);
  const busy = status === "run" || status === "input";

  useEffect(() => {
    etagRef.current = null;
    setChat(null);
    let stop = false;
    let timer: ReturnType<typeof setTimeout> | null = null;

    const poll = async () => {
      if (stop) return;
      try {
        if (document.visibilityState === "visible") {
          const res = await fetchChat(machineId, paneId, sessionId, etagRef.current);
          if (res && !stop) {
            etagRef.current = res.etag;
            setChat(res.body);
          }
          setError(null);
        }
      } catch (e) {
        if (!stop) setError(errMsg(e));
      } finally {
        if (!stop) timer = setTimeout(poll, busy ? 2000 : 10000);
      }
    };
    void poll();
    return () => {
      stop = true;
      if (timer) clearTimeout(timer);
    };
  }, [machineId, paneId, sessionId, busy]);

  // Follow the bottom only while the reader is already there.
  useEffect(() => {
    const log = logRef.current;
    if (log && stickRef.current) log.scrollTop = log.scrollHeight;
  }, [chat]);

  const onScroll = () => {
    const log = logRef.current;
    if (!log) return;
    stickRef.current = log.scrollHeight - log.scrollTop - log.clientHeight < 60;
  };

  const turns = chat?.turns ?? [];
  const sessions = chat?.sessions ?? [];

  return (
    <div className="chat">
      {sessions.length > 1 && (
        <Select
          className="chat-session-pick"
          value={sessionId ?? chat?.session_id ?? ""}
          onChange={setSessionId}
          options={sessions.map((s) => ({ value: s.id, label: s.title || s.id.slice(0, 8), hint: new Date(s.mod_time).toLocaleString() }))}
        />
      )}
      <div className="chat-log" ref={logRef} onScroll={onScroll}>
        {turns.length === 0 && (
          <div className="chat-empty">
            {chat === null ? "Loading…" : "No transcript for this pane's folder yet."}
          </div>
        )}
        {turns.map((t, i) => (
          <TurnView key={i} turn={t} onOpenFile={onOpenFile} />
        ))}
      </div>
      {status === "input" && (
        <div className="prompt-card">
          <span>The agent is waiting for an answer — check the terminal, or:</span>
          <div className="prompt-card-keys">
            <button onClick={() => void sendKeys(machineId, paneId, ["up"])}>↑</button>
            <button onClick={() => void sendKeys(machineId, paneId, ["down"])}>↓</button>
            <button onClick={() => void sendKeys(machineId, paneId, ["enter"])}>Enter</button>
            <button onClick={() => void sendKeys(machineId, paneId, ["esc"])}>Esc</button>
          </div>
        </div>
      )}
      {error && <div className="chat-error">{error}</div>}
      <Composer machineId={machineId} paneId={paneId} working={status === "run"} />
    </div>
  );
}

// The composer owns the draft, so typing never re-renders the transcript.
// Attachments (paperclip, a pasted screenshot, a drop) upload into the pane's
// folder and put an @path mention at the caret; the agent reads the file
// from there once the message is sent.
function Composer({ machineId, paneId, working }: { machineId: string; paneId: string; working: boolean }) {
  const [draft, setDraft] = useState("");
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [dragOver, setDragOver] = useState(false);
  const area = useRef<HTMLTextAreaElement>(null);
  const picker = useRef<HTMLInputElement>(null);
  const att = useAttachments(machineId, paneId);

  const mention = (rel: string) => `@${rel}`;

  // the caret where the files were added: the paperclip steals focus, so it
  // is remembered on every blur
  const caret = useRef<number | null>(null);

  // insertAt puts text at the remembered caret (or the end), padded with
  // spaces, and moves the caret past it — one update for all mentions so
  // several files keep their order.
  const insertAt = (text: string) => {
    const el = area.current;
    setDraft((d) => {
      const at = Math.min(caret.current ?? d.length, d.length);
      const before = d.slice(0, at);
      const after = d.slice(at);
      const pad = before && !/\s$/.test(before) ? " " : "";
      const tail = after && !/^\s/.test(after) ? " " : "";
      const next = before + pad + text + tail + after;
      const end = before.length + pad.length + text.length + tail.length;
      caret.current = end;
      requestAnimationFrame(() => el?.setSelectionRange(end, end));
      return next;
    });
  };

  const attach = async (files: File[]) => {
    setError(null);
    const ups = await att.add(files);
    if (ups.length) insertAt(ups.map((u) => mention(u.rel)).join(" "));
  };

  // a host (the IDE plugin) pushing text into the composer
  useEffect(() => {
    const onInsert = (e: Event) => {
      insertAt((e as CustomEvent<string>).detail);
      area.current?.focus();
    };
    window.addEventListener(INSERT_EVENT, onInsert);
    return () => window.removeEventListener(INSERT_EVENT, onInsert);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

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
      setDraft("");
      att.clear();
      setError(null);
    } catch (e) {
      setError(errMsg(e));
    } finally {
      setSending(false);
    }
  };

  return (
    <>
      {error && <div className="chat-error">{error}</div>}
      {att.items.length > 0 && (
        <div className="attachments">
          {att.items.map((a) => (
            <div key={a.id} className={`attachment${a.error ? " failed" : ""}${a.upload ? "" : " pending"}`} title={a.error ?? a.upload?.rel ?? a.file.name}>
              {a.thumb ? <img src={a.thumb} alt="" /> : <span className="attachment-icon"><Icon name="file" size={18} /></span>}
              <span className="attachment-name">{a.file.name}</span>
              {!a.upload && !a.error && <span className="attachment-spin" />}
              <button type="button" className="attachment-x" title="Remove" onClick={() => removeAttachment(a)}>
                <Icon name="x" size={12} />
              </button>
            </div>
          ))}
        </div>
      )}
      <div
        className={`composer${dragOver ? " drag" : ""}`}
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
        <button
          type="button"
          className="composer-attach"
          title={`Attach files (up to ${MAX_ATTACHMENTS}) — they are uploaded into the pane's folder`}
          disabled={att.items.length >= MAX_ATTACHMENTS}
          onClick={() => picker.current?.click()}
        >
          <Icon name="paperclip" size={17} />
        </button>
        <textarea
          ref={area}
          value={draft}
          placeholder={isTouch ? "Message the agent… (Enter = new line)" : "Message the agent… (Shift+Enter = new line)"}
          rows={Math.min(6, Math.max(1, draft.split("\n").length))}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={(e) => (caret.current = e.target.selectionStart)}
          onSelect={(e) => (caret.current = e.currentTarget.selectionStart)}
          onPaste={(e) => {
            const files = filesFrom(e.clipboardData);
            if (files.length === 0) return;
            e.preventDefault(); // a pasted screenshot is a file, not text
            void attach(files);
          }}
          onKeyDown={(e) => {
            // Phone keyboards have no Shift+Enter: there Enter breaks the line
            // and the Send button sends, like any messaging app.
            if (isTouch) return;
            if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
              e.preventDefault();
              void send();
            }
          }}
        />
        <button className="composer-send" disabled={sending || att.uploading || !draft.trim()} onClick={() => void send()} title="Send (Enter)">
          {sending ? "…" : att.uploading ? "Uploading…" : <><span>Send</span><Icon name="send" size={14} /></>}
        </button>
        {working && (
          <button
            className="composer-stop"
            title="Send Esc to interrupt"
            onClick={() => void sendKeys(machineId, paneId, ["esc"])}
          >
            <Icon name="square" size={12} /> Stop
          </button>
        )}
      </div>
    </>
  );
}

const TurnView = memo(function TurnView({ turn, onOpenFile }: { turn: Turn; onOpenFile: OpenFile }) {
  const cls = turn.role === "user" ? "turn turn-user" : "turn turn-assistant";
  return (
    <div className={cls}>
      {turn.parts.map((p, i) => (
        <PartView key={i} part={p} onOpenFile={onOpenFile} />
      ))}
    </div>
  );
});

const EDIT_TOOLS = new Set(["Edit", "Write", "MultiEdit", "NotebookEdit"]);

// Folded renders its body only once opened: thinking and tool I/O can be
// kilobytes per turn, and most are never looked at.
function Folded({ className, summary, children }: { className: string; summary: React.ReactNode; children: React.ReactNode }) {
  const [open, setOpen] = useState(false);
  return (
    <details className={className} open={open} onToggle={(e) => setOpen((e.target as HTMLDetailsElement).open)}>
      <summary>{summary}</summary>
      {open && children}
    </details>
  );
}

function PartView({ part, onOpenFile }: { part: Part; onOpenFile: OpenFile }) {
  if (part.type === "text") {
    return <Markdown text={part.text ?? ""} onOpenFile={onOpenFile} />;
  }
  if (part.type === "thinking") {
    return (
      <Folded className="part-thinking" summary={<><Icon name="sparkles" size={12} className="icon part-tool-icon" /> Thinking</>}>
        <div className="part-text">{part.text}</div>
      </Folded>
    );
  }
  if (part.type === "tool") {
    const { name, hint, file } = toolLabel(part);
    return (
      <Folded
        className={part.is_error ? "part-tool part-tool-error" : "part-tool"}
        summary={
          <>
            <Icon name={part.is_error ? "alert-triangle" : "chevron-right"} size={12} className="icon part-tool-icon" />{" "}
            {name}
            {hint && " · "}
            {file ? (
              <button
                className="path-link"
                onClick={(e) => {
                  e.preventDefault();
                  onOpenFile({ path: file, prefer: EDIT_TOOLS.has(name) ? "changes" : "file" });
                }}
                title={EDIT_TOOLS.has(name) ? "Open the file with its changes" : "Open the file"}
              >
                {hint}
              </button>
            ) : (
              hint
            )}
          </>
        }
      >
        {part.input !== undefined && <pre className="tool-io">{pretty(part.input)}</pre>}
        {part.output && <pre className="tool-io tool-out">{part.output}</pre>}
      </Folded>
    );
  }
  return null;
}

function toolLabel(part: Part): { name: string; hint: string; file: string | null } {
  const name = part.tool_name ?? "tool";
  const input = part.input as Record<string, unknown> | undefined | string;
  if (input && typeof input === "object") {
    const obj = input as Record<string, unknown>;
    const file = obj["file_path"] ?? obj["notebook_path"] ?? obj["path"];
    if (typeof file === "string" && file) {
      return { name, hint: file.length > 70 ? "…" + file.slice(-69) : file, file };
    }
    const hint = obj["command"] ?? obj["pattern"] ?? obj["url"] ?? obj["description"] ?? obj["query"];
    if (typeof hint === "string") {
      return { name, hint: hint.length > 60 ? hint.slice(0, 60) + "…" : hint, file: null };
    }
  }
  return { name, hint: "", file: null };
}

function pretty(v: unknown): string {
  if (typeof v === "string") {
    try {
      return JSON.stringify(JSON.parse(v), null, 2);
    } catch {
      return v;
    }
  }
  return JSON.stringify(v, null, 2);
}
