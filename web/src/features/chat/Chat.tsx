// Chat is the chat lens: the transcript from Claude Code's own session file, read server-side and polled
// at the pane's pace; sending types into the live pane.

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { actions, app } from "@/app/store";
import { useAgentTargets } from "@/features/agents/useAgentTargets";
import SendToAgent from "@/features/agents/SendToAgent";
import type { OpenFile } from "@/features/files/target";
import { act } from "@/app/act";
import { createTab, sendKeys, submitPrompt, type PaneUsage } from "@/features/roster/api";
import { copyText } from "@/lib/host";
import { usePref } from "@/lib/prefs";
import { errMsg } from "@/lib/text";
import { whenShort } from "@/lib/time";
import { Icon, Notice, Select, useTimedNotice } from "@/ui";
import type { Subagent, Turn } from "./api";
import Composer from "./Composer";
import { anchorsItself, hiddenCount, idle, nextReveal } from "./mount";
import { isPinned, pinKey, pinOf, togglePin, type Pin } from "./pins";
import { PlanBar } from "./PlanBar";
import { QuestionCard } from "./QuestionCard";
import { revealTurn } from "./reveal";
import { TimelineBar } from "./TimelineBar";
import { turnLink } from "./turnLink";
import { TurnView } from "./TurnView";
import { useChat } from "./useChat";
import { useFind } from "./useFind";
import { useFollowBottom } from "./useFollowBottom";

export default function Chat({ machineId, paneId, status, origin, dir, usage, workspaceId, agent, active = true, subagent, session }: {
  machineId: string;
  paneId: string; // "" shows a transcript on its own (a search jump), read-only
  status: string;
  origin: string; // "workspace on machine", named in a forwarded message
  dir: string;
  usage?: PaneUsage;
  workspaceId?: string; // with agent: an older session picked here can be resumed in a new tab of this workspace
  agent?: string;
  active?: boolean; // the lens on screen; a hidden one keeps its state and waits
  subagent?: string; // with no pane: one subagent's own conversation, read-only
  session?: string; // with no pane: the session to show when no jump names it (the viewer)
}) {
  const readOnly = !paneId;
  const agents = useAgentTargets();
  // the jump names its pane ("" = the viewer): this chat takes it only when it is that pane's
  const anyJump = app.use((s) => s.jump);
  const jump = anyJump && anyJump.machineId === machineId && anyJump.paneId === paneId ? anyJump : null;
  const onOpenFile = useCallback<OpenFile>((target) => actions.openFile(machineId, paneId, target, dir), [machineId, paneId, dir]);
  const [forward, setForward] = useState<string | null>(null);
  const onForward = useCallback((text: string) => setForward(`From ${origin}:\n\n${text}`), [origin]);
  const others = agents.some((a) => a.machineId !== machineId || a.paneId !== paneId);
  const [forwarded, showForwarded, clearForwarded] = useTimedNotice(4000);
  const [sessionId, setSessionId] = useState<string | null>(jump?.session ?? session ?? null);
  const live = useRef<string | undefined>(undefined); // the pane's own session, noted while it is the one shown
  // the session picked here is the one the export (in the tab strip, the palette) saves; the pane's live one is null
  useEffect(() => {
    if (readOnly) return;
    actions.showChatSession(sessionId);
    return () => actions.showChatSession(null);
  }, [sessionId, readOnly]);
  const [actionError, setActionError] = useState<string | null>(null);
  const logRef = useRef<HTMLDivElement>(null);
  const busy = status === "run" || status === "input";

  // the transcript, at the pane's pace: two seconds while the agent works or asks, ten otherwise
  const { data: chat, error: fetchError } = useChat({ machineId, paneId, sessionId, dir, every: busy ? 2000 : 10000, active, subagent });
  // the pane's own session id goes to the store, so the Pins list can tell its pins from an older session's
  useEffect(() => {
    if (readOnly || sessionId !== null || !chat?.session_id) return;
    actions.setChatLive(chat.session_id);
  }, [readOnly, sessionId, chat?.session_id]);
  useEffect(() => () => actions.setChatLive(null), []);

  // a long conversation mounts from its end: the last turns at once, the earlier ones in idle slices; the find bar and a
  // jump to an earlier turn want them all at once. `revealed` belongs to one session, so a switch starts over.
  const turns = chat?.turns ?? [];
  const loaded = chat?.session_id; // the file on screen, which the revealed count belongs to
  const [reveal, setReveal] = useState<{ session?: string; n: number }>({ n: 0 });
  const revealed = reveal.session === loaded ? reveal.n : 0;
  const hidden = hiddenCount(turns.length, revealed);
  const revealAll = useCallback(() => setReveal({ session: loaded, n: turns.length }), [loaded, turns.length]);
  const anchor = useRef<{ height: number; top: number } | null>(null); // the log's measure before a slice, for a browser without scroll anchoring
  useEffect(() => {
    const log = logRef.current;
    if (hidden === 0 || !active || !log) return;
    return idle(log.ownerDocument.defaultView ?? window, () => {
      anchor.current = { height: log.scrollHeight, top: log.scrollTop };
      setReveal((r) => ({ session: loaded, n: nextReveal(turns.length, r.session === loaded ? r.n : 0) }));
    });
  }, [hidden, active, loaded, turns.length]);
  useLayoutEffect(() => {
    const log = logRef.current;
    const was = anchor.current;
    anchor.current = null;
    if (!log || !was || anchorsItself()) return;
    log.scrollTop = was.top + (log.scrollHeight - was.height);
  }, [hidden]);
  const finding = app.use((s) => s.find !== null);
  useEffect(() => {
    if (finding && hidden > 0) revealAll();
  }, [finding, hidden, revealAll]);
  useFind(logRef, { active, chat, mounted: hidden === 0 });
  const follow = useFollowBottom(logRef, { active, chat, startAtBottom: !jump?.turn, mounted: hidden });

  // a search hit: scroll to its turn once that session is on screen, or say it is older than the loaded tail
  const pendingTurn = useRef<string | null>(jump?.turn ?? null);
  const [jumpMiss, setJumpMiss] = useState(false);
  const shownSession = useRef<string | undefined>(undefined);
  shownSession.current = chat?.session_id;
  // a subagent's own conversation opens in the transcript viewer, read-only, under this conversation's session
  const onOpenSubagent = useCallback(
    (s: Subagent) => actions.openViewer({ machine: machineId, dir, session: shownSession.current ?? "", subagent: { id: s.id, description: s.description || s.kind || s.id } }),
    [machineId, dir],
  );
  // a link to a turn of a pane's conversation: the address with the pane, the session shown and the turn's first record
  const onLink = useCallback(
    (turn: Turn) => {
      const id = turn.ids?.[0];
      if (!id) return;
      void copyText(turnLink(location.origin, { machineId, paneId, session: shownSession.current ?? "", turn: id })).then((ok) =>
        actions.notify(ok ? "Link copied" : "The link could not be copied", { brief: true, error: !ok }),
      );
    },
    [machineId, paneId],
  );
  // the pins of this folder's conversations: a bookmark on a turn of the session shown, kept per folder so it outlives a compaction
  const [pins, setPins] = usePref<Pin[]>(pinKey(machineId, dir), []);
  const pinnedIds = useMemo(() => new Set(pins.map((p) => p.turn)), [pins]);
  const pinsRef = useRef(pins);
  pinsRef.current = pins;
  const onPin = useCallback(
    (turn: Turn) => {
      const pin = pinOf(turn, shownSession.current ?? "");
      if (!pin) return;
      const was = isPinned(pinsRef.current, pin.turn);
      setPins((prev) => togglePin(prev, pin));
      actions.notify(was ? "Unpinned" : "Pinned — Pins… in the View menu lists it", { brief: true });
    },
    [setPins],
  );
  useEffect(() => {
    if (!jump) return;
    pendingTurn.current = jump.turn || null;
    follow.stick.current = !jump.turn;
    setJumpMiss(false);
    // the session on screen is not refetched: the query blanks the log first; a jump naming no session takes the one shown
    if (jump.session && shownSession.current !== jump.session) setSessionId(jump.session);
  }, [jump]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    const id = pendingTurn.current;
    const log = logRef.current;
    if (!id || !log || !chat || !jump || (jump.session && chat.session_id !== jump.session)) return;
    const el = log.querySelector(`[data-ids~="${CSS.escape(id)}"]`);
    if (!el && hidden > 0 && turns.some((t) => t.ids?.includes(id))) {
      revealAll(); // the turn is loaded but not mounted yet: this effect runs again once it is
      return;
    }
    pendingTurn.current = null;
    actions.clearJump();
    if (!el) {
      setJumpMiss(true);
      follow.stick.current = true;
      return;
    }
    follow.stick.current = false;
    revealTurn(log, el);
    // remove, reflow, add: the CSS animation restarts when the class is still there from a previous jump
    el.classList.remove("turn-hit");
    void (el as HTMLElement).offsetWidth;
    el.classList.add("turn-hit");
    setTimeout(() => el.classList.remove("turn-hit"), 2600);
  }, [chat, jump, hidden]); // eslint-disable-line react-hooks/exhaustive-deps
  // a jump into a session the lens cannot open (a pin whose file is gone): said once, the conversation shown stays
  useEffect(() => {
    if (!jump || !fetchError || !pendingTurn.current || (chat && chat.session_id === jump.session)) return;
    pendingTurn.current = null;
    actions.clearJump();
    actions.notify(`That turn's conversation could not be opened: ${fetchError}`, { error: true });
    if (!readOnly) setSessionId(null);
  }, [fetchError, jump, chat, readOnly]);

  const sessions = chat?.sessions ?? [];
  if (sessionId === null && chat?.session_id) live.current = chat.session_id;
  const older = sessionId !== null && sessionId !== live.current ? sessionId : null;
  const resume = () => {
    if (!older || !workspaceId || !agent) return;
    void act(createTab({ machine: machineId, workspace: workspaceId, name: "", agent, resume: older }).then((created) => actions.selectPane(machineId, created.pane)));
  };
  const keys = (ks: string[]) => sendKeys(machineId, paneId, ks).catch((e) => setActionError(errMsg(e)));
  const context = usage && usage.window > 0 ? usage.context / usage.window : 0;
  const error = actionError ?? (fetchError || null);

  return (
    <div className="chat">
      {sessions.length > 1 && (
        <div className="chat-session-row">
          <Select
            className="chat-session-pick"
            value={sessionId ?? chat?.session_id ?? ""}
            onChange={setSessionId}
            options={sessions.map((s) => ({ value: s.id, label: s.title || s.id.slice(0, 8), hint: whenShort(s.mod_time) }))}
          />
          {older && workspaceId && agent && (
            <button type="button" className="btn sm" title="A new tab in this workspace, the agent started on this conversation" onClick={resume}>
              <Icon name="corner-down-left" size={12} /> Resume in a new tab
            </button>
          )}
        </div>
      )}
      <TimelineBar turns={turns} machineId={machineId} paneId={paneId} session={chat?.session_id} />
      <PlanBar plan={chat?.plan} />
      <div className="chat-log" ref={logRef} onScroll={follow.onScroll}>
        {turns.length === 0 && <div className="empty chat-empty">{chat === null ? "Loading…" : readOnly ? "This transcript is empty." : "No transcript for this pane's folder yet."}</div>}
        {turns.slice(hidden).map((t, i) => (
          <TurnView
            key={t.ids?.[0] || hidden + i}
            turn={t}
            machineId={machineId}
            onOpenFile={onOpenFile}
            onForward={others ? onForward : undefined}
            onLink={readOnly ? undefined : onLink}
            onPin={readOnly ? undefined : onPin}
            pinned={!!t.ids?.[0] && pinnedIds.has(t.ids[0])}
            subagents={chat?.subagents}
            onOpenSubagent={onOpenSubagent}
            diffs
          />
        ))}
      </div>
      {!readOnly && status === "input" && (
        <Notice tone="warn" icon="alert-triangle" className="chat-notice">
          <QuestionCard machineId={machineId} paneId={paneId} active={active} onKeys={keys}>
            The agent is waiting for an answer — check the terminal, or:
          </QuestionCard>
        </Notice>
      )}
      {error && (
        <Notice tone="error" className="chat-notice" onDismiss={() => setActionError(null)}>
          {error}
        </Notice>
      )}
      {!readOnly && context >= 0.8 && status !== "run" && (
        <Notice
          tone="warn"
          icon="chart"
          className={`chat-notice${context >= 0.9 ? " is-red" : ""}`}
          action={
            <button type="button" className="linkish" onClick={() => void submitPrompt(machineId, paneId, "/compact").catch((e) => setActionError(errMsg(e)))}>
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
