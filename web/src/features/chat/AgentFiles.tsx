// The files rail's Agent tab: every file the agent edited or read this session, from its own tool calls, the newest
// touch first. A click opens the file with its changes; the code button opens it in the Code mode on that machine. It
// polls the transcript itself, at a wall tile's pace, only while on screen, and says what it counts for the rail's head.

import { useEffect, useMemo, useState } from "react";
import type { OpenFile } from "@/features/files/target";
import { basename, dirname, plural } from "@/lib/text";
import { ago } from "@/lib/time";
import { Icon, Tip } from "@/ui";
import { FileIcon } from "@/ui/FileIcon";
import { footprint, shownPath, type Touch } from "./footprint";
import { useChat } from "./useChat";
import { t } from "@/lib/i18n";

const RECENT_MS = 60_000;

export default function AgentFiles({ machineId, paneId, status, dir, active, onOpenFile, onOpenInCode, onSummary }: {
  machineId: string;
  paneId: string;
  status: string;
  dir: string;
  active: boolean;
  onOpenFile: OpenFile;
  onOpenInCode?: (path: string) => void; // absent on a machine with no editor (a Windows PC): no code button
  onSummary: (words: string) => void; // what the head says beside the tabs
}) {
  const busy = status === "run" || status === "input";
  const { data: chat, error } = useChat({ machineId, paneId, sessionId: null, dir, every: busy ? 3000 : 15000, active });
  const { edited, read } = useMemo(() => footprint(chat?.turns ?? []), [chat]);
  const now = useClock(active);
  const [showRead, setShowRead] = useState(false);
  const summary = !chat ? "…" : `${edited.length > 0 ? t("{files} edited", { files: plural(edited.length, "file") }) : t("nothing edited")}${read.length > 0 ? ` · ${t("{n} read", { n: read.length })}` : ""}`;
  useEffect(() => onSummary(summary), [summary, onSummary]);
  const row = (touch: Touch) => (
    <Row
      key={touch.path}
      touch={touch}
      dir={dir}
      now={now}
      onOpen={() => onOpenFile({ path: touch.path, prefer: touch.edits > 0 ? "changes" : "file" })}
      onCode={onOpenInCode && (() => onOpenInCode(touch.path))}
    />
  );
  return (
    <div className="footprint-list">
      {chat === null && !error && <div className="empty">{t("Loading…")}</div>}
      {error && <div className="empty">{error}</div>}
      {chat && edited.length === 0 && read.length === 0 && <div className="empty">{t("No file touched yet.")}</div>}
      {edited.map(row)}
      {read.length > 0 && (
        <div className="fp-fold">
          <button type="button" className="linkish" aria-expanded={showRead} onClick={() => setShowRead((shown) => !shown)}>
            <Icon name={showRead ? "chevron-down" : "chevron-right"} size={12} /> {t("{files} only read", { files: plural(read.length, "file") })}
          </button>
          {showRead && read.map(row)}
        </div>
      )}
    </div>
  );
}

function Row({ touch, dir, now, onOpen, onCode }: { touch: Touch; dir: string; now: number; onOpen: () => void; onCode?: () => void }) {
  const shown = shownPath(touch.path, dir);
  const recent = touch.at > 0 && now - touch.at < RECENT_MS;
  return (
    <div className={`fp-row${recent ? " recent" : ""}`}>
      <button type="button" className="linkish fp-open" title={`${touch.path}\n${touch.edits > 0 ? t("Open the file with its changes") : t("Open the file")}`} onClick={onOpen}>
        <span className="fp-path">
          <FileIcon path={touch.path} size={14} className="fp-icon" />
          <span className="fp-name">{basename(shown)}</span>
          <span className="fp-dir">{dirname(shown)}</span>
        </span>
        <span className="fp-meta">
          {touch.failed && <Icon name="alert-triangle" size={12} className="fp-failed" />}
          {touch.edits > 0 && plural(touch.edits, "edit")}
          {touch.edits > 0 && touch.reads > 0 && " · "}
          {touch.reads > 0 && plural(touch.reads, "read")}
          {touch.at > 0 && ` · ${ago(touch.at, now)}`}
        </span>
      </button>
      {onCode && (
        <Tip text={t("Open in Code (VS Code on this machine)")}>
          <button type="button" className="icon-btn sm fp-code" aria-label={t("Open in Code")} onClick={onCode}>
            <Icon name="code" size={12} />
          </button>
        </Tip>
      )}
    </div>
  );
}

// the "ago" words move on their own, half a minute at a time, while the rail is on screen
function useClock(active: boolean): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!active) return;
    setNow(Date.now());
    const timer = setInterval(() => setNow(Date.now()), 30_000);
    return () => clearInterval(timer);
  }, [active]);
  return now;
}
