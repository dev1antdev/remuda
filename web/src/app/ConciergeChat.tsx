// The concierge's conversation, drawn by the chat lens's own pieces: its turns, followed at the bottom, polled at the
// pace of a chat, and the composer without attachments; an empty conversation offers a few questions to start with.

import { useCallback, useEffect, useRef, useState } from "react";
import Composer from "@/features/chat/Composer";
import { TurnView } from "@/features/chat/TurnView";
import { useChat } from "@/features/chat/useChat";
import { useFollowBottom } from "@/features/chat/useFollowBottom";
import type { OpenFile } from "@/features/files/target";
import type { Pane } from "@/features/roster/api";
import { t } from "@/lib/i18n";
import { Notice } from "@/ui";
import { HINTS, shownTurns } from "@/features/concierge/concierge";
import { actions } from "./store";

export default function ConciergeChat({ machineId, pane, dir, setAside, onSession }: {
  machineId: string;
  pane: Pane;
  dir: string;
  setAside: string | null; // the conversation a New conversation ended, not drawn while the chat still reads it
  onSession: (id: string | undefined) => void; // the conversation on screen, for the next New conversation to set aside
}) {
  const busy = pane.status === "run" || pane.status === "input";
  const { data: chat, error } = useChat({ machineId, paneId: pane.id, sessionId: null, dir, every: busy ? 2000 : 10000, active: true });
  const logRef = useRef<HTMLDivElement>(null);
  const follow = useFollowBottom(logRef, { active: true, chat, startAtBottom: true });
  const onOpenFile = useCallback<OpenFile>((target) => actions.openFile(machineId, pane.id, target, dir), [machineId, pane.id, dir]);
  const [fill, setFill] = useState<{ text: string; n: number } | undefined>(undefined);
  const turns = shownTurns(chat, setAside);
  const session = chat?.session_id;
  useEffect(() => onSession(session), [session, onSession]);

  return (
    <div className="chat concierge-chat">
      <div className="chat-log" ref={logRef} onScroll={follow.onScroll}>
        {turns.length === 0 && chat !== null && (
          <div className="concierge-hints">
            <p>{t("Ask what is going on, in your own words. The concierge reads your agents and your tasks, and files a task into Proposed when you ask for one.")}</p>
            <div className="concierge-hint-list">
              {HINTS.map((hint) => (
                <button key={hint} type="button" className="btn concierge-hint" onClick={() => setFill((was) => ({ text: hint, n: (was?.n ?? 0) + 1 }))}>
                  {hint}
                </button>
              ))}
            </div>
          </div>
        )}
        {turns.length === 0 && chat === null && <div className="empty chat-empty">{t("Loading…")}</div>}
        {turns.map((turn, i) => (
          <TurnView key={turn.ids?.[0] || i} turn={turn} machineId={machineId} onOpenFile={onOpenFile} live={pane.status === "run" && i === turns.length - 1} />
        ))}
      </div>
      {error && (
        <Notice tone="error" className="chat-notice">
          {error}
        </Notice>
      )}
      <Composer machineId={machineId} paneId={pane.id} working={pane.status === "run"} dir={dir} agent="claude" bare fill={fill} placeholder={t("Ask the concierge…")} />
    </div>
  );
}
