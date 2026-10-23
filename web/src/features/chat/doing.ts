// What the agent is doing this second, read from the end of its conversation: the tool call still without a
// result, else thinking before the next step, else writing its answer. A wall tile shows it while the agent works.

import type { Turn } from "./api";
import { toolLabel } from "./footprint";

export type Doing = { what: "tool"; name: string; hint: string } | { what: "thinking" | "writing" };

export function doing(turns: Turn[]): Doing {
  const last = turns[turns.length - 1];
  if (!last || last.role === "user") return { what: "thinking" };
  const part = last.parts[last.parts.length - 1];
  if (!part) return { what: "thinking" };
  if (part.type === "tool") {
    if (part.output !== undefined) return { what: "thinking" };
    const { name, hint } = toolLabel(part);
    return { what: "tool", name, hint };
  }
  return { what: part.type === "thinking" ? "thinking" : "writing" };
}
