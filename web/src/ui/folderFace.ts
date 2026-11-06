// A folder's face when the person gave it none: its initial on a tile in one of Catppuccin's fourteen colours, the
// colour from its name, so a folder keeps its look on every device. The pure part of FolderIcon.

import { ACCENTS, type Accent } from "@/lib/theme";

// initialOf is the folder name's first letter or digit, upper-cased; "?" for a name with neither
export function initialOf(name: string): string {
  for (const ch of name) if (/[\p{L}\p{N}]/u.test(ch)) return ch.toLocaleUpperCase();
  return "?";
}

// tileColour is the accent a name always gets
export function tileColour(name: string): Accent {
  let hash = 5381;
  for (const ch of name) hash = (hash * 33) ^ ch.codePointAt(0)!;
  return ACCENTS[Math.abs(hash) % ACCENTS.length]!;
}

// iconURL is where the page reads a folder's picture
export const iconURL = (image: string) => `/api/icons/${encodeURIComponent(image)}`;
