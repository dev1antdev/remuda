// What the person sent from this device, newest first, so ↑ in an empty composer brings it back the way a shell does.
// Pure: the list and a walk through it; useHistory.ts keeps them.

export const HISTORY_MAX = 50;

// remember puts a sent text first, once: a repeat moves up rather than doubling, and the list stays short
export function remember(list: string[], text: string, max = HISTORY_MAX): string[] {
  const sent = text.trim();
  if (!sent) return list;
  return [sent, ...list.filter((entry) => entry !== sent)].slice(0, max);
}

// leftAfterSend is the draft once the text sent went through: nothing when it did not change meanwhile, what was typed
// after it when the person kept writing while the send was out, the draft whole when they rewrote it
export function leftAfterSend(draft: string, sent: string): string {
  if (draft === sent) return "";
  return draft.startsWith(sent) ? draft.slice(sent.length).replace(/^\s+/, "") : draft;
}

export type Walk = { at: number; text: string }; // at: -1 is the live draft, 0 the newest entry

// recall steps through the list: up towards the oldest, down back to the draft that was being written
export function recall(list: string[], at: number, direction: "up" | "down", draft: string): Walk | null {
  if (direction === "up") {
    if (at + 1 >= list.length) return null;
    return { at: at + 1, text: list[at + 1]! };
  }
  if (at < 0) return null;
  if (at === 0) return { at: -1, text: draft };
  return { at: at - 1, text: list[at - 1]! };
}
