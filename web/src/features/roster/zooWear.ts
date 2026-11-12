// What a zoo creature wears, so two agents of one kind tell apart across the room: its species by the agent's vendor (a
// round critter with antennae for Claude Code, a boxy robot for Codex, an egg with sparkles for Gemini CLI, a moon
// rabbit for Kimi Code, a tufted blob for the rest), a hat by the model it runs on, and a trinket of its own — no two
// creatures of a species on the wall share one while there are trinkets left. The pure part; app/zoo draws them.

export type Species = "claude" | "codex" | "gemini" | "kimi" | "other";

const SPECIES: Record<string, Species> = { claude: "claude", codex: "codex", gemini: "gemini", kimi: "kimi" };

// speciesOf is the creature's body for herdr's agent name
export const speciesOf = (agent: string): Species => SPECIES[agent] ?? "other";

export type Hat = "" | "tophat" | "wizard" | "beret" | "sprout" | "laurel" | "cap" | "crown";

// hatOf is the hat a model wears, by its family as the transcripts name it: Opus a top hat, Fable a wizard's, Sonnet a
// beret, Haiku a sprout, Mythos a laurel, OpenAI's models a cap; none for a model it does not know
export function hatOf(model: string | undefined): Hat {
  const name = (model ?? "").toLowerCase();
  if (name.includes("opus")) return "tophat";
  if (name.includes("fable")) return "wizard";
  if (name.includes("sonnet")) return "beret";
  if (name.includes("haiku")) return "sprout";
  if (name.includes("mythos")) return "laurel";
  if (/^(gpt|o\d|codex)/.test(name.replace(/^openai\//, ""))) return "cap";
  return "";
}

// smallModel says whether the model is the family's little one, drawn a size smaller
export const smallModel = (model: string | undefined): boolean => /haiku|[-_](mini|nano)\b|flash-lite/i.test(model ?? "");

export type Trinket = "" | "glasses" | "bow" | "scarf" | "bowtie" | "freckles" | "mustache" | "spots";

// TRINKETS is every trinket, the bare face first
export const TRINKETS: Trinket[] = ["", "glasses", "bow", "scarf", "bowtie", "freckles", "mustache", "spots"];

// seedOf is a creature's own number, the same for the same pane on every page: FNV-1a over its key
export function seedOf(key: string): number {
  let hash = 0x811c9dc5;
  for (let i = 0; i < key.length; i++) {
    hash ^= key.charCodeAt(i);
    hash = Math.imul(hash, 0x01000193);
  }
  return hash >>> 0;
}

export type Outfit = { species: Species; hat: Hat; small: boolean; trinket: Trinket; seed: number };

export type Wearer = { key: string; agent: string; model?: string };

// outfits dresses the wall in the order given (the roster's, stable): each creature takes the trinket its seed names,
// or, when another of its species already wears that one, the next one free; past the last free one they repeat
export function outfits(wearers: Wearer[]): Map<string, Outfit> {
  const taken = new Map<Species, Set<Trinket>>();
  const out = new Map<string, Outfit>();
  for (const w of wearers) {
    const species = speciesOf(w.agent);
    const seed = seedOf(w.key);
    const worn = taken.get(species) ?? new Set<Trinket>();
    taken.set(species, worn);
    const first = seed % TRINKETS.length;
    let trinket = TRINKETS[first]!;
    for (let step = 0; step < TRINKETS.length; step++) {
      const candidate = TRINKETS[(first + step) % TRINKETS.length]!;
      if (!worn.has(candidate)) {
        trinket = candidate;
        break;
      }
    }
    worn.add(trinket);
    out.set(w.key, { species, hat: hatOf(w.model), small: smallModel(w.model), trinket, seed });
  }
  return out;
}
