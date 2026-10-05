import { memo, useEffect, useRef, useState } from "react";
import { fetchChat, sendKeys, submitPrompt, type ChatResponse, type Part, type Turn } from "./api";
import { Markdown, type OpenFile } from "./markdown";
import { errMsg, isTouch } from "./prefs";

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
        <select
          className="chat-session-pick"
          value={sessionId ?? chat?.session_id ?? ""}
          onChange={(e) => setSessionId(e.target.value)}
        >
          {sessions.map((s) => (
            <option key={s.id} value={s.id}>
              {(s.title || s.id.slice(0, 8)) + " · " + new Date(s.mod_time).toLocaleString()}
            </option>
          ))}
        </select>
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
function Composer({ machineId, paneId, working }: { machineId: string; paneId: string; working: boolean }) {
  const [draft, setDraft] = useState("");
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const send = async () => {
    const text = draft.trim();
    if (!text || sending) return;
    setSending(true);
    try {
      await submitPrompt(machineId, paneId, text);
      setDraft("");
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
      <div className="composer">
        <textarea
          value={draft}
          placeholder={isTouch ? "Message the agent… (Enter = new line)" : "Message the agent… (Shift+Enter = new line)"}
          rows={Math.min(6, Math.max(1, draft.split("\n").length))}
          onChange={(e) => setDraft(e.target.value)}
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
        <button className="composer-send" disabled={sending || !draft.trim()} onClick={() => void send()}>
          {sending ? "…" : "Send"}
        </button>
        {working && (
          <button
            className="composer-stop"
            title="Send Esc to interrupt"
            onClick={() => void sendKeys(machineId, paneId, ["esc"])}
          >
            Stop
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
      <Folded className="part-thinking" summary="Thinking">
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
            {part.is_error ? "✗ " : "▸ "}
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
