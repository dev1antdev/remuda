// SubagentRow is one agent a turn spawned, drawn under the call: its state, its description, the call it is on or the
// first line of its report, its tool calls, and the way to its own conversation.

import { plural } from "@/lib/text";
import { elapsed, spent } from "@/lib/time";
import { StatusDot } from "@/ui";
import type { Subagent } from "./api";
import { subagentLine } from "./subagents";

const DOTS: Record<Subagent["status"], string> = { working: "run", quiet: "idle", done: "done" };

export function SubagentRow({ subagent: s, onOpen }: { subagent: Subagent; onOpen?: (s: Subagent) => void }) {
  const state = s.status === "working" ? "working" : s.status === "quiet" ? `quiet for ${elapsed(s.ended ?? Date.now())}` : s.took ? `done in ${spent(s.took)}` : "done";
  const words = subagentLine(s) || state;
  const title = [s.kind, s.model, s.branch ? `branch ${s.branch}` : "", state, plural(s.tools, "tool call"), s.failed ? `${s.failed} failed` : ""].filter(Boolean).join(" · ");
  return (
    <div className={`subagent-row is-${s.status}`} title={title}>
      <StatusDot status={DOTS[s.status] ?? "idle"} />
      <span className="subagent-desc">{s.description || s.kind || s.id}</span>
      <span className="subagent-line">{words}</span>
      <span className="badge quiet subagent-count">
        {s.tools}
        {s.failed ? ` · ${s.failed} failed` : ""}
      </span>
      {onOpen && (
        <button type="button" className="linkish subagent-open" title="This agent's own conversation, read-only" onClick={() => onOpen(s)}>
          Open
        </button>
      )}
    </div>
  );
}
