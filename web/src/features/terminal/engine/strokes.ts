// What the terminal's typing carries: text — typed, or a paste — and keys, coalesced into one request in the order
// typed. Pure.

export type Stroke = { text: string; paste?: boolean } | { key: string };

// Batch is one request of the terminal's typing: the text, then the keys after it.
export type Batch = { text: string; paste: boolean; keys: string[] };

// MAX_TEXT bounds the typed text one request carries; a paste goes whole.
export const MAX_TEXT = 4096;
// MAX_KEYS bounds the keys one request carries.
export const MAX_KEYS = 64;

// takeBatch takes the next request off the front of the queue: typed text, or one paste, then the keys after it; a text
// after a key waits for the next request, so the order typed is the order sent
export function takeBatch(queue: Stroke[]): { batch: Batch; rest: Stroke[] } | null {
  if (!queue.length) return null;
  let text = "";
  let paste = false;
  const keys: string[] = [];
  let i = 0;
  for (; i < queue.length; i++) {
    const stroke = queue[i]!;
    if ("key" in stroke) {
      if (keys.length >= MAX_KEYS) break;
      keys.push(stroke.key);
      continue;
    }
    if (i > 0 && (keys.length || paste || stroke.paste || text.length + stroke.text.length > MAX_TEXT)) break;
    paste = !!stroke.paste;
    text += stroke.text;
  }
  return { batch: { text, paste, keys }, rest: queue.slice(i) };
}
