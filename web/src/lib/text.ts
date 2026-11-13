// The one-liners every screen was re-typing: an error's words, a path's parts, a word with its count.

import { tn } from "./i18n";

export function errMsg(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

// a path's last segment and the folder before it, a Windows machine's path too (pathname.ts)
export { basename, dirname } from "./pathname";

// plural says "3 files", "1 file" — in another language the forms the dictionary keeps under "{n} files".
export const plural = (count: number, word: string) => tn(count, `{n} ${word}`, `{n} ${word}s`);

// shellQuote wraps a path for a POSIX shell (the terminal lens types paths).
export function shellQuote(p: string): string {
  return /^[\w./-]+$/.test(p) ? p : "'" + p.replace(/'/g, "'\\''") + "'";
}
