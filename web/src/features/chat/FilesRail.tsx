// The files rail beside a pane's lenses: every file the agent edited or read this session, from its own tool
// calls, the newest touch first. A click opens the file with its changes; the code button opens it in the
// Code mode on that machine. It polls the transcript itself, at a wall tile's pace, only while on screen.

import { useEffect, useMemo, useState } from "react";
import { actions, app } from "@/app/store";
import type { OpenFile } from "@/features/files/target";
import { basename, dirname, plural } from "@/lib/text";
import { ago } from "@/lib/time";
import { Icon } from "@/ui";
import { footprint, shownPath, type Touch } from "./footprint";
import { useChat } from "./useChat";

const RECENT_MS = 60_000;

export default function FilesRail({ machineId, paneId, status, dir, active, onOpenFile, onOpenInCode }: {
  machineId: string;
  paneId: string;
  status: string;
  dir: string;
  active: boolean;
  onOpenFile: OpenFile;
  onOpenInCode: (path: string) => void;
}) {
  const busy = status === "run" || status === "input";
  const { data: chat, error } = useChat({ machineId, paneId, sessionId: null, dir, every: busy ? 3000 : 15000, active });
  const { edited, read } = useMemo(() => footprint(chat?.turns ?? []), [chat]);
  const now = useClock(active);
  const [showRead, setShowRead] = useState(false);
  const row = (touch: Touch) => (
    <Row
      key={touch.path}
      touch={touch}
      dir={dir}
      now={now}
      onOpen={() => onOpenFile({ path: touch.path, prefer: touch.edits > 0 ? "changes" : "file" })}
      onCode={() => onOpenInCode(touch.path)}
    />
  );
  return (
    <aside className="footprint" aria-label="Files the agent touched">
      <div className="footprint-head">
        <Icon name="folder" size={14} /> Files
        <span className="footprint-sub">
          {edited.length > 0 ? `${plural(edited.length, "file")} edited` : "nothing edited"}
          {read.length > 0 && ` · ${read.length} read`}
        </span>
      </div>
      <div className="footprint-list">
        {chat === null && !error && <div className="empty">Loading…</div>}
        {error && <div className="empty">{error}</div>}
        {chat && edited.length === 0 && read.length === 0 && <div className="empty">No file touched yet.</div>}
        {edited.map(row)}
        {read.length > 0 && (
          <div className="fp-fold">
            <button type="button" className="linkish" aria-expanded={showRead} onClick={() => setShowRead((shown) => !shown)}>
              <Icon name={showRead ? "chevron-down" : "chevron-right"} size={12} /> {plural(read.length, "file")} only read
            </button>
            {showRead && read.map(row)}
          </div>
        )}
      </div>
    </aside>
  );
}

function Row({ touch, dir, now, onOpen, onCode }: { touch: Touch; dir: string; now: number; onOpen: () => void; onCode: () => void }) {
  const shown = shownPath(touch.path, dir);
  const recent = touch.at > 0 && now - touch.at < RECENT_MS;
  return (
    <div className={`fp-row${recent ? " recent" : ""}`}>
      <button type="button" className="linkish fp-open" title={touch.edits > 0 ? `${touch.path}\nOpen the file with its changes` : `${touch.path}\nOpen the file`} onClick={onOpen}>
        <span className="fp-path">
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
      <button type="button" className="icon-btn sm fp-code" title="Open in Code (VS Code on this machine)" onClick={onCode}>
        <Icon name="code" size={12} />
      </button>
    </div>
  );
}

// FilesRailToggle is the rail's switch in the tab strip's tools slot: there whatever the lens, so nothing in the strip moves.
export function FilesRailToggle({ disabled }: { disabled: boolean }) {
  const on = app.use((s) => s.footprint);
  return (
    <button
      type="button"
      className={`icon-btn kb-toggle fp-toggle${on ? " on" : ""}`}
      aria-pressed={on}
      disabled={disabled}
      title={disabled ? "The files an agent touched: a shell pane has no transcript" : on ? "Hide the files the agent touched" : "The files the agent touched this session"}
      onClick={actions.toggleFootprint}
    >
      <Icon name="folder" />
    </button>
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
