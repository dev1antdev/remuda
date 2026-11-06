import { t } from "@/lib/i18n";
// What a person may ask of an agent about to start, beyond its kind: a model, arguments of their own, a first prompt to
// send once it is up. The dialogs hold a Draft; optionsFor turns it into what the create routes take.

export type StartOptions = { model?: string; args?: string; prompt?: string };

// Draft is the Options fold's own state: the model as a choice (a known name, CUSTOM, or "" for the default) plus a typed id
export type Draft = { choice: string; custom: string; args: string; prompt: string };

export const CUSTOM = "custom";
export const EMPTY_DRAFT: Draft = { choice: "", custom: "", args: "", prompt: "" };

const MODELS: Record<string, { value: string; label: string }[]> = {
  claude: [
    { value: "opus", label: t("Opus") },
    { value: "sonnet", label: t("Sonnet") },
    { value: "haiku", label: t("Haiku") },
  ],
};

const PLACEHOLDERS: Record<string, string> = {
  claude: "claude-opus-4-1",
  codex: "gpt-5",
  gemini: "gemini-2.5-pro",
  kimi: "kimi-k2-thinking",
};

// MODEL_KINDS are the agents whose model flag remuda knows (sessions.ModelArgs); any other kind takes arguments only
export const MODEL_KINDS = Object.keys(PLACEHOLDERS);

// hasModelFlag says whether the kind can be told a model at all
export const hasModelFlag = (kind: string): boolean => MODEL_KINDS.includes(kind);

// modelChoices lists what the model Select offers for a kind: the default, the names remuda knows, and a typed id;
// empty for a kind with no known names, which gets a text field instead
export function modelChoices(kind: string): { value: string; label: string; hint?: string }[] {
  const known = MODELS[kind];
  if (!known) return [];
  return [{ value: "", label: t("Default"), hint: t("the agent's own choice") }, ...known, { value: CUSTOM, label: t("Another model…"), hint: t("a full model id") }];
}

// modelPlaceholder is the example id a kind's text field shows
export const modelPlaceholder = (kind: string): string => PLACEHOLDERS[kind] ?? "";

// modelOf is the model a draft asks for a kind: the choice, or the typed id when the choice is CUSTOM or the kind has no names
export function modelOf(kind: string, draft: Draft): string {
  if (!hasModelFlag(kind)) return "";
  if (draft.choice === CUSTOM || !MODELS[kind]) return draft.custom.trim();
  return draft.choice.trim();
}

// optionsFor is what the create request carries: only the fields that say something, nothing for a shell
export function optionsFor(kind: string, draft: Draft): StartOptions {
  if (kind === "" || kind === "shell") return {};
  const out: StartOptions = {};
  const model = modelOf(kind, draft);
  if (model) out.model = model;
  if (draft.args.trim()) out.args = draft.args.trim();
  if (draft.prompt.trim()) out.prompt = draft.prompt.trim();
  return out;
}

// hasOptions says whether a draft asks for anything, so a dialog can show the fold as "set"
export const hasOptions = (kind: string, draft: Draft): boolean => Object.keys(optionsFor(kind, draft)).length > 0;

// choiceFor is the draft's model part for a remembered model of a kind: a known name is the choice, anything else a typed id
export function choiceFor(kind: string, model: string): Pick<Draft, "choice" | "custom"> {
  if (!model) return { choice: "", custom: "" };
  if (MODELS[kind]?.some((m) => m.value === model)) return { choice: model, custom: "" };
  return { choice: MODELS[kind] ? CUSTOM : "", custom: model };
}

// lastModelKey is the pref that remembers the model last used for a kind on this device
export const lastModelKey = (kind: string): string => `start.model:${kind}`;
