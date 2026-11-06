// SubagentRow is one agent a turn spawned, drawn under the call: its state, its description, the call it is on or the
// first line of its report, its tool calls, and the way to its own conversation.

import { plural } from "@/lib/text";
import { elapsed, spent } from "@/lib/time";
import { StatusDot } from "@/ui";
import type { Subagent } from "./api";
import { subagentLine } from "./subagents";
import { t } from "@/lib/i18n";

const DOTS: Record<Subagent["status"], string> = { working: "run", quiet: "idle", done: "done" };

export function SubagentRow({ subagent: s, onOpen }: { subagent: Subagent; onOpen?: (s: Subagent) => void }) {
  const state = s.status === "working" ? t("working") : s.status === "quiet" ? t("quiet for {time}", { time: elapsed(s.ended ?? Date.now()) }) : s.took ? t("done in {time}", { time: spent(s.took) }) : t("done");
  const words = subagentLine(s) || state;
  const title = [s.kind, s.model, s.branch ? t("branch {branch}", { branch: s.branch }) : "", state, plural(s.tools, "tool call"), s.failed ? t("{n} failed", { n: s.failed }) : ""].filter(Boolean).join(" · ");
  return (
    <div className={`subagent-row is-${s.status}`} title={title}>
      <StatusDot status={DOTS[s.status] ?? "idle"} />
      <span className="subagent-desc">{s.description || s.kind || s.id}</span>
      <span className="subagent-line">{words}</span>
      <span className="badge quiet subagent-count">
        {s.tools}
        {s.failed ? ` · ${t("{n} failed", { n: s.failed })}` : ""}
      </span>
      {onOpen && (
        <button type="button" className="linkish subagent-open" title={t("This agent's own conversation, read-only")} onClick={() => onOpen(s)}>
          {t("Open")}
        </button>
      )}
    </div>
  );
}
