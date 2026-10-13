import { useMemo } from "react";
import { useRoster } from "@/features/roster/useRoster";
import { agentTargets, type AgentTarget } from "./targets";

// useAgentTargets is every running agent as a pick, recomputed only when the roster changes.
export function useAgentTargets(): AgentTarget[] {
  const { machines } = useRoster();
  return useMemo(() => agentTargets(machines), [machines]);
}
