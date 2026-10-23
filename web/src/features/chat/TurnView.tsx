// One turn of the transcript: its text as Markdown, its thinking and tool calls folded, a forward button.

import { memo, useState, type ReactNode } from "react";
import type { OpenFile } from "@/features/files/target";
import { tok, usd } from "@/features/usage/format";
import { spent } from "@/lib/time";
import { ErrorBoundary, Icon, Notice } from "@/ui";
import { Markdown } from "@/ui/Markdown";
import type { Part, Turn } from "./api";
import { EDIT_TOOLS, toolLabel } from "./footprint";

// TurnView re-renders only when the turn's content changed, so a selection in the log survives the polls
export const TurnView = memo(
  function TurnView({ turn, onOpenFile, onForward }: { turn: Turn; onOpenFile: OpenFile; onForward?: (text: string) => void }) {
    const cls = turn.role === "user" ? "turn turn-user" : "turn turn-assistant";
    const text = turn.parts
      .filter((p) => p.type === "text")
      .map((p) => p.text ?? "")
      .join("\n\n")
      .trim();
    return (
      <div className={cls} data-ids={turn.ids?.join(" ")}>
        <ErrorBoundary fallback={(message) => <Notice tone="error">This turn could not be drawn: {message}</Notice>}>
          {turn.parts.map((p, i) => (
            <PartView key={i} part={p} onOpenFile={onOpenFile} />
          ))}
        </ErrorBoundary>
        {turn.role === "assistant" && (turn.took || turn.tokens) ? <Spent turn={turn} /> : null}
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
export function sameTurn(a: Turn, b: Turn): boolean {
  if (a.role !== b.role || a.ts !== b.ts || a.parts.length !== b.parts.length || a.ids?.length !== b.ids?.length) return false;
  if (a.took !== b.took || a.tokens !== b.tokens || a.cost !== b.cost) return false;
  for (let i = 0; i < a.parts.length; i++) {
    const p = a.parts[i]!;
    const q = b.parts[i]!;
    if (p.type !== q.type || p.text !== q.text || p.output !== q.output || p.is_error !== q.is_error || p.tool_name !== q.tool_name) return false;
  }
  return true;
}

// Spent is the quiet foot of an answer: how long it took, the tokens it used, what it cost at API prices.
function Spent({ turn }: { turn: Turn }) {
  const words = [turn.took ? spent(turn.took) : "", turn.tokens ? `${tok(turn.tokens)} tokens` : "", turn.cost ? usd(turn.cost) : ""].filter(Boolean);
  const title = `${turn.model ? `${turn.model} · ` : ""}from your prompt to the end of this answer; the tokens of its API responses, cache included, at API prices — on a subscription a measure, not a bill`;
  return (
    <span className="turn-spent" title={title}>
      {words.join(" · ")}
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

function PartView({ part, onOpenFile }: { part: Part; onOpenFile: OpenFile }) {
  if (part.type === "text") {
    return <Markdown text={part.text ?? ""} onOpenFile={onOpenFile} />;
  }
  if (part.type === "thinking") {
    return (
      <Folded
        className="part-thinking"
        summary={
          <>
            <Icon name="sparkles" size={12} className="part-tool-icon" /> Thinking
          </>
        }
      >
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
