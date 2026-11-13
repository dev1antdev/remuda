// ToolCard is a run of a turn's tool calls as one bordered card, a 30 px line a call: ✓ done, ✗ failed, a spinner while it
// runs, the tool, what it was about (dim, a file a link), how long it took, a chevron that opens its input and result. An
// Edit's change, a picture the call read or wrote and the subagents an Agent call spawned stay under their line, inside
// the card; the thinking between two calls is a line of its own. After Orchi's chat.

import { useState, type ReactNode } from "react";
import { fileUrl } from "@/features/files/api";
import type { OpenFile } from "@/features/files/target";
import { useNow } from "@/lib/clock";
import { t } from "@/lib/i18n";
import { basename } from "@/lib/text";
import { took } from "@/lib/time";
import { Icon } from "@/ui";
import type { Part, Subagent } from "./api";
import { EDIT_CHANGE_TOOLS, EditChange } from "./EditChange";
import { editsOf } from "./editDiff";
import { EDIT_TOOLS, imagePath, toolLabel } from "./footprint";
import { PLAN_TOOLS, planCallWords } from "./plan";
import { PLAN_MARK } from "./PlanBar";
import { SubagentRow } from "./SubagentRow";
import { callState, type CallState, type Indexed } from "./toolRuns";

export type CallsProps = {
  machineId?: string;
  onOpenFile: OpenFile;
  subagents?: Subagent[];
  onOpenSubagent?: (s: Subagent) => void;
  diffs?: boolean; // an Edit's change under its line: the lens, not a tile
  live?: boolean; // the turn under way: a call without its result is running
};

export function ToolCard({ items, ...props }: CallsProps & { items: Indexed[] }) {
  return (
    <div className="tool-card">
      {items.map(({ part, index }) =>
        part.type === "thinking" ? (
          <Folded
            key={index}
            className="tool-row is-thinking"
            summary={
              <>
                <span className="tool-mark">
                  <Icon name="sparkles" size={12} />
                </span>
                <span className="tool-name">{t("Thinking")}</span>
                <span className="tool-target" />
                <Icon name="chevron-right" size={12} className="tool-chevron" />
              </>
            }
          >
            <div className="part-text tool-thought">{part.text}</div>
          </Folded>
        ) : PLAN_TOOLS.has(part.tool_name ?? "") ? (
          <PlanLine key={index} part={part} />
        ) : (
          <CallLine key={index} part={part} {...props} />
        ),
      )}
    </div>
  );
}

// PlanLine is a plan tool's call, one line: the item it added or moved, not its JSON
function PlanLine({ part }: { part: Part }) {
  const { words, mark } = planCallWords(part);
  return (
    <div className={part.is_error ? "tool-plan is-failed" : "tool-plan"} title={part.is_error ? part.output : words}>
      <span className="tool-mark">
        <Icon name={mark === "list" ? "list" : PLAN_MARK[mark]} size={12} />
      </span>
      <span className="tool-target">{words}</span>
    </div>
  );
}

function CallLine({ part, machineId, onOpenFile, subagents, onOpenSubagent, diffs, live = false }: CallsProps & { part: Part }) {
  const { name, hint, file } = toolLabel(part);
  const state = callState(part, live);
  const picture = machineId ? imagePath(part) : null;
  const spawned = part.tool_id && subagents ? subagents.filter((s) => s.tool_id === part.tool_id) : [];
  const diffed = !!diffs && EDIT_CHANGE_TOOLS.has(name);
  const everywhere = diffed && editsOf(name, part.input).some((edit) => edit.replaceAll);
  return (
    <div className={`tool-call is-${state}`}>
      <Folded
        className="tool-row"
        summary={
          <>
            <StateMark state={state} />
            <span className="tool-name">{name}</span>
            {file ? (
              <button
                type="button"
                className="path-link tool-target"
                onClick={(e) => {
                  e.preventDefault();
                  onOpenFile({ path: file, prefer: EDIT_TOOLS.has(name) ? "changes" : "file" });
                }}
                title={`${file}\n${EDIT_TOOLS.has(name) ? t("Open the file with its changes") : t("Open the file")}`}
              >
                <bdi dir="ltr">{hint}</bdi>
              </button>
            ) : (
              <span className="tool-target" title={hint}>
                {hint}
              </span>
            )}
            {everywhere && <span className="tool-every">{t("· every occurrence")}</span>}
            <span className="tool-took" title={state === "failed" ? t("The call failed, and how long it took") : t("How long the call took")}>
              {state === "running" && part.at ? <Elapsed since={part.at} /> : part.took ? took(part.took) : ""}
            </span>
            <Icon name="chevron-right" size={12} className="tool-chevron" />
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
    </div>
  );
}

const stateWords = (state: CallState) => ({ done: t("Done"), failed: t("Failed"), running: t("Running"), unanswered: t("No result") })[state];

function StateMark({ state }: { state: CallState }) {
  return (
    <span className={`tool-mark is-${state}`} role="img" aria-label={stateWords(state)}>
      {state === "done" ? <Icon name="check" size={14} /> : state === "failed" ? <Icon name="x" size={14} /> : state === "running" ? <span className="tool-spin" /> : <span className="tool-dot" />}
    </span>
  );
}

// Elapsed counts how long a running call has run, a second at a time
function Elapsed({ since }: { since: string }) {
  const now = useNow(1000);
  const seconds = (now - new Date(since).getTime()) / 1000;
  return Number.isFinite(seconds) ? <>{took(Math.max(0, seconds))}</> : null;
}

// Folded renders its body only once opened: thinking and tool I/O can be kilobytes a turn
export function Folded({ className, summary, children }: { className: string; summary: ReactNode; children: ReactNode }) {
  const [open, setOpen] = useState(false);
  return (
    <details className={className} open={open} onToggle={(e) => setOpen((e.target as HTMLDetailsElement).open)}>
      <summary>{summary}</summary>
      {open && children}
    </details>
  );
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
