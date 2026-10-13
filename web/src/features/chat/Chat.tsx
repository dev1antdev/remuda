import { memo, useCallback, useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { actions, app } from "@/app/store";
import { useAgentTargets } from "@/features/agents/useAgentTargets";
import { fetchChat, fetchQuick, saveQuick, type ChatResponse, type Part, type QuickPrompt, type Turn } from "@/features/chat/api";
import { sendKeys, submitPrompt, type PaneUsage } from "@/features/roster/api";
import SendToAgent from "@/features/agents/SendToAgent";
import { filesFrom } from "@/lib/transfer";
import { MAX_ATTACHMENTS, useAttachments, type Attachment } from "@/features/chat/attachments";
import { Markdown } from "@/ui/Markdown";
import type { OpenFile } from "@/features/files/target";
import { basename, errMsg } from "@/lib/text";
import { isTouch } from "@/lib/prefs";
import Select from "@/ui/Select";
import Icon from "@/ui/icons";
import { whenShort } from "@/lib/time";
import { Dialog, Notice, useTimedNotice } from "@/ui";

// Chat is the chat lens: the transcript from Claude Code's own session file, read server-side; sending types into the live pane
export default function Chat({ machineId, paneId, status, origin, dir, usage, active = true }: {
  machineId: string;
  paneId: string; // "" shows a transcript on its own (a search jump), read-only
  status: string;
  origin: string; // "workspace on machine", named in a forwarded message
  dir: string;
  usage?: PaneUsage;
  active?: boolean; // the lens on screen; a hidden one keeps its state and waits
}) {
  const readOnly = !paneId;
  const agents = useAgentTargets();
  // the jump names its pane ("" = the viewer): this chat takes it only when it is that pane's
  const anyJump = app.use((s) => s.jump);
  const jump = anyJump && anyJump.machineId === machineId && anyJump.paneId === paneId ? anyJump : null;
  const onOpenFile = useCallback<OpenFile>((target) => actions.openFile(machineId, paneId, target), [machineId, paneId]);
  const activeRef = useRef(active);
  activeRef.current = active;
  const [forward, setForward] = useState<string | null>(null);
  const onForward = useCallback((text: string) => setForward(`From ${origin}:\n\n${text}`), [origin]);
  const others = agents.some((a) => a.machineId !== machineId || a.paneId !== paneId);
  const [forwarded, showForwarded, clearForwarded] = useTimedNotice(4000);
  const [chat, setChat] = useState<ChatResponse | null>(null);
  const [sessionId, setSessionId] = useState<string | null>(jump?.session ?? null);
  const [error, setError] = useState<string | null>(null);
  const etagRef = useRef<string | null>(null);
  const logRef = useRef<HTMLDivElement>(null);
  const stickRef = useRef(!jump);
  const busy = status === "run" || status === "input";
  const busyRef = useRef(busy);
  busyRef.current = busy;

  const pendingTurn = useRef<string | null>(jump?.turn ?? null);
  const [jumpMiss, setJumpMiss] = useState(false);
  const shownSession = useRef<string | undefined>(undefined);
  shownSession.current = chat?.session_id;
  useEffect(() => {
    if (!jump) return;
    pendingTurn.current = jump.turn || null;
    stickRef.current = !jump.turn;
    setJumpMiss(false);
    // the session on screen is not refetched: the poll effect blanks the log first
    if (shownSession.current !== jump.session) setSessionId(jump.session);
  }, [jump]);

  // the status sets the pace through a ref: a flip must not restart the poll, which blanks the log
  const pollNow = useRef<() => void>(() => {});
  useEffect(() => {
    etagRef.current = null;
    setChat(null);
    let stop = false;
    let inflight = false;
    let again = false;
    let timer: ReturnType<typeof setTimeout> | null = null;

    const poll = async () => {
      if (stop) return;
      if (inflight) {
        again = true;
        return;
      }
      if (timer) clearTimeout(timer);
      timer = null;
      inflight = true;
      try {
        if (document.visibilityState === "visible" && activeRef.current) {
          const res = await fetchChat(machineId, paneId, sessionId, etagRef.current, dir);
          if (stop) return;
          if (res) {
            etagRef.current = res.etag;
            setChat(res.body);
          }
          setError(null);
        }
      } catch (e) {
        if (!stop) setError(errMsg(e));
      } finally {
        inflight = false;
        if (!stop) {
          timer = setTimeout(poll, again ? 0 : busyRef.current ? 2000 : 10000);
          again = false;
        }
      }
    };
    pollNow.current = () => void poll();
    void poll();
    return () => {
      stop = true;
      pollNow.current = () => {};
      if (timer) clearTimeout(timer);
    };
  }, [machineId, paneId, sessionId, dir]);
  // not on mount: the poll above has just gone out
  const busySeen = useRef(false);
  useEffect(() => {
    if (!busySeen.current) {
      busySeen.current = true;
      return;
    }
    if (active) pollNow.current();
  }, [busy, active]);

  // a hidden element forgets its scroll, so the place is put back on screen; this runs before the jump effect below, so a search hit wins
  const savedTop = useRef<number | null>(null);
  useEffect(() => {
    const log = logRef.current;
    if (!active || !log) return;
    ownScroll.current = true;
    log.scrollTop = stickRef.current ? log.scrollHeight : (savedTop.current ?? log.scrollTop);
    requestAnimationFrame(() => (ownScroll.current = false));
  }, [active]);

  useEffect(() => {
    const id = pendingTurn.current;
    const log = logRef.current;
    if (!id || !log || !chat || !jump || chat.session_id !== jump.session) return;
    const el = log.querySelector(`[data-ids~="${CSS.escape(id)}"]`);
    pendingTurn.current = null;
    if (!el) {
      setJumpMiss(true);
      stickRef.current = true;
      return;
    }
    stickRef.current = false;
    el.scrollIntoView({ block: "start" });
    // remove, reflow, add: the CSS animation restarts when the class is still there from a previous jump
    el.classList.remove("turn-hit");
    void (el as HTMLElement).offsetWidth;
    el.classList.add("turn-hit");
    setTimeout(() => el.classList.remove("turn-hit"), 2600);
  }, [chat, jump]);

  // the log follows the bottom only while the reader is there and not selecting or dragging; an upward wheel or finger lets go at once
  const ownScroll = useRef(false);
  const dragging = useRef(false);
  const released = useRef(false);
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
    if (log.clientHeight > 0) savedTop.current = log.scrollTop; // a hidden log reads 0: not a place
    const d = log.scrollHeight - log.scrollTop - log.clientHeight;
    if (d < 1) released.current = false;
    stickRef.current = !released.current && d < 60;
  };
  useEffect(() => {
    const log = logRef.current;
    if (!log) return;
    const letGo = (up: boolean) => {
      released.current = up;
      if (up) stickRef.current = false;
    };
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
  const keys = (ks: string[]) => void sendKeys(machineId, paneId, ks).catch((e) => setError(errMsg(e)));
  const context = usage && usage.window > 0 ? usage.context / usage.window : 0;

  return (
    <div className="chat">
      {sessions.length > 1 && (
        <Select
          className="chat-session-pick"
          value={sessionId ?? chat?.session_id ?? ""}
          onChange={setSessionId}
          options={sessions.map((s) => ({ value: s.id, label: s.title || s.id.slice(0, 8), hint: whenShort(s.mod_time) }))}
        />
      )}
      <div className="chat-log" ref={logRef} onScroll={onScroll}>
        {turns.length === 0 && (
          <div className="empty chat-empty">
            {chat === null ? "Loading…" : readOnly ? "This transcript is empty." : "No transcript for this pane's folder yet."}
          </div>
        )}
        {turns.map((t, i) => (
          <TurnView key={t.ids?.[0] || i} turn={t} onOpenFile={onOpenFile} onForward={others ? onForward : undefined} />
        ))}
      </div>
      {!readOnly && status === "input" && (
        <Notice tone="warn" icon="alert-triangle" className="chat-notice">
          The agent is waiting for an answer — check the terminal, or:
          <span className="prompt-card-keys">
            <button type="button" className="btn sm" title="Up" aria-label="Up" onClick={() => keys(["up"])}>
              <Icon name="chevron-up" size={14} />
            </button>
            <button type="button" className="btn sm" title="Down" aria-label="Down" onClick={() => keys(["down"])}>
              <Icon name="chevron-down" size={14} />
            </button>
            <button type="button" className="btn sm" onClick={() => keys(["enter"])}>
              Enter
            </button>
            <button type="button" className="btn sm" onClick={() => keys(["esc"])}>
              Esc
            </button>
          </span>
        </Notice>
      )}
      {error && (
        <Notice tone="error" className="chat-notice" onDismiss={() => setError(null)}>
          {error}
        </Notice>
      )}
      {!readOnly && context >= 0.8 && status !== "run" && (
        <Notice
          tone="warn"
          icon="chart"
          className={`chat-notice${context >= 0.9 ? " is-red" : ""}`}
          action={
            <button type="button" className="linkish" onClick={() => void submitPrompt(machineId, paneId, "/compact").catch((e) => setError(errMsg(e)))}>
              Compact now
            </button>
          }
        >
          Context {Math.round(100 * context)}% full — Claude Code compacts on its own near the edge; compacting now keeps the summary yours to steer.
        </Notice>
      )}
      {jumpMiss && (
        <Notice tone="info" className="chat-notice" onDismiss={() => setJumpMiss(false)}>
          That turn is older than the loaded part of this transcript (the last 16 MB).
        </Notice>
      )}
      {!readOnly && <Composer machineId={machineId} paneId={paneId} working={status === "run"} dir={dir} />}
      {forwarded && (
        <Notice tone="success" icon="check" className="chat-notice" onDismiss={clearForwarded}>
          {forwarded}
        </Notice>
      )}
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
            showForwarded(`Forwarded to ${target.agent} in ${target.sessionName}.`);
          }}
        />
      )}
    </div>
  );
}

// Composer owns the draft, so typing never re-renders the transcript
function Composer({ machineId, paneId, working, dir }: { machineId: string; paneId: string; working: boolean; dir: string }) {
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
  const [prompts, setPrompts] = useState<QuickPrompt[] | null>(null);
  const [quickErr, setQuickErr] = useState("");
  useEffect(() => {
    if (!quick) return;
    let stop = false;
    setPrompts(null);
    setQuickErr("");
    fetchQuick().then(
      (list) => !stop && setPrompts(list),
      (e) => !stop && setQuickErr(errMsg(e)),
    );
    return () => {
      stop = true;
    };
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
      setDraft("");
      att.clear();
      setError(null);
    } catch (e) {
      setError(errMsg(e));
    } finally {
      setSending(false);
    }
  };

  const visible = (prompts ?? []).filter((q) => !q.dir || q.dir === dir);

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
              {a.thumb ? <img src={a.thumb} alt="" /> : <span className="attachment-icon"><Icon name="file" size={18} /></span>}
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
        {quick && (
          <div className="row-menu quick-menu" role="menu" style={{ left: quick.x, top: quick.y }} onClick={(e) => e.stopPropagation()}>
            {visible.map((q) => (
              <button
                key={q.id ?? q.label}
                type="button"
                role="menuitem"
                title={q.text}
                onClick={() => {
                  insertAt(q.text);
                  setQuick(null);
                  area.current?.focus();
                }}
              >
                <Icon name="zap" size={12} className="quick-icon" />
                <span className="quick-label">{q.label}</span>
                {q.dir && <span className="sel-hint">this folder</span>}
              </button>
            ))}
            {quickErr ? (
              <Notice tone="error">{quickErr}</Notice>
            ) : !prompts ? (
              <Notice tone="hint">Loading…</Notice>
            ) : (
              visible.length === 0 && <Notice tone="hint">No quick prompts yet.</Notice>
            )}
            <button
              type="button"
              role="menuitem"
              className="quick-manage"
              onClick={() => {
                setQuick(null);
                setManage(true);
              }}
            >
              <Icon name="sliders" size={12} className="quick-icon" /> Manage…
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
            e.preventDefault();
            void attach(files);
          }}
          onKeyDown={(e) => {
            // phone keyboards have no Shift+Enter: there Enter breaks the line and the button sends
            if (isTouch) return;
            if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
              e.preventDefault();
              void send();
            }
          }}
        />
        <button type="button" className="btn primary composer-send" disabled={sending || att.uploading || !draft.trim()} onClick={() => void send()} title={isTouch ? "Send" : "Send (Enter)"}>
          {sending ? "…" : att.uploading ? "Uploading…" : <><span>Send</span><Icon name="send" size={14} /></>}
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

// QuickPromptsDialog edits the saved prompts as one list: a label, the text, every folder or this one
function QuickPromptsDialog({ dir, onClose }: { dir: string; onClose: () => void }) {
  // the save replaces the list whole, so nothing is saved until the current one has loaded
  const [list, setList] = useState<QuickPrompt[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  useEffect(() => {
    let stop = false;
    fetchQuick().then(
      (l) => !stop && setList(l),
      (e) => !stop && setErr(errMsg(e)),
    );
    return () => {
      stop = true;
    };
  }, []);
  const edit = (i: number, patch: Partial<QuickPrompt>) => setList((l) => (l ?? []).map((q, j) => (j === i ? { ...q, ...patch } : q)));
  const folder = basename(dir);
  return (
    <Dialog
      title="Quick prompts"
      icon="zap"
      size="md"
      as="form"
      className="quick-dialog"
      onClose={onClose}
      onSubmit={(e) => {
        e.preventDefault();
        if (!list || busy) return;
        setBusy(true);
        setErr("");
        saveQuick(list.filter((q) => q.text.trim()))
          .then(onClose, (er) => setErr(errMsg(er)))
          .finally(() => setBusy(false));
      }}
    >
      <p className="modal-hint">
        Offered under <Icon name="zap" size={12} /> in the composer; one tap puts the text in. Shared by every device that uses this remuda.
      </p>
      {!list && !err && <Notice tone="hint">Loading…</Notice>}
      <div className="quick-list">
        {(list ?? []).map((q, i) => (
          <div key={i} className="quick-row">
            <input value={q.label} placeholder="Label" onChange={(e) => edit(i, { label: e.target.value })} />
            <textarea rows={2} value={q.text} placeholder="The prompt" onChange={(e) => edit(i, { text: e.target.value })} />
            <label className="quick-scope">
              <input type="checkbox" checked={!!q.dir} onChange={(e) => edit(i, { dir: e.target.checked ? dir : "" })} /> only {folder}
            </label>
            <button type="button" className="icon-btn danger" title="Remove" aria-label="Remove" onClick={() => setList((l) => (l ?? []).filter((_, j) => j !== i))}>
              <Icon name="trash" size={14} />
            </button>
          </div>
        ))}
      </div>
      <button type="button" className="btn sm" disabled={!list} onClick={() => setList((l) => [...(l ?? []), { label: "", text: "" }])}>
        <Icon name="plus" size={12} /> Add
      </button>
      {err && <Notice tone="error">{err}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          Cancel
        </button>
        <button type="submit" className="btn primary" disabled={busy || !list}>
          Save
        </button>
      </div>
    </Dialog>
  );
}

// dictation through the browser's own speech recognition: on the device or its platform's service, never through remuda
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
type SpeechCtor = new () => SpeechRecognitionLike;
const speech = window as unknown as { SpeechRecognition?: SpeechCtor; webkitSpeechRecognition?: SpeechCtor };
const SPEECH = speech.SpeechRecognition ?? speech.webkitSpeechRecognition;

function useDictation(onText: (text: string) => void) {
  const [listening, setListening] = useState(false);
  const rec = useRef<SpeechRecognitionLike | null>(null);
  const toggle = () => {
    if (!SPEECH) return;
    if (rec.current) {
      rec.current.stop();
      return;
    }
    const r = new SPEECH();
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
  // unmount: the handlers go before stop(), so nothing lands afterwards
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
  return { supported: !!SPEECH, listening, toggle };
}

// TurnView re-renders only when the turn's content changed, so a selection in the log survives the polls
const TurnView = memo(
  function TurnView({ turn, onOpenFile, onForward }: { turn: Turn; onOpenFile: OpenFile; onForward?: (text: string) => void }) {
    const cls = turn.role === "user" ? "turn turn-user" : "turn turn-assistant";
    const text = turn.parts.filter((p) => p.type === "text").map((p) => p.text ?? "").join("\n\n").trim();
    return (
      <div className={cls} data-ids={turn.ids?.join(" ")}>
        {turn.parts.map((p, i) => (
          <PartView key={i} part={p} onOpenFile={onOpenFile} />
        ))}
        {onForward && text && turn.role === "assistant" && (
          <button type="button" className="linkish turn-forward" title="Forward this to another agent" onClick={() => onForward(text)}>
            <Icon name="send" size={12} /> forward
          </button>
        )}
      </div>
    );
  },
  (a, b) => a.onOpenFile === b.onOpenFile && a.onForward === b.onForward && sameTurn(a.turn, b.turn),
);

// sameTurn compares content, as every poll brings new objects; a tool's input never changes once written, so it is skipped
function sameTurn(a: Turn, b: Turn): boolean {
  if (a.role !== b.role || a.ts !== b.ts || a.parts.length !== b.parts.length || a.ids?.length !== b.ids?.length) return false;
  for (let i = 0; i < a.parts.length; i++) {
    const p = a.parts[i]!;
    const q = b.parts[i]!;
    if (p.type !== q.type || p.text !== q.text || p.output !== q.output || p.is_error !== q.is_error || p.tool_name !== q.tool_name) return false;
  }
  return true;
}

const EDIT_TOOLS = new Set(["Edit", "Write", "MultiEdit", "NotebookEdit"]);

// Folded renders its body only once opened: thinking and tool I/O can be kilobytes a turn
function Folded({ className, summary, children }: { className: string; summary: ReactNode; children: ReactNode }) {
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
      <Folded className="part-thinking" summary={<><Icon name="sparkles" size={12} className="part-tool-icon" /> Thinking</>}>
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
            <Icon name={part.is_error ? "alert-triangle" : "chevron-right"} size={12} className="part-tool-icon" />{" "}
            {name}
            {hint && " · "}
            {file ? (
              <button
                type="button"
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
        {part.output && <pre className="tool-io">{part.output}</pre>}
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
