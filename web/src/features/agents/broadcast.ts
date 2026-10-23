// One text to several agents: which agents a quick pick means, and the sends one after another with what each
// answered — an agent busy with a question refuses and is reported, nothing is queued. Pure but for the send given.

import { targetKey, type AgentTarget } from "./targets";

export type QuickPick = "all" | "working" | "resting" | "none";

// picked is the set of target keys a quick pick ticks: an agent asking a question is never picked, it would refuse
export function picked(targets: AgentTarget[], pick: QuickPick): Set<string> {
  const keep = (target: AgentTarget) => {
    if (target.status === "input") return false;
    if (pick === "all") return true;
    if (pick === "working") return target.status === "run";
    if (pick === "resting") return target.status !== "run";
    return false;
  };
  return new Set(targets.filter(keep).map(targetKey));
}

export type Outcome = { target: AgentTarget; error?: string };

// broadcast sends the text to each target in turn and says how each went; a refusal stops nothing
export async function broadcast(targets: AgentTarget[], text: string, send: (target: AgentTarget, text: string) => Promise<void>): Promise<Outcome[]> {
  const out: Outcome[] = [];
  for (const target of targets) {
    try {
      await send(target, text);
      out.push({ target });
    } catch (e) {
      out.push({ target, error: e instanceof Error ? e.message : String(e) });
    }
  }
  return out;
}

// statusWords is the status as the picker says it
export function statusWords(status: string): string {
  switch (status) {
    case "run":
      return "working";
    case "input":
      return "asking — would refuse";
    case "done":
      return "done";
    default:
      return "idle";
  }
}
