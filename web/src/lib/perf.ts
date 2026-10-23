// The page watching itself: a long task — the main thread held for a stretch, nothing clickable meanwhile — is
// reported with where the page was, so a stall can be traced to its mode and pane instead of guessed at. The
// browser's PerformanceObserver on "longtask" entries; the words and the toast rule are pure and tested.

export type Stall = { ms: number; where: string; at: number };

export const STALL_LOGGED = 250; // ms: a long task worth a line in the console
export const STALL_TOASTED = 2000; // ms: a stall the person felt, worth a toast
export const TOAST_SPACING = 60_000; // ms: one toast a minute at most

// stallWords is the line: "The page stalled 4.2 s on the Wall (13 tiles)"
export function stallWords(stall: Stall): string {
  const seconds = stall.ms >= 10_000 ? `${Math.round(stall.ms / 1000)} s` : `${(stall.ms / 1000).toFixed(1)} s`;
  return `The page stalled ${seconds} ${stall.where}`.trimEnd();
}

// toastDue says whether a stall gets a toast: long enough, and not within a minute of the last one
export function toastDue(stall: Stall, lastToast: number): boolean {
  return stall.ms >= STALL_TOASTED && stall.at - lastToast >= TOAST_SPACING;
}

// watchStalls reports every long task from STALL_LOGGED up with where the page is; the browser decides what a long task is (50 ms and up)
export function watchStalls(where: () => string, report: (stall: Stall) => void): () => void {
  if (typeof PerformanceObserver === "undefined" || !PerformanceObserver.supportedEntryTypes?.includes("longtask")) return () => {};
  const observer = new PerformanceObserver((list) => {
    for (const entry of list.getEntries()) {
      if (entry.duration < STALL_LOGGED) continue;
      report({ ms: Math.round(entry.duration), where: where(), at: Date.now() });
    }
  });
  observer.observe({ type: "longtask", buffered: true });
  return () => observer.disconnect();
}
