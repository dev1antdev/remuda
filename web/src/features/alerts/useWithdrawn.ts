// Closes the notifications the roster says were taken back, as it moves and once as the page opens. The Mac app takes
// its own down over its socket, and a viewer link's screen gets no push to close.

import { useEffect } from "react";
import { useRoster } from "@/features/roster/useRoster";
import { inShell } from "@/lib/shell";
import { closeWithdrawn } from "./withdraw";

export function useWithdrawn(): void {
  const { withdrawn, viewOnly } = useRoster();
  const tags = (withdrawn ?? []).join("\n");
  useEffect(() => {
    if (tags && !viewOnly && !inShell()) void closeWithdrawn(tags.split("\n"));
  }, [tags, viewOnly]);
}
