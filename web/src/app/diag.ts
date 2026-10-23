// The page's own diagnostics: every stall of the main thread goes to the console with where the page was — the
// mode, the lens, the pane, how many wall tiles — and a stall the person felt becomes a toast pointing there.

import { stallWords, toastDue, watchStalls } from "@/lib/perf";
import { plural } from "@/lib/text";
import { actions, app } from "./store";

const MODE_NAMES = { herdr: "Agents", wall: "the Wall", tasks: "Tasks", code: "Code" } as const;

// wherePage says where the page is right now, in the words a person would use; the pane shown is the address bar's (url.ts keeps it)
export function wherePage(): string {
  const { mode, lens } = app.get();
  if (mode === "wall") return `on the Wall (${plural(document.querySelectorAll(".tile").length, "tile")})`;
  const url = new URLSearchParams(location.search);
  const pane = url.get("pane") ? `, ${url.get("machine") ?? "local"} ${url.get("pane")}` : "";
  return mode === "herdr" ? `in Agents, ${lens} lens${pane}` : `in ${MODE_NAMES[mode]}`;
}

// startDiagnostics begins the watch; the console line carries the stall for a bug report, the toast says a long one happened
export function startDiagnostics(): () => void {
  let lastToast = 0;
  return watchStalls(wherePage, (stall) => {
    console.warn(`[remuda] ${stallWords(stall)}`, stall);
    if (!toastDue(stall, lastToast)) return;
    lastToast = stall.at;
    actions.notify(`${stallWords(stall)} — the console has the details`, { error: true });
  });
}
