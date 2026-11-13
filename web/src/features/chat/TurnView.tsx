// One turn of the transcript: its text as Markdown, its thinking folded, every run of its tool calls one card (ToolCard),
// a message the person sent while the agent worked in its place, marked so, a foot with what the answer spent and what
// went wrong, a forward button, Undo this turn for an answer whose changes the cockpit kept, a link to the turn and a pin
// that keeps it in the Pins list.

import { memo } from "react";
import type { OpenFile } from "@/features/files/target";
import { tok, tokFine, usd } from "@/features/usage/format";
import { spent, whenShort } from "@/lib/time";
import { ErrorBoundary, Icon, Notice, Tip } from "@/ui";
import { Markdown } from "@/ui/Markdown";
import type { Part, Subagent, Turn } from "./api";
import { callStats, callWords } from "./calls";
import { sameSubagents, subagentsOf } from "./subagents";
import { Folded, ToolCard } from "./ToolCard";
import { pieces } from "./toolRuns";
import { t } from "@/lib/i18n";

// TurnView re-renders only when the turn's content changed, so a selection in the log survives the polls
export const TurnView = memo(
  function TurnView({ turn, machineId, onOpenFile, onForward, onLink, onPin, pinned, subagents, onOpenSubagent, diffs, undo, onUndo, live }: {
    turn: Turn;
    machineId?: string;
    onOpenFile: OpenFile;
    onForward?: (text: string) => void;
    onLink?: (turn: Turn) => void; // copies a link to this turn; only a pane's conversation has one
    onPin?: (turn: Turn) => void; // pins or unpins this turn; only a pane's conversation has one
    pinned?: boolean; // the turn is in the Pins list: a mark in its corner, the pin button on
    subagents?: Subagent[]; // the conversation's, drawn under the Agent calls that spawned them
    onOpenSubagent?: (s: Subagent) => void;
    diffs?: boolean; // an Edit's change drawn under its call, a mermaid fence drawn as its diagram: the lens, not a tile
    undo?: string; // the kept turn of the folder this answer is, which its foot offers to undo
    onUndo?: (turn: string) => void;
    live?: boolean; // the turn under way: a call without its result is still running
  }) {
    const cls = `turn ${turn.role === "user" ? "turn-user" : "turn-assistant"}${pinned ? " turn-pinned" : ""}`;
    const text = turn.parts
      .filter((p) => p.type === "text")
      .map((p) => p.text ?? "")
      .join("\n\n")
      .trim();
    return (
      <div className={cls} data-ids={turn.ids?.join(" ")}>
        {pinned && (
          <span className="turn-pin-mark" title={t("Pinned")}>
            <Icon name="pin" size={12} />
          </span>
        )}
        <ErrorBoundary resetKey={turn} fallback={(message) => <Notice tone="error">{t("This turn could not be drawn: {message}", { message })}</Notice>}>
          {pieces(turn.parts).map((piece) =>
            piece.kind === "card" ? (
              <ToolCard key={piece.items[0]!.index} items={piece.items} machineId={machineId} onOpenFile={onOpenFile} subagents={subagents} onOpenSubagent={onOpenSubagent} diffs={diffs} live={live} />
            ) : (
              <PartView key={piece.item.index} part={piece.item.part} onOpenFile={onOpenFile} diffs={diffs} />
            ),
          )}
        </ErrorBoundary>
        {turn.role === "assistant" && <Spent turn={turn} />}
        {(!!(onForward && text && turn.role === "assistant") || !!((onLink || onPin) && turn.ids?.length) || !!(undo && onUndo)) && (
          <div className="turn-actions">
            {onForward && text && turn.role === "assistant" && (
              <button type="button" className="linkish turn-forward" title={t("Forward this to another agent")} onClick={() => onForward(text)}>
                <Icon name="send" size={12} /> {t("forward")}
              </button>
            )}
            {undo && onUndo && (
              <button type="button" className="linkish turn-forward turn-undo" title={t("Put the files back as they were before this turn; you see what changes first")} onClick={() => onUndo(undo)}>
                <Icon name="undo" size={12} /> {t("Undo this turn")}
              </button>
            )}
            {onLink && !!turn.ids?.length && (
              <Tip text={t("Copy a link that opens this conversation at this turn")}>
                <button type="button" className="icon-btn sm turn-link" aria-label={t("Copy link to this turn")} onClick={() => onLink(turn)}>
                  <Icon name="link" size={12} />
                </button>
              </Tip>
            )}
            {onPin && !!turn.ids?.length && (
              <Tip text={pinned ? t("Unpin this turn") : t("Pin this turn: the Pins list in the View menu finds it again")}>
                <button type="button" className={pinned ? "icon-btn sm turn-pin on" : "icon-btn sm turn-pin"} aria-label={pinned ? t("Unpin this turn") : t("Pin this turn")} aria-pressed={!!pinned} onClick={() => onPin(turn)}>
                  <Icon name="pin" size={12} />
                </button>
              </Tip>
            )}
          </div>
        )}
      </div>
    );
  },
  (a, b) =>
    a.machineId === b.machineId &&
    a.onOpenFile === b.onOpenFile &&
    a.onForward === b.onForward &&
    a.onLink === b.onLink &&
    a.onPin === b.onPin &&
    a.pinned === b.pinned &&
    a.onOpenSubagent === b.onOpenSubagent &&
    a.diffs === b.diffs &&
    a.undo === b.undo &&
    a.onUndo === b.onUndo &&
    a.live === b.live &&
    sameTurn(a.turn, b.turn) &&
    sameSubagents(subagentsOf(a.turn, a.subagents), subagentsOf(b.turn, b.subagents)),
);

// sameTurn compares content, as every poll brings new objects; a tool's input never changes once written, so it is skipped
export function sameTurn(a: Turn, b: Turn): boolean {
  if (a.role !== b.role || a.ts !== b.ts || a.parts.length !== b.parts.length || a.ids?.length !== b.ids?.length) return false;
  if (a.took !== b.took || a.tokens !== b.tokens || a.output !== b.output || a.cost !== b.cost) return false;
  for (let i = 0; i < a.parts.length; i++) {
    const p = a.parts[i]!;
    const q = b.parts[i]!;
    if (p.type !== q.type || p.text !== q.text || p.output !== q.output || p.is_error !== q.is_error || p.tool_name !== q.tool_name || p.took !== q.took || p.from !== q.from) return false;
  }
  return true;
}

// Spent is the quiet foot of an answer: how long it took, the tokens it wrote (↓, as the CLI counts them) and used in all,
// what it cost at API prices, and what went wrong among its calls — the failed ones, the ones repeating an earlier call
// exactly. Nothing when there is nothing to say.
function Spent({ turn }: { turn: Turn }) {
  const words = [turn.took ? spent(turn.took) : "", turn.output ? `↓ ${tokFine(turn.output)}` : "", turn.tokens ? t("{n} tokens", { n: tok(turn.tokens) }) : "", turn.cost ? usd(turn.cost) : ""].filter(Boolean);
  const wrong = callWords(callStats(turn));
  if (!words.length && !wrong) return null;
  const title = `${turn.model ? `${turn.model} · ` : ""}${t("from your prompt to the end of this answer; ↓ the tokens it wrote, as the CLI counts them; the tokens of its API responses, cache included, at API prices — on a subscription a measure, not a bill")}${wrong ? t("; a repeated call is one whose tool and input match an earlier call of this answer exactly") : ""}`;
  return (
    <span className="turn-spent" title={title}>
      {words.join(" · ")}
      {wrong && <span className="turn-wrong">{words.length ? ` · ${wrong}` : wrong}</span>}
    </span>
  );
}

function PartView({ part, onOpenFile, diffs }: { part: Part; onOpenFile: OpenFile; diffs?: boolean }) {
  if (part.type === "text") {
    return <Markdown text={part.text ?? ""} onOpenFile={onOpenFile} diagrams={diffs} />;
  }
  if (part.type === "queued") return <SentPart part={part} onOpenFile={onOpenFile} />;
  if (part.type === "thinking") {
    return (
      <Folded
        className="part-thinking"
        summary={
          <>
            <Icon name="sparkles" size={12} className="part-tool-icon" /> {t("Thinking")}
          </>
        }
      >
        <div className="part-text">{part.text}</div>
      </Folded>
    );
  }
  return null;
}

// SentPart is a message sent to the agent while it worked, the person's or another agent's, where it reached the agent
function SentPart({ part, onOpenFile }: { part: Part; onOpenFile: OpenFile }) {
  const who = part.from ? t("From {agent}, while it worked", { agent: part.from }) : t("Sent while it worked");
  return (
    <div className={part.from ? "part-sent is-peer" : "part-sent"}>
      <div className="part-sent-mark">
        <Icon name="message" size={12} /> {who}
        {part.at && ` · ${whenShort(part.at)}`}
      </div>
      <Markdown text={part.text ?? ""} onOpenFile={onOpenFile} />
    </div>
  );
}
