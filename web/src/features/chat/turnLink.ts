// A link to one turn of a pane's conversation: the page's own address with the pane in its chat lens, the session shown
// and the turn's first record, which the page reads once at load into a jump. The pure part; TurnView copies it.

export type TurnLinkTarget = { machineId: string; paneId: string; session: string; turn: string };

// turnLink is the address that opens the conversation scrolled to the turn
export function turnLink(origin: string, target: TurnLinkTarget): string {
  const q = new URLSearchParams();
  q.set("machine", target.machineId);
  q.set("pane", target.paneId);
  q.set("lens", "chat");
  if (target.session) q.set("session", target.session);
  q.set("turn", target.turn);
  return `${origin}/?${q}`;
}

// readTurnLink is the turn an address asks for, with its session when it names one; nothing without a pane and a turn
export function readTurnLink(search: string): TurnLinkTarget | null {
  const q = new URLSearchParams(search);
  const paneId = q.get("pane");
  const turn = q.get("turn");
  if (!paneId || !turn) return null;
  return { machineId: q.get("machine") || "local", paneId, session: q.get("session") || "", turn };
}
