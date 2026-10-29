import { useEffect, useState } from "react";
import { actions } from "@/app/store";
import { errMsg } from "@/lib/text";
import { t } from "@/lib/i18n";
import { Icon, Notice, Select, Tip } from "@/ui";
import { useRoster } from "@/features/roster/useRoster";
import { deviceZone, readQuietWindow } from "@/features/alerts/prefs";
import {
  baseChoices,
  checkDraft,
  draftOf,
  emptyDraft,
  getWebhooks,
  hookKindOptions,
  hookOf,
  hookWords,
  outcomeWords,
  putWebhooks,
  testWebhook,
  toggleKind,
  webhookKindOptions,
  type Draft,
  type Webhook,
} from "@/features/alerts/webhooks";

// The Webhooks section of the alerts panel: the address the links carry, the list with a Test on each, a form to add or edit one.
export default function WebhooksSection() {
  const { tailscale } = useRoster();
  const [base, setBase] = useState("");
  const [savedBase, setSavedBase] = useState("");
  const [hooks, setHooks] = useState<Webhook[] | null>(null);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [note, setNote] = useState("");

  useEffect(() => {
    getWebhooks()
      .then((a) => {
        setBase(a.base);
        setSavedBase(a.base);
        setHooks(a.hooks);
      })
      .catch((e) => setErr(errMsg(e)));
  }, []);

  const run = async (fn: () => Promise<void>, ok = "") => {
    setBusy(true);
    setErr("");
    setNote("");
    try {
      await fn();
      if (ok) setNote(ok);
    } catch (e) {
      setErr(errMsg(e));
    } finally {
      setBusy(false);
    }
  };

  const save = (nextBase: string, nextHooks: Webhook[], ok = "") =>
    run(async () => {
      const a = await putWebhooks(nextBase, nextHooks);
      setBase(a.base);
      setSavedBase(a.base);
      setHooks(a.hooks);
      setDraft(null);
    }, ok);

  const remove = (h: Webhook) =>
    actions.ask(t("Remove the webhook “{name}”? Nothing will be sent there any more.", { name: h.name }), t("Remove"), () => {
      void save(base, (hooks ?? []).filter((x) => x.id !== h.id), t("Webhook removed."));
    });

  const test = (h: Webhook) =>
    run(async () => {
      await testWebhook(h.id);
      setHooks((await getWebhooks()).hooks);
    }, t("Test sent to {name}.", { name: h.name }));

  const choices = baseChoices(base, tailscale?.url, location.origin);
  const zone = deviceZone();

  return (
    <section className="webhooks">
      <h3>
        {t("Webhooks")}
        <button type="button" className="btn sm" disabled={busy || !!draft} onClick={() => setDraft(emptyDraft(readQuietWindow()))}>
          <Icon name="plus" size={12} /> {t("Add a webhook")}
        </button>
      </h3>
      <small>{t("A Telegram bot, a Slack incoming webhook, an ntfy topic or any address taking JSON hears the same alerts as a device, the kinds you tick, with a link back to the pane. Nothing is sent to an address you did not set here.")}</small>

      <label className="webhooks-base">
        {t("The address the links lead to")}
        <input
          value={base}
          placeholder="https://remuda.your-tailnet.ts.net"
          disabled={busy}
          onChange={(e) => setBase(e.target.value)}
          onBlur={() => hooks && base.trim() !== savedBase && void save(base, hooks)}
          onKeyDown={(e) => e.key === "Enter" && hooks && base.trim() !== savedBase && void save(base, hooks)}
        />
        {choices.length > 0 && (
          <span className="webhooks-choices">
            {choices.map((c) => (
              <button key={c} type="button" className="linkish" disabled={busy} onClick={() => hooks && void save(c, hooks)}>
                {t("Use {address}", { address: c })}
              </button>
            ))}
          </span>
        )}
      </label>

      {hooks && hooks.length === 0 && !draft && <div className="empty">{t("No webhooks yet.")}</div>}
      {hooks?.map((h) => {
        const outcome = outcomeWords(h);
        return (
          <div key={h.id} className={`webhook-row${h.enabled ? "" : " is-off"}`}>
            <span className="webhook-name">
              {h.name}
              <small>{hookWords(h)}</small>
              <small className={`webhook-outcome is-${outcome.tone}`}>{outcome.text}</small>
            </span>
            <button type="button" className="btn sm" disabled={busy} onClick={() => void test(h)}>
              {t("Test")}
            </button>
            <Tip text={t("Edit this webhook")}>
              <button type="button" className="icon-btn sm" aria-label={t("Edit this webhook")} disabled={busy} onClick={() => setDraft(draftOf(h, readQuietWindow()))}>
                <Icon name="edit" size={14} />
              </button>
            </Tip>
            <Tip text={t("Remove this webhook")}>
              <button type="button" className="icon-btn sm danger" aria-label={t("Remove this webhook")} disabled={busy} onClick={() => remove(h)}>
                <Icon name="trash" size={14} />
              </button>
            </Tip>
          </div>
        );
      })}

      {draft && (
        <WebhookForm
          draft={draft}
          busy={busy}
          onChange={setDraft}
          onCancel={() => setDraft(null)}
          onSave={() => {
            const why = checkDraft(draft);
            if (why) {
              setErr(why);
              return;
            }
            const next = hookOf(draft, zone);
            const list = hooks ?? [];
            void save(base, next.id ? list.map((x) => (x.id === next.id ? next : x)) : [...list, next], t("Webhook saved — press Test to see it arrive."));
          }}
        />
      )}
      {err && <Notice tone="error" onDismiss={() => setErr("")}>{err}</Notice>}
      {note && <Notice tone="success" onDismiss={() => setNote("")}>{note}</Notice>}
    </section>
  );
}

function WebhookForm({ draft, busy, onChange, onCancel, onSave }: { draft: Draft; busy: boolean; onChange: (d: Draft) => void; onCancel: () => void; onSave: () => void }) {
  const set = (part: Partial<Draft>) => onChange({ ...draft, ...part });
  const setTime = (key: "from" | "to", value: string) => /^\d\d:\d\d$/.test(value) && set({ quiet: { ...draft.quiet, [key]: value } });
  return (
    <form
      className="webhook-form"
      onSubmit={(e) => {
        e.preventDefault();
        onSave();
      }}
    >
      <div className="webhook-fields">
        <label>
          {t("Kind")}
          <Select value={draft.kind} options={webhookKindOptions()} onChange={(kind) => set({ kind })} />
        </label>
        <label className="grow">
          {t("Name")}
          <input value={draft.name} placeholder={t("My phone")} onChange={(e) => set({ name: e.target.value })} autoFocus />
        </label>
      </div>
      {draft.kind === "telegram" ? (
        <div className="webhook-fields">
          <label className="grow">
            {t("Bot token")}
            <input
              value={draft.token}
              type="password"
              autoComplete="off"
              placeholder={draft.hasToken ? t("kept as it is") : "123456:ABC…"}
              onChange={(e) => set({ token: e.target.value })}
            />
            <small>{t("From @BotFather. Message the bot once, or add it to the chat, so it may write to you.")}</small>
          </label>
          <label>
            {t("Chat id")}
            <input value={draft.chat} inputMode="numeric" placeholder="123456789" onChange={(e) => set({ chat: e.target.value })} />
          </label>
        </div>
      ) : (
        <label>
          {t("Address")}
          <input value={draft.url} type="url" autoComplete="off" placeholder={placeholderFor(draft.kind)} onChange={(e) => set({ url: e.target.value })} />
          <small>{hintFor(draft.kind)}</small>
        </label>
      )}
      <fieldset className="webhook-kinds">
        <legend>{t("Send it")}</legend>
        {hookKindOptions().map((o) => (
          <label key={o.value} className="alerts-opt">
            <input type="checkbox" checked={draft.kinds.includes(o.value)} onChange={(e) => set({ kinds: toggleKind(draft.kinds, o.value, e.target.checked) })} />
            <span>
              {o.label}
              <small>{o.hint}</small>
            </span>
          </label>
        ))}
      </fieldset>
      <label className="alerts-opt">
        <input type="checkbox" checked={draft.quietOn} onChange={(e) => set({ quietOn: e.target.checked })} />
        <span>
          <b>{t("Quiet hours")}</b>
          <small>{t("Nothing goes there between the two times, by this device's clock ({zone}).", { zone: deviceZone() })}</small>
        </span>
      </label>
      <div className="alerts-quiet">
        <input type="time" className="sm" value={draft.quiet.from} disabled={!draft.quietOn} aria-label={t("Quiet from")} onChange={(e) => setTime("from", e.target.value)} />
        <span>to</span>
        <input type="time" className="sm" value={draft.quiet.to} disabled={!draft.quietOn} aria-label={t("Quiet until")} onChange={(e) => setTime("to", e.target.value)} />
      </div>
      <label className="alerts-opt">
        <input type="checkbox" checked={draft.enabled} onChange={(e) => set({ enabled: e.target.checked })} />
        <span>
          <b>{t("On")}</b>
          <small>{t("Off, the webhook stays in the list and hears nothing.")}</small>
        </span>
      </label>
      <div className="webhook-actions">
        <button type="button" className="btn" disabled={busy} onClick={onCancel}>
          {t("Cancel")}
        </button>
        <button type="submit" className="btn primary" disabled={busy}>
          {draft.id ? t("Save") : t("Add")}
        </button>
      </div>
    </form>
  );
}

const placeholderFor = (kind: Draft["kind"]) =>
  kind === "slack" ? "https://hooks.slack.com/services/…" : kind === "ntfy" ? "https://ntfy.sh/my-topic" : "https://example.com/hook";

const hintFor = (kind: Draft["kind"]) =>
  kind === "slack"
    ? t("An incoming webhook of your Slack app; the message is its text.")
    : kind === "ntfy"
      ? t("The topic's address on ntfy.sh or your own server; the title, the body and a Click link.")
      : t("A POST with the alert as JSON: kind, title, body, machine, pane, link, at.");
