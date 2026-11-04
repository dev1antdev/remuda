// The Automatic tab's words: where a found picture came from, and where the search looked when it found none.

import { t } from "@/lib/i18n";
import type { FoundIcons, IconCandidate } from "./api";

// fromWords says where a candidate came from: its file in the folder, or whose avatar it is
export function fromWords(candidate: IconCandidate): string {
  if (candidate.kind === "github") return t("GitHub avatar · {owner}", { owner: candidate.from.replace(/^github\.com\//, "") });
  if (candidate.kind === "gitlab") return t("GitLab avatar · {owner}", { owner: candidate.from.replace(/^[^/]+\//, "") });
  return candidate.from;
}

// noteWords says why no avatar came, as sentences
export function noteWords(notes: string[] = []): string {
  return notes.map((note) => `${note.charAt(0).toUpperCase()}${note.slice(1)}.`).join(" ");
}

// lookedWords is the line under "none found": the folders searched, then why no avatar came
export function lookedWords(found: FoundIcons): string {
  const places = t("Looked in {places}.", { places: found.looked.join(", ") });
  return [places, noteWords(found.notes)].filter(Boolean).join(" ");
}
