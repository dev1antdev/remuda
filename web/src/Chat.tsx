import { memo, useCallback, useEffect, useRef, useState } from "react";
import { fetchChat, fetchQuick, saveQuick, sendKeys, submitPrompt, type ChatResponse, type Part, type QuickPrompt, type Turn } from "./api";
import type { AgentTarget } from "./Tasks";
import SendToAgent from "./SendToAgent";
import { filesFrom, MAX_ATTACHMENTS, useAttachments, type Attachment } from "./attachments";
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
  agents,
  origin,
  dir,
}: {
  machineId: string;
  paneId: string;
  status: string;
  onOpenFile: OpenFile;
  agents: AgentTarget[]; // where a turn can be forwarded
  origin: string; // "workspace on machine", named in a forwarded message
  dir: string; // the pane's folder, for its own quick prompts
}) {
  // forwarding a turn to another agent
  const [forward, setForward] = useState<string | null>(null);
  const onForward = useCallback((text: string) => setForward(`From ${origin}:\n\n${text}`), [origin]);
  const others = agents.some((a) => a.machineId !== machineId || a.paneId !== paneId);
  const [forwarded, setForwarded] = useState("");
  const noticeTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => clearTimeout(noticeTimer.current), []);
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

  // Follow the bottom only while the reader is there and not busy with the
  // text: a wheel or a finger moving up lets go at once (not after 60px, so
  // a poll landing mid-gesture cannot yank the view back), a drag or a
  // selection in the log holds it, and our own scroll never counts as the
  // reader's.
  const ownScroll = useRef(false);
  const dragging = useRef(false);
  useEffect(() => {
    const log = logRef.current;
    if (!log || !stickRef.current || dragging.current) return;
    const sel = window.getSelection();
    if (sel && !sel.isCollapsed && sel.anchorNode && log.contains(sel.anchorNode)) return;
    ownScroll.current = true;
    log.scrollTop = log.scrollHeight;
    requestAnimationFrame(() => (ownScroll.current = false));
  }, [chat]);

  const onScroll = () => {
    const log = logRef.current;
    if (!log || ownScroll.current) return;
    stickRef.current = log.scrollHeight - log.scrollTop - log.clientHeight < 60;
  };
  const letGo = (up: boolean) => {
    if (up) stickRef.current = false;
  };
  useEffect(() => {
    const log = logRef.current;
    if (!log) return;
    const onWheel = (e: WheelEvent) => letGo(e.deltaY < 0);
    let touchY = 0;
    const onTouchStart = (e: TouchEvent) => (touchY = e.touches[0]?.clientY ?? 0);
    const onTouchMove = (e: TouchEvent) => letGo((e.touches[0]?.clientY ?? 0) > touchY + 4);
    const onDown = () => (dragging.current = true);
    const onUp = () => (dragging.current = false);
    log.addEventListener("wheel", onWheel, { passive: true });
    log.addEventListener("touchstart", onTouchStart, { passive: true });
    log.addEventListener("touchmove", onTouchMove, { passive: true });
    log.addEventListener("mousedown", onDown);
    document.addEventListener("mouseup", onUp);
    return () => {
      log.removeEventListener("wheel", onWheel);
      log.removeEventListener("touchstart", onTouchStart);
      log.removeEventListener("touchmove", onTouchMove);
      log.removeEventListener("mousedown", onDown);
      document.removeEventListener("mouseup", onUp);
    };
  }, []);

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
          <TurnView key={i} turn={t} onOpenFile={onOpenFile} onForward={others ? onForward : undefined} />
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
      <Composer machineId={machineId} paneId={paneId} working={status === "run"} dir={dir} />
      {forwarded && <div className="tasks-notice chat-notice">{forwarded}</div>}
      {forward !== null && (
        <SendToAgent
          title="Forward to another agent"
          text={forward}
          agents={agents}
          currentPane={{ machineId, paneId }}
          exclude={{ machineId, paneId }}
          rememberKey={`forward:${machineId}|${paneId}`}
          onClose={() => setForward(null)}
          onSend={async (target, text) => {
            await submitPrompt(target.machineId, target.paneId, text);
            setForward(null);
            setForwarded(`Forwarded to ${target.agent} in ${target.sessionName}.`);
            clearTimeout(noticeTimer.current);
            noticeTimer.current = setTimeout(() => setForwarded(""), 4000);
          }}
        />
      )}
    </div>
  );
}

// The composer owns the draft, so typing never re-renders the transcript.
// Attachments (paperclip, a pasted screenshot, a drop) upload into the pane's
// folder and put an @path mention at the caret; the agent reads the file
// from there once the message is sent.
function Composer({ machineId, paneId, working, dir }: { machineId: string; paneId: string; working: boolean; dir: string }) {
  const [draft, setDraft] = useState("");
  const [quick, setQuick] = useState<{ x: number; y: number } | null>(null);
  const [manage, setManage] = useState(false);
  const [prompts, setPrompts] = useState<QuickPrompt[] | null>(null); // null: not loaded (yet)
  const [quickErr, setQuickErr] = useState("");
  useEffect(() => {
    if (!quick) return;
    setPrompts(null);
    setQuickErr("");
    fetchQuick().then(setPrompts, (e) => setQuickErr(errMsg(e)));
  }, [quick]);
  useEffect(() => {
    if (!quick) return;
    const close = () => setQuick(null);
    document.addEventListener("click", close);
    return () => document.removeEventListener("click", close);
  }, [quick]);
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
  const dictation = useDictation((text) => {
    caret.current = null; // the next insertion goes after the dictated text
    setDraft((d) => (d ? d.replace(/\s*$/, " ") : "") + text);
  });

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
        <button
          type="button"
          className={`composer-attach${quick ? " on" : ""}`}
          title="Quick prompts"
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
            className={`composer-attach composer-mic${dictation.listening ? " listening" : ""}`}
            title={dictation.listening ? "Stop dictating" : "Dictate (on this device's speech recognition)"}
            onClick={dictation.toggle}
          >
            <Icon name="mic" size={16} />
          </button>
        )}
        {quick && (
          <div className="row-menu quick-menu" style={{ left: quick.x, top: quick.y }} onClick={(e) => e.stopPropagation()}>
            {(prompts ?? [])
              .filter((q) => !q.dir || q.dir === dir)
              .map((q) => (
                <button key={q.id ?? q.label} title={q.text} onClick={() => { insertAt(q.text); setQuick(null); area.current?.focus(); }}>
                  <Icon name="zap" size={12} className="icon quick-icon" />
                  <span className="quick-label">{q.label}</span>
                  {q.dir && <span className="sel-hint">this folder</span>}
                </button>
              ))}
            {quickErr ? (
              <div className="chat-error">{quickErr}</div>
            ) : !prompts ? (
              <div className="placeholder small">Loading…</div>
            ) : (
              prompts.filter((q) => !q.dir || q.dir === dir).length === 0 && <div className="placeholder small">No quick prompts yet.</div>
            )}
            <button className="quick-manage" onClick={() => { setQuick(null); setManage(true); }}>
              <Icon name="sliders" size={12} className="icon quick-icon" /> Manage…
            </button>
          </div>
        )}
        {manage && <QuickPromptsDialog dir={dir} onClose={() => setManage(false)} />}
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
            <Icon name="square" size={12} /> <span>Stop</span>
          </button>
        )}
      </div>
    </>
  );
}

// Saved prompts: edit the whole list in one place — a label, the text, and
// whether it is for every folder or this one.
function QuickPromptsDialog({ dir, onClose }: { dir: string; onClose: () => void }) {
  // the list is replaced whole on save, so nothing is saved until the
  // current one has loaded
  const [list, setList] = useState<QuickPrompt[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  useEffect(() => {
    fetchQuick().then(setList, (e) => setErr(errMsg(e)));
  }, []);
  const edit = (i: number, patch: Partial<QuickPrompt>) => setList((l) => (l ?? []).map((q, j) => (j === i ? { ...q, ...patch } : q)));
  const folder = dir.split("/").filter(Boolean).pop() ?? dir;
  return (
    <div className="modal-scrim" onClick={onClose}>
      <form
        className="modal quick-dialog"
        onClick={(e) => e.stopPropagation()}
        onSubmit={(e) => {
          e.preventDefault();
          if (!list) return;
          setBusy(true);
          setErr("");
          saveQuick(list.filter((q) => q.text.trim()))
            .then(onClose, (er) => setErr(errMsg(er)))
            .finally(() => setBusy(false));
        }}
      >
        <h2>Quick prompts</h2>
        <p className="modal-hint">Offered under ⚡ in the composer; one tap puts the text in. Shared by every device that uses this herdr-ui.</p>
        {!list && !err && <div className="placeholder small">Loading…</div>}
        <div className="quick-list">
          {(list ?? []).map((q, i) => (
            <div key={i} className="quick-row">
              <input value={q.label} placeholder="Label" onChange={(e) => edit(i, { label: e.target.value })} />
              <textarea rows={2} value={q.text} placeholder="The prompt" onChange={(e) => edit(i, { text: e.target.value })} />
              <label className="quick-scope">
                <input type="checkbox" checked={!!q.dir} onChange={(e) => edit(i, { dir: e.target.checked ? dir : "" })} /> only {folder}
              </label>
              <button type="button" className="icon-btn" title="Remove" onClick={() => setList((l) => (l ?? []).filter((_, j) => j !== i))}>
                <Icon name="trash" size={14} />
              </button>
            </div>
          ))}
        </div>
        <button type="button" className="sidebar-head-btn" disabled={!list} onClick={() => setList((l) => [...(l ?? []), { label: "", text: "" }])}>
          <Icon name="plus" size={13} /> Add
        </button>
        {err && <div className="chat-error">{err}</div>}
        <div className="modal-actions">
          <button type="button" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className="primary" disabled={busy || !list}>
            Save
          </button>
        </div>
      </form>
    </div>
  );
}

// Dictation through the browser's own speech recognition (Safari and
// Chrome ship one; it runs on the device or the platform's service, not
// through herdr-ui). Final results are handed over as they come.
type SpeechRecognitionLike = {
  lang: string;
  continuous: boolean;
  interimResults: boolean;
  onresult: ((e: { resultIndex: number; results: ArrayLike<{ isFinal: boolean; 0: { transcript: string } }> }) => void) | null;
  onend: (() => void) | null;
  onerror: (() => void) | null;
  start: () => void;
  stop: () => void;
};
function useDictation(onText: (text: string) => void) {
  const ctor = (window as unknown as { SpeechRecognition?: new () => SpeechRecognitionLike; webkitSpeechRecognition?: new () => SpeechRecognitionLike }).SpeechRecognition
    ?? (window as unknown as { webkitSpeechRecognition?: new () => SpeechRecognitionLike }).webkitSpeechRecognition;
  const [listening, setListening] = useState(false);
  const rec = useRef<SpeechRecognitionLike | null>(null);
  const toggle = () => {
    if (!ctor) return;
    if (rec.current) {
      rec.current.stop();
      return;
    }
    const r = new ctor();
    r.lang = navigator.language || "en-US";
    r.continuous = true;
    r.interimResults = false;
    r.onresult = (e) => {
      for (let i = e.resultIndex; i < e.results.length; i++) {
        const res = e.results[i]!;
        if (res.isFinal) onText(res[0].transcript.trim());
      }
    };
    r.onend = () => {
      rec.current = null;
      setListening(false);
    };
    r.onerror = () => {
      rec.current = null;
      setListening(false);
    };
    rec.current = r;
    try {
      r.start();
      setListening(true);
    } catch {
      rec.current = null;
    }
  };
  // leaving the pane stops the microphone and nothing lands afterwards
  useEffect(
    () => () => {
      const r = rec.current;
      if (!r) return;
      r.onresult = r.onend = r.onerror = null;
      r.stop();
      rec.current = null;
    },
    [],
  );
  return { supported: !!ctor, listening, toggle };
}

const TurnView = memo(
  function TurnView({ turn, onOpenFile, onForward }: { turn: Turn; onOpenFile: OpenFile; onForward?: (text: string) => void }) {
    const cls = turn.role === "user" ? "turn turn-user" : "turn turn-assistant";
    const text = turn.parts.filter((p) => p.type === "text").map((p) => p.text ?? "").join("\n\n").trim();
    return (
      <div className={cls}>
        {turn.parts.map((p, i) => (
          <PartView key={i} part={p} onOpenFile={onOpenFile} />
        ))}
        {onForward && text && turn.role === "assistant" && (
          <button type="button" className="turn-forward" title="Forward this to another agent" onClick={() => onForward(text)}>
            <Icon name="send" size={12} /> forward
          </button>
        )}
      </div>
    );
  },
  (a, b) => a.onOpenFile === b.onOpenFile && a.onForward === b.onForward && sameTurn(a.turn, b.turn),
);

// every poll brings new objects; only the content tells turns apart (a
// tool's input never changes once written, so it is not compared)
function sameTurn(a: Turn, b: Turn): boolean {
  if (a.role !== b.role || a.ts !== b.ts || a.parts.length !== b.parts.length) return false;
  for (let i = 0; i < a.parts.length; i++) {
    const p = a.parts[i]!;
    const q = b.parts[i]!;
    if (p.type !== q.type || p.text !== q.text || p.output !== q.output || p.is_error !== q.is_error || p.tool_name !== q.tool_name) return false;
  }
  return true;
}

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
