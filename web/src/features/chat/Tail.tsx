// Tail is the chat of a wall tile: the state line, the last few turns at a slower pace, the keys for a question, a
// one-line composer.
// Memoized on its props so the wall's spinner tick redraws the tile's head, not the transcript.

import { memo, useCallback, useMemo, useRef, useState } from "react";
import { actions } from "@/app/store";
import type { OpenFile } from "@/features/files/target";
import { sendKeys } from "@/features/roster/api";
import { useNow } from "@/lib/clock";
import { errMsg } from "@/lib/text";
import { took } from "@/lib/time";
import { Notice } from "@/ui";
import type { Subagent } from "./api";
import Composer from "./Composer";
import { useRoster } from "@/features/roster/useRoster";
import { doing, type Doing } from "./doing";
import { counts, progressWords } from "./plan";
import { QuestionCard } from "./QuestionCard";
import { said } from "./said";
import { subagentWords } from "./subagents";
import { TurnView } from "./TurnView";
import { useChat } from "./useChat";
import { useFollowBottom } from "./useFollowBottom";
import { t } from "@/lib/i18n";

export const TAIL_TURNS = 12;

const Tail = memo(function Tail({ machineId, paneId, status, dir, agent = "", words, settled = false, said: fallback, active }: {
  machineId: string;
  paneId: string;
  status: string;
  dir: string;
  agent?: string; // the pane's agent kind, for the composer's completion
  words: string;
  settled?: boolean; // the agent finished or sits idle: the state line says what it answered
  said?: string; // the roster's line, until the tail has the turns
  active: boolean;
}) {
  const busy = status === "run" || status === "input";
  const { data: chat, error: fetchError } = useChat({ machineId, paneId, sessionId: null, dir, every: busy ? 3000 : 15000, active, last: TAIL_TURNS });
  const logRef = useRef<HTMLDivElement>(null);
  const follow = useFollowBottom(logRef, { active, chat, startAtBottom: true });
  const onOpenFile = useCallback<OpenFile>((target) => actions.openFile(machineId, paneId, target, dir), [machineId, paneId, dir]);
  const turns = chat?.turns ?? [];
  const now = useMemo(() => (status === "run" && turns.length > 0 ? doing(turns) : null), [status, turns]);
  const answered = useMemo(() => (settled ? (turns.length > 0 ? said(turns) : fallback) : undefined), [settled, turns, fallback]);
  const subagents = chat?.subagents;
  const subs = useMemo(() => subagentWords(subagents, now?.what === "subagents"), [subagents, now]);
  const session = chat?.session_id ?? "";
  const onOpenSubagent = useCallback((s: Subagent) => actions.openViewer({ machine: machineId, dir, session, subagent: { id: s.id, description: s.description || s.kind || s.id } }), [machineId, dir, session]);
  const plan = chat?.plan;
  const progress = useMemo(() => (plan && counts(plan).open > 0 ? progressWords(plan) : undefined), [plan]);
  return (
    <div className="chat tail">
      <TileState words={words} doing={now} said={answered} plan={progress} subagents={subs} />
      <div className="chat-log" ref={logRef} onScroll={follow.onScroll}>
        {turns.length === 0 && <div className="empty chat-empty">{chat === null ? t("Loading…") : t("No transcript yet.")}</div>}
        {turns.map((t, i) => (
          <TurnView key={t.ids?.[0] || i} turn={t} machineId={machineId} onOpenFile={onOpenFile} subagents={subagents} onOpenSubagent={onOpenSubagent} />
        ))}
      </div>
      <TailAnswer machineId={machineId} paneId={paneId} status={status} dir={dir} agent={agent} active={active} error={fetchError || null} />
    </div>
  );
});

export default Tail;

// TileState is the line under a tile's head: the state and how long, and while the agent works, what it does this second;
// once it has finished, what it answered; while its plan has open items, how far it is; and how many subagents still work.
export function TileState({ words, doing, said, plan, subagents }: { words: string; doing?: Doing | null; said?: string; plan?: string; subagents?: string }) {
  return (
    <div className="tile-state">
      <span className="tile-words">{words}</span>
      {plan && (
        <span className="tile-plan" title={t("The agent's plan: items done of all")}>
          {plan}
        </span>
      )}
      {said && !doing && (
        <span className="tile-said" title={said}>
          · {said}
        </span>
      )}
      {doing && (
        <span className="tile-doing" title={doing.what === "tool" ? `${doing.name} ${doing.hint}` : undefined}>
          {doing.what === "tool" ? (
            <>
              <span className="tile-doing-name">{doing.name}</span> {doing.hint}
              {doing.at && <Running since={doing.at} />}
            </>
          ) : doing.what === "subagents" ? (
            `${subagents || "waiting on a subagent"}…`
          ) : (
            `${doing.what}…`
          )}
        </span>
      )}
      {subagents && doing?.what !== "subagents" && <span className="tile-subs">· {subagents}</span>}
    </div>
  );
}

// LONG_CALL is when a running call turns amber: a command going on past a minute deserves a look
export const LONG_CALL = 60;

// Running counts how long the call without a result has run, a second at a time, amber past LONG_CALL
function Running({ since }: { since: string }) {
  const now = useNow(1000);
  const seconds = Math.max(0, (now - new Date(since).getTime()) / 1000);
  if (!Number.isFinite(seconds)) return null;
  return (
    <span className={seconds >= LONG_CALL ? "tile-running is-long" : "tile-running"} title={seconds >= LONG_CALL ? t("This call has run for over a minute") : undefined}>
      {" · "}
      {took(seconds)}
    </span>
  );
}

// TailAnswer is the foot of a tile whatever it shows above: the keys for a question the agent asks, an error, the one-line composer.
export function TailAnswer({ machineId, paneId, status, dir, agent = "", active = true, error: shown = null }: { machineId: string; paneId: string; status: string; dir: string; agent?: string; active?: boolean; error?: string | null }) {
  const [actionError, setActionError] = useState<string | null>(null);
  const { viewOnly } = useRoster();
  const keys = (ks: string[]) => sendKeys(machineId, paneId, ks).catch((e) => setActionError(errMsg(e)));
  const error = actionError ?? shown;
  if (viewOnly) {
    return error ? (
      <Notice tone="error" className="chat-notice">
        {error}
      </Notice>
    ) : null;
  }
  return (
    <>
      {status === "input" && (
        <Notice tone="warn" icon="alert-triangle" className="chat-notice">
          <QuestionCard machineId={machineId} paneId={paneId} active={active} compact onKeys={keys}>
            {t("Waiting for an answer:")}
          </QuestionCard>
        </Notice>
      )}
      {error && (
        <Notice tone="error" className="chat-notice" onDismiss={() => setActionError(null)}>
          {error}
        </Notice>
      )}
      <Composer machineId={machineId} paneId={paneId} working={status === "run"} dir={dir} agent={agent} compact />
    </>
  );
}
