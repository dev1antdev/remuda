// The notes queued for an agent while it works: written down as they come, sent only when the person presses Send —
// one, or all as one numbered message — never by themselves. Kept per device and per pane. The pure part of QueueList.

// QueuedNote is one note, oldest first in the list: the order they are sent in
export type QueuedNote = { id: string; text: string; at: number };

// MAX_NOTES is how many notes a pane's queue keeps; past it the oldest go
export const MAX_NOTES = 50;

// queueKey is the pref a pane's queue lives in; the folder is in it, so a pane id herdr gives again elsewhere starts empty
export const queueKey = (machineId: string, dir: string, paneId: string) => `queue:${machineId}|${dir}|${paneId}`;

// addNote puts a note at the end of the queue; a text of nothing but blanks adds nothing
export function addNote(notes: QueuedNote[], text: string, id: string, now: number): QueuedNote[] {
  const kept = text.trim();
  if (!kept) return notes;
  return [...notes, { id, text: kept, at: now }].slice(-MAX_NOTES);
}

export const removeNote = (notes: QueuedNote[], id: string): QueuedNote[] => notes.filter((n) => n.id !== id);

// numbered is the message all the notes make: one note as it is, several numbered, a note's later lines indented under it
export function numbered(notes: QueuedNote[]): string {
  if (notes.length === 1) return notes[0]!.text;
  return notes
    .map((n, i) => {
      const mark = `${i + 1}. `;
      const [first, ...rest] = n.text.split("\n");
      return [mark + first, ...rest.map((line) => (line ? " ".repeat(mark.length) + line : ""))].join("\n");
    })
    .join("\n");
}

// sentAway is the queue after a send went through: the notes sent leave, any added meanwhile stay
export const sentAway = (notes: QueuedNote[], sent: QueuedNote[]): QueuedNote[] => notes.filter((n) => !sent.some((s) => s.id === n.id));

// noteLine is a note's first line, for its row
export const noteLine = (note: QueuedNote): string => note.text.split("\n")[0] ?? "";
