// The Automatic tab's words: where a found picture came from, and where the search looked when it found none.

import { t } from "@/lib/i18n";
import type { FoundIcons, IconCandidate } from "./api";

// fromWords says where a candidate came from: its file in the folder, or whose avatar it is
export function fromWords(candidate: IconCandidate): string {
  if (candidate.kind === "github") return t("GitHub avatar · {owner}", { owner: candidate.from.replace(/^github\.com\//, "") });
  if (candidate.kind === "gitlab") return t("GitLab avatar · {owner}", { owner: candidate.from.replace(/^[^/]+\//, "") });
  return candidate.from;
}

// NOTES are the server's fixed notes, each with the one name it carries, so the page can say them in its language
const NOTES: [RegExp, string][] = [
  [/^no origin remote, so no owner's avatar$/, "no origin remote, so no owner's avatar"],
  [/^the origin remote names no host and owner$/, "the origin remote names no host and owner"],
  [/^GitHub has no avatar for (.+)$/, "GitHub has no avatar for {name}"],
  [/^GitLab has no avatar for (.+) or its group$/, "GitLab has no avatar for {name} or its group"],
  [/^no glab on (.+) to read the GitLab avatar$/, "no glab on {name} to read the GitLab avatar"],
  [/^glab could not be asked on (.+)$/, "glab could not be asked on {name}"],
];

// noteText is one note in the page's language, a note it does not know as the server wrote it
export function noteText(note: string): string {
  for (const [pattern, key] of NOTES) {
    const match = pattern.exec(note);
    if (match) return t(key, { name: match[1] ?? "" });
  }
  return note;
}

// noteWords says why no avatar came, as sentences
export function noteWords(notes: string[] = []): string {
  return notes
    .map(noteText)
    .map((note) => `${note.charAt(0).toUpperCase()}${note.slice(1)}.`)
    .join(" ");
}

// lookedWords is the line under "none found": the folders searched, then why no avatar came
export function lookedWords(found: FoundIcons): string {
  const places = t("Looked in {places}.", { places: found.looked.join(", ") });
  return [places, noteWords(found.notes)].filter(Boolean).join(" ");
}
