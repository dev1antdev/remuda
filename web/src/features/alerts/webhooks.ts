// Webhooks: the places alerts go beside this device — a Telegram bot, a Slack incoming webhook, an ntfy topic or any
// address taking JSON. The server keeps the list with its secrets; the page sees them masked and sends them back as
// they came, so a secret it never saw stays. The pure part is here with its tests; WebhooksSection.tsx draws it.

import { getJSON, sendJSON } from "@/lib/http";
import { ago } from "@/lib/time";
import { t } from "@/lib/i18n";
import type { Quiet } from "@/features/alerts/push";

export type WebhookKind = "telegram" | "slack" | "ntfy" | "generic";

export type Webhook = {
  id: string;
  kind: WebhookKind;
  name: string;
  url?: string; // masked past the host when it comes from the server
  token?: string; // never comes from the server; sent only when typed
  chat?: string;
  kinds: string[];
  enabled: boolean;
  quiet?: Quiet;
  has_token?: boolean;
  last_ok?: string;
  last_error?: string;
  last_error_at?: string;
};

export type WebhooksAnswer = { base: string; hooks: Webhook[]; kinds: string[] };

export const getWebhooks = () => getJSON<WebhooksAnswer>("/api/webhooks");
export const putWebhooks = (base: string, hooks: Webhook[]) => sendJSON<WebhooksAnswer>("PUT", "/api/webhooks", { base, hooks });
export const testWebhook = (id: string) => sendJSON<{ ok: boolean }>("POST", `/api/webhooks/${encodeURIComponent(id)}/test`);

// The kinds of webhook, as the picker offers them.
export const webhookKindOptions = (): { value: WebhookKind; label: string }[] => [
  { value: "telegram", label: t("Telegram bot") },
  { value: "slack", label: t("Slack incoming webhook") },
  { value: "ntfy", label: t("ntfy topic") },
  { value: "generic", label: t("Any address, as JSON") },
];

// The kinds of alert a webhook can take, in the panel's order, each with what it means.
export const hookKindOptions = (): { value: string; label: string; hint: string }[] => [
  { value: "question", label: t("a question"), hint: t("an agent waits for an answer, 10 s after — answered at the desk, it's never sent") },
  { value: "finish", label: t("a finish"), hint: t("every turn an agent ends, 10 s after") },
  { value: "watchdog", label: t("a watchdog note"), hint: t("stuck, waiting too long, or looping on one call") },
  { value: "digest", label: t("the digest"), hint: t("a digest schedule's standup") },
  { value: "schedule", label: t("a schedule"), hint: t("a scheduled prompt that could not be delivered, a scout's answer") },
  { value: "task", label: t("a task"), hint: t("an agent's run on a task finished or opened a pull request") },
  { value: "budget", label: t("a budget"), hint: t("a limit at 80 % or crossed") },
  { value: "review", label: t("a review"), hint: t("a rejected workspace whose agent says it is ready again") },
  { value: "machine", label: t("a machine away"), hint: t("a remote unreachable for two minutes, and back again after that") },
];

// DEFAULT_KINDS is what a new webhook hears: everything but the finishes, which come after every turn.
export const DEFAULT_KINDS = ["question", "watchdog", "digest", "schedule", "task", "budget", "review", "machine"];

const kindLabel = (kind: string) => hookKindOptions().find((o) => o.value === kind)?.label ?? kind;

// hookWords is a row's second line: the kind of webhook, what it hears, its quiet hours.
export function hookWords(h: Webhook): string {
  const kind = webhookKindOptions().find((o) => o.value === h.kind)?.label ?? h.kind;
  const parts = [kind];
  if (h.kind === "telegram" && h.chat) parts.push(t("chat {chat}", { chat: h.chat }));
  else if (h.url) parts.push(h.url);
  parts.push(h.kinds.length ? h.kinds.map(kindLabel).join(", ") : t("hears nothing"));
  if (h.quiet) parts.push(t("quiet {from}–{to}", { from: h.quiet.from, to: h.quiet.to }));
  if (!h.enabled) parts.push(t("off"));
  return parts.join(" · ");
}

// outcomeWords says how the last delivery went, for the row's end.
export function outcomeWords(h: Webhook, now = Date.now()): { text: string; tone: "ok" | "bad" | "none" } {
  const okAt = h.last_ok ? Date.parse(h.last_ok) : 0;
  const badAt = h.last_error_at ? Date.parse(h.last_error_at) : 0;
  if (h.last_error && badAt >= okAt) return { text: t("failed {ago}: {error}", { ago: ago(badAt, now), error: h.last_error }), tone: "bad" };
  if (okAt) return { text: t("delivered {ago}", { ago: ago(okAt, now) }), tone: "ok" };
  return { text: t("not tried yet"), tone: "none" };
}

// baseChoices are the addresses worth offering for the links: the tailnet's and this page's, when they differ from the one set.
export function baseChoices(current: string, tailscale: string | undefined, origin: string): string[] {
  const out: string[] = [];
  for (const candidate of [tailscale, origin]) {
    const trimmed = (candidate ?? "").replace(/\/+$/, "");
    if (trimmed && trimmed !== current && !out.includes(trimmed)) out.push(trimmed);
  }
  return out;
}

// Draft is the form's state: one shape for every kind, the secrets typed fresh or left as they are.
export type Draft = {
  id: string;
  kind: WebhookKind;
  name: string;
  url: string;
  token: string;
  chat: string;
  kinds: string[];
  enabled: boolean;
  hasToken: boolean;
  quietOn: boolean;
  quiet: Omit<Quiet, "tz">;
};

export const emptyDraft = (quiet: Omit<Quiet, "tz">): Draft => ({
  id: "",
  kind: "telegram",
  name: "",
  url: "",
  token: "",
  chat: "",
  kinds: [...DEFAULT_KINDS],
  enabled: true,
  hasToken: false,
  quietOn: false,
  quiet,
});

// draftOf opens a stored webhook in the form; the masked URL stays as it is, so sending it back keeps the stored one.
export function draftOf(h: Webhook, quiet: Omit<Quiet, "tz">): Draft {
  return {
    id: h.id,
    kind: h.kind,
    name: h.name,
    url: h.url ?? "",
    token: "",
    chat: h.chat ?? "",
    kinds: [...h.kinds],
    enabled: h.enabled,
    hasToken: !!h.has_token,
    quietOn: !!h.quiet,
    quiet: h.quiet ? { from: h.quiet.from, to: h.quiet.to } : quiet,
  };
}

// hookOf is what goes to the server: an empty token keeps the stored one, a masked URL too.
export function hookOf(d: Draft, zone: string): Webhook {
  const h: Webhook = { id: d.id, kind: d.kind, name: d.name.trim(), kinds: d.kinds, enabled: d.enabled };
  if (d.kind === "telegram") {
    h.chat = d.chat.trim();
    if (d.token.trim()) h.token = d.token.trim();
  } else {
    h.url = d.url.trim();
  }
  if (d.quietOn) h.quiet = { ...d.quiet, tz: zone };
  return h;
}

// checkDraft is the reason a draft cannot be saved yet, or "".
export function checkDraft(d: Draft): string {
  if (d.kind === "telegram") {
    if (!d.token.trim() && !d.hasToken) return t("The bot's token is needed.");
    if (!d.chat.trim()) return t("The chat id is needed.");
    return "";
  }
  if (!/^https?:\/\/[^\s/]+/.test(d.url.trim())) return t("The address must start with http:// or https://");
  return "";
}

// toggleKind adds or removes one kind, keeping the panel's order.
export function toggleKind(kinds: string[], kind: string, on: boolean): string[] {
  const order = hookKindOptions().map((o) => o.value);
  const set = new Set(kinds);
  if (on) set.add(kind);
  else set.delete(kind);
  return order.filter((k) => set.has(k));
}
