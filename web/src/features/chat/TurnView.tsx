// One turn of the transcript: its text as Markdown, its thinking and tool calls folded (each call with how long it took, a
// failed one in red), an Edit's change under its call in the lens, a picture the agent read or wrote as a thumbnail, a foot
// with what the answer spent and what went wrong, a forward button, a link to the turn and a pin that keeps it in the Pins list.

import { memo, useState, type ReactNode } from "react";
import { fileUrl } from "@/features/files/api";
import type { OpenFile } from "@/features/files/target";
import { tok, usd } from "@/features/usage/format";
import { basename } from "@/lib/text";
import { spent, took } from "@/lib/time";
import { ErrorBoundary, Icon, Notice, Tip } from "@/ui";
import { Markdown } from "@/ui/Markdown";
import type { Part, Subagent, Turn } from "./api";
import { callStats, callWords } from "./calls";
import { EDIT_CHANGE_TOOLS, EditChange } from "./EditChange";
import { editsOf } from "./editDiff";
import { EDIT_TOOLS, imagePath, toolLabel } from "./footprint";
import { PLAN_TOOLS, planCallWords } from "./plan";
import { PLAN_MARK } from "./PlanBar";
import { SubagentRow } from "./SubagentRow";
import { sameSubagents, subagentsOf } from "./subagents";
import { t } from "@/lib/i18n";

// TurnView re-renders only when the turn's content changed, so a selection in the log survives the polls
export const TurnView = memo(
  function TurnView({ turn, machineId, onOpenFile, onForward, onLink, onPin, pinned, subagents, onOpenSubagent, diffs }: {
    turn: Turn;
    machineId?: string;
    onOpenFile: OpenFile;
    onForward?: (text: string) => void;
    onLink?: (turn: Turn) => void; // copies a link to this turn; only a pane's conversation has one
    onPin?: (turn: Turn) => void; // pins or unpins this turn; only a pane's conversation has one
    pinned?: boolean; // the turn is in the Pins list: a mark in its corner, the pin button on
    subagents?: Subagent[]; // the conversation's, drawn under the Agent calls that spawned them
    onOpenSubagent?: (s: Subagent) => void;
    diffs?: boolean; // an Edit's change drawn under its call: the lens, not a tile
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
        <ErrorBoundary fallback={(message) => <Notice tone="error">{t("This turn could not be drawn: {message}", { message })}</Notice>}>
          {turn.parts.map((p, i) => (
            <PartView key={i} part={p} machineId={machineId} onOpenFile={onOpenFile} subagents={subagents} onOpenSubagent={onOpenSubagent} diffs={diffs} />
          ))}
        </ErrorBoundary>
        {turn.role === "assistant" && <Spent turn={turn} />}
        {(!!(onForward && text && turn.role === "assistant") || !!((onLink || onPin) && turn.ids?.length)) && (
          <div className="turn-actions">
            {onForward && text && turn.role === "assistant" && (
              <button type="button" className="linkish turn-forward" title={t("Forward this to another agent")} onClick={() => onForward(text)}>
                <Icon name="send" size={12} /> forward
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
    sameTurn(a.turn, b.turn) &&
    sameSubagents(subagentsOf(a.turn, a.subagents), subagentsOf(b.turn, b.subagents)),
);

// sameTurn compares content, as every poll brings new objects; a tool's input never changes once written, so it is skipped
export function sameTurn(a: Turn, b: Turn): boolean {
  if (a.role !== b.role || a.ts !== b.ts || a.parts.length !== b.parts.length || a.ids?.length !== b.ids?.length) return false;
  if (a.took !== b.took || a.tokens !== b.tokens || a.cost !== b.cost) return false;
  for (let i = 0; i < a.parts.length; i++) {
    const p = a.parts[i]!;
    const q = b.parts[i]!;
    if (p.type !== q.type || p.text !== q.text || p.output !== q.output || p.is_error !== q.is_error || p.tool_name !== q.tool_name || p.took !== q.took) return false;
  }
  return true;
}

// Spent is the quiet foot of an answer: how long it took, the tokens it used, what it cost at API prices, and what
// went wrong among its calls — the failed ones, the ones repeating an earlier call exactly. Nothing when there is nothing to say.
function Spent({ turn }: { turn: Turn }) {
  const words = [turn.took ? spent(turn.took) : "", turn.tokens ? t("{n} tokens", { n: tok(turn.tokens) }) : "", turn.cost ? usd(turn.cost) : ""].filter(Boolean);
  const wrong = callWords(callStats(turn));
  if (!words.length && !wrong) return null;
  const title = `${turn.model ? `${turn.model} · ` : ""}from your prompt to the end of this answer; the tokens of its API responses, cache included, at API prices — on a subscription a measure, not a bill${wrong ? t("; a repeated call is one whose tool and input match an earlier call of this answer exactly") : ""}`;
  return (
    <span className="turn-spent" title={title}>
      {words.join(" · ")}
      {wrong && <span className="turn-wrong">{words.length ? ` · ${wrong}` : wrong}</span>}
    </span>
  );
}

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

function PartView({ part, machineId, onOpenFile, subagents, onOpenSubagent, diffs }: { part: Part; machineId?: string; onOpenFile: OpenFile; subagents?: Subagent[]; onOpenSubagent?: (s: Subagent) => void; diffs?: boolean }) {
  if (part.type === "text") {
    return <Markdown text={part.text ?? ""} onOpenFile={onOpenFile} />;
  }
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
  if (part.type === "tool" && PLAN_TOOLS.has(part.tool_name ?? "")) {
    // a plan tool's call is one line: the item it added or moved, not its JSON
    const { words, mark } = planCallWords(part);
    return (
      <div className={part.is_error ? "part-plan part-tool-error" : "part-plan"} title={part.is_error ? part.output : words}>
        <Icon name={mark === "list" ? "list" : PLAN_MARK[mark]} size={12} className="part-tool-icon" /> {words}
      </div>
    );
  }
  if (part.type === "tool") {
    const { name, hint, file } = toolLabel(part);
    const picture = machineId ? imagePath(part) : null;
    const spawned = part.tool_id && subagents ? subagents.filter((s) => s.tool_id === part.tool_id) : [];
    const diffed = !!diffs && EDIT_CHANGE_TOOLS.has(name);
    const everywhere = diffed && editsOf(name, part.input).some((edit) => edit.replaceAll);
    return (
      <>
        <Folded
          className={part.is_error ? "part-tool part-tool-error" : "part-tool"}
          summary={
            <>
              <Icon name={part.is_error ? "alert-triangle" : "chevron-right"} size={12} className="part-tool-icon" /> {name}
              {hint && " · "}
              {file ? (
                <button
                  type="button"
                  className="path-link"
                  onClick={(e) => {
                    e.preventDefault();
                    onOpenFile({ path: file, prefer: EDIT_TOOLS.has(name) ? "changes" : "file" });
                  }}
                  title={EDIT_TOOLS.has(name) ? t("Open the file with its changes") : t("Open the file")}
                >
                  {hint}
                </button>
              ) : (
                <span className="part-hint">{hint}</span>
              )}
              {everywhere && <span className="part-hint"> {t("· every occurrence")}</span>}
              {part.took ? (
                <span className="part-took" title={part.is_error ? t("The call failed, and how long it took") : t("How long the call took")}>
                  {took(part.took)}
                </span>
              ) : null}
            </>
          }
        >
          {part.input !== undefined && <pre className="tool-io">{pretty(part.input)}</pre>}
          {part.output && <pre className="tool-io">{part.output}</pre>}
        </Folded>
        {diffed && <EditChange part={part} />}
        {spawned.map((s) => (
          <SubagentRow key={s.id} subagent={s} onOpen={onOpenSubagent} />
        ))}
        {picture && <Thumb machineId={machineId!} path={picture} onOpenFile={onOpenFile} />}
      </>
    );
  }
  return null;
}

// Thumb is the picture a Read or Write was about, drawn under the call from the machine's own file; a file gone since is nothing
function Thumb({ machineId, path, onOpenFile }: { machineId: string; path: string; onOpenFile: OpenFile }) {
  const [gone, setGone] = useState(false);
  if (gone) return null;
  return (
    <button type="button" className="linkish part-image" title={`${path}\n${t("Open the file")}`} onClick={() => onOpenFile({ path, prefer: "file" })}>
      <img src={fileUrl(machineId, path)} alt={basename(path)} loading="lazy" onError={() => setGone(true)} />
    </button>
  );
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
