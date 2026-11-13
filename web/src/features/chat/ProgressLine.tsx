// ProgressLine is the turn under way, over the composer: what Claude Code's own spinner says — "Refactoring… 2 min 14 s ·
// ↓ 6.8k" — off the chat's read of the pane's screen, its clock ticking between reads; "Working…" and the time since the
// prompt where the screen names nothing (another agent, a spinner out of sight).

import { useNow } from "@/lib/clock";
import { tc } from "@/lib/i18n";
import { spinnerSeconds, spinnerWords, type LiveRead } from "./progress";

export function ProgressLine({ live }: { live: LiveRead }) {
  const words = useSpinner(live);
  return (
    <div className="chat-progress" role="status">
      <span className="tool-spin" aria-hidden />
      <span className="chat-progress-words">{words}</span>
    </div>
  );
}

// useSpinner is the spinner's words a second at a time
export function useSpinner(live: LiveRead): string {
  const now = useNow(1000);
  return spinnerWords(live.progress?.verb ?? tc("progress", "Working"), spinnerSeconds(live.progress, live.readAt, now, live.since || undefined), live.progress?.tokens);
}
