// What a chat read means when it comes back thinner than the one before: a live session file never loses its turns, so
// an answer that parsed to none for the same session is a read caught while the file was written, not an empty chat.

import type { ChatResponse } from "./api";

// keepsTurns says the turns on screen stay and the read is asked again: the answer is the live pane's (not an older
// session or a subagent), it holds nothing, and the same session had turns a moment ago.
export function keepsTurns(previous: ChatResponse | null, next: ChatResponse, sessionId: string | null, subagent?: string): boolean {
  return !sessionId && !subagent && next.turns.length === 0 && !!previous && previous.session_id === next.session_id && previous.turns.length > 0;
}
