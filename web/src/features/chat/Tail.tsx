// Tail is the chat of a wall tile: the last few turns at a slower pace, the keys for a question, a one-line composer.
// Memoized on its props so the wall's spinner tick redraws the tile's head, not the transcript.

import { memo, useCallback, useRef, useState } from "react";
import { actions } from "@/app/store";
import type { OpenFile } from "@/features/files/target";
import { sendKeys } from "@/features/roster/api";
import { errMsg } from "@/lib/text";
import { Notice } from "@/ui";
import Composer from "./Composer";
import { PromptKeys } from "./PromptKeys";
import { TurnView } from "./TurnView";
import { useChat } from "./useChat";
import { useFollowBottom } from "./useFollowBottom";

export const TAIL_TURNS = 12;

const Tail = memo(function Tail({ machineId, paneId, status, dir, active }: { machineId: string; paneId: string; status: string; dir: string; active: boolean }) {
  const busy = status === "run" || status === "input";
  const { data: chat, error: fetchError } = useChat({ machineId, paneId, sessionId: null, dir, every: busy ? 3000 : 15000, active, last: TAIL_TURNS });
  const logRef = useRef<HTMLDivElement>(null);
  const follow = useFollowBottom(logRef, { active, chat, startAtBottom: true });
  const onOpenFile = useCallback<OpenFile>((target) => actions.openFile(machineId, paneId, target, dir), [machineId, paneId, dir]);
  const turns = chat?.turns ?? [];
  return (
    <div className="chat tail">
      <div className="chat-log" ref={logRef} onScroll={follow.onScroll}>
        {turns.length === 0 && <div className="empty chat-empty">{chat === null ? "Loading…" : "No transcript yet."}</div>}
        {turns.map((t, i) => (
          <TurnView key={t.ids?.[0] || i} turn={t} onOpenFile={onOpenFile} />
        ))}
      </div>
      <TailAnswer machineId={machineId} paneId={paneId} status={status} dir={dir} error={fetchError || null} />
    </div>
  );
});

export default Tail;

// TailAnswer is the foot of a tile whatever it shows above: the keys for a question the agent asks, an error, the one-line composer.
export function TailAnswer({ machineId, paneId, status, dir, error: shown = null }: { machineId: string; paneId: string; status: string; dir: string; error?: string | null }) {
  const [actionError, setActionError] = useState<string | null>(null);
  const keys = (ks: string[]) => void sendKeys(machineId, paneId, ks).catch((e) => setActionError(errMsg(e)));
  const error = actionError ?? shown;
  return (
    <>
      {status === "input" && (
        <Notice tone="warn" icon="alert-triangle" className="chat-notice">
          Waiting for an answer: <PromptKeys onKeys={keys} />
        </Notice>
      )}
      {error && (
        <Notice tone="error" className="chat-notice" onDismiss={() => setActionError(null)}>
          {error}
        </Notice>
      )}
      <Composer machineId={machineId} paneId={paneId} working={status === "run"} dir={dir} compact />
    </>
  );
}
