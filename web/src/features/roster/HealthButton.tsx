// A machine's vitals: the ⓘ beside its name, amber when something wants a look, and the popover with the
// versions against the newest known, disk, load, and a button per update on offer.

import { useEffect, useState } from "react";
import UpdatesDialog from "@/features/updates/Updates";
import { errMsg } from "@/lib/text";
import { ago } from "@/lib/time";
import { newer } from "@/lib/version";
import { Icon, Popover, Tip } from "@/ui";
import { useRoster } from "./useRoster";
import { useSharing } from "./useSharing";
import { actions, app } from "@/app/store";
import { refreshHealth, type Health, type Latest, type Machine } from "./api";
import { awakeBad, awakeWords } from "./awake";
import { resumeStopped } from "@/features/updates/recover";
import { forecastTone, limitLine, limitTone } from "@/features/usage/limits";
import { concerns, updateLabel } from "./concerns";
import { loginWords } from "./signin";
import { BodyRows } from "./BodyRows";
import { Sparkline } from "./Sparkline";
import { DISK_THRESHOLDS, loadThresholds, rangeOf, sparkTitle, spanWords } from "./spark";
import { t, tn } from "@/lib/i18n";

// viewOnly: a viewer link signed the page in — the vitals read, nothing to run, refresh or update
export function HealthButton({ machine: m, latest, serverVersion, viewOnly = false }: { machine: Machine; latest?: Latest; serverVersion?: string; viewOnly?: boolean }) {
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [fresh, setFresh] = useState<{ health: Health | null; latest: Latest } | null>(null);
  const [updateWhat, setUpdateWhat] = useState<string | null>(null);
  const share = useSharing();
  const { retirements } = useRoster();
  const name = share.machineLabel(m);
  // a manual read stands until the collector's next one
  useEffect(() => setFresh(null), [m.health?.at]);
  // the vitals go with the drawer they were opened from, on a phone
  const drawer = app.use((s) => s.drawer);
  useEffect(() => {
    if (!drawer) setOpen(false);
  }, [drawer]);
  const h = fresh?.health ?? m.health;
  const l = fresh?.latest ?? latest;
  // while sharing, a stopped chat goes by its pane rather than its title, and the rest by the neutral names
  const stopped = share.on ? m.stopped?.map((chat) => ({ ...chat, title: "" })) : m.stopped;
  const list = concerns(h, l, serverVersion, m.id !== "local", stopped, viewOnly, { machine: name, retiring: m.retiring, retirements }).map((c) => ({ ...c, text: share.redact(c.text) }));
  const worst = list.some((c) => c.bad) ? "bad" : list.length ? "warn" : "";
  if (!h && m.state !== "connected") return null;
  const refresh = async () => {
    setBusy(true);
    try {
      setFresh(await refreshHealth(m.id));
    } catch (e) {
      setFresh({ health: { ...(h ?? { at: "", disk_free_gb: 0, disk_used_pct: 0, load1: 0 }), error: errMsg(e) }, latest: l ?? { at: "" } });
    } finally {
      setBusy(false);
    }
  };
  const remudaHave = m.id === "local" ? serverVersion : h?.remuda;
  const history = h?.history && h.history.length >= 2 ? h.history : null;
  const span = history ? spanWords(history[0]!.at, history[history.length - 1]!.at) : "";
  const loads = history ? history.map((s) => s.load) : [];
  const free = history ? history.map((s) => s.disk_free) : [];
  const loadRange = rangeOf(loads, 0, h?.cores ?? 0);
  const update = (what: string) => {
    setOpen(false);
    if (what === "recover") {
      void resumeStopped(m.id, m.stopped?.length ?? 0); // the chats a reboot stopped: a job followed by toasts, no dialog
      return;
    }
    if (what === "signin") {
      actions.openFloat(m.id); // a shell there for `claude auth login`: remuda never signs in for the person
      return;
    }
    setUpdateWhat(what);
  };
  const summary = list.length ? list.map((c) => share.redact(c.text)) : [h ? t("{machine}: all good — {os}", { machine: name, os: h.os ?? "" }) : t("Vitals not read yet")];
  return (
    <>
      {updateWhat !== null && <UpdatesDialog machine={m} what={updateWhat || undefined} onClose={() => setUpdateWhat(null)} onDone={() => void refresh()} />}
      <Popover
        open={open}
        onOpenChange={setOpen}
        className="health-pop"
        label={t("{machine} vitals", { machine: name })}
        tip={summary.map((line, i) => (
          <div key={i}>{line}</div>
        ))}
        trigger={
          <button type="button" className={`icon-btn${worst ? ` health-${worst}` : ""}${open ? " on" : ""}`} data-health-btn={m.id} aria-label={t("{machine} vitals", { machine: name })}>
            <Icon name={list.length ? "alert-triangle" : "info"} size={14} />
            {list.length > 0 && <span className="health-count">{list.length}</span>}
          </button>
        }
      >
        <div className="health-head">
          <b>{name}</b>
          <span className="dim">{[h?.os, h?.model].filter(Boolean).join(" · ")}</span>
          {!viewOnly && (
            <Tip text={t("Read again now")}>
              <button type="button" className="icon-btn sm" aria-label={t("Read again now")} disabled={busy} onClick={() => void refresh()}>
                <Icon name="refresh-cw" size={12} className={busy ? "spin" : undefined} />
              </button>
            </Tip>
          )}
        </div>
        {h ? (
          <dl className="health-rows">
            <dt>{t("Claude Code")}</dt>
            <dd>
              {h.claude ?? "—"}
              {h.claude && l?.claude && newer(h.claude, l.claude) ? <span className="health-new"> → {l.claude}</span> : null}
            </dd>
            {h.claude_login && (
              <>
                <dt>{t("Claude sign-in")}</dt>
                <dd>{loginWords(h.claude_login, Date.now())}</dd>
              </>
            )}
            {h.codex && (
              <>
                <dt>{t("Codex")}</dt>
                <dd>
                  {h.codex}
                  {l?.codex && newer(h.codex, l.codex) ? <span className="health-new"> → {l.codex}</span> : null}
                </dd>
              </>
            )}
            {h.gemini && (
              <>
                <dt>{t("Gemini CLI")}</dt>
                <dd>
                  {h.gemini}
                  {l?.gemini && newer(h.gemini, l.gemini) ? <span className="health-new"> → {l.gemini}</span> : null}
                </dd>
              </>
            )}
            {h.kimi && (
              <>
                <dt>{t("Kimi Code")}</dt>
                <dd>
                  {h.kimi}
                  {l?.kimi && newer(h.kimi, l.kimi) ? <span className="health-new"> → {l.kimi}</span> : null}
                </dd>
              </>
            )}
            <dt>herdr</dt>
            <dd>
              {h.herdr ?? "—"}
              {h.herdr_server && h.herdr !== h.herdr_server ? <span className="health-new"> · {t("server {version}", { version: h.herdr_server })}</span> : null}
              {h.herdr && l?.herdr && newer(h.herdr, l.herdr) ? <span className="health-new"> → {l.herdr}</span> : null}
            </dd>
            {h.outdated_hooks?.length ? (
              <>
                <dt>{t("herdr hooks")}</dt>
                <dd>
                  {h.outdated_hooks.map((k, i) => (
                    <span key={k.agent}>
                      {i > 0 ? ", " : ""}
                      {k.agent} {k.have}
                      {k.want ? <span className="health-new"> → {k.want}</span> : null}
                    </span>
                  ))}
                </dd>
              </>
            ) : null}
            {h.plugins?.length ? (
              <>
                <dt>{t("herdr plugins")}</dt>
                <dd>{h.plugins.map((p) => p.id).join(", ")}</dd>
              </>
            ) : null}
            <dt>remuda</dt>
            <dd>
              {h.remuda ?? (m.id === "local" ? serverVersion : t("not deployed"))}
              {m.id !== "local" && h.remuda ? <span className="dim"> · {h.remuda_running ? t("running, terminals go through it") : t("not running")}</span> : null}
              {remudaHave && l?.remuda && newer(remudaHave, l.remuda) ? <span className="health-new"> → {l.remuda}</span> : null}
            </dd>
            <dt>{t("Disk")}</dt>
            <dd>
              {t("{free} GB free · {used}% used", { free: h.disk_free_gb.toFixed(0), used: h.disk_used_pct })}
              {history && <Sparkline values={free} min={0} max={100} color="var(--ctp-green)" thresholds={DISK_THRESHOLDS} title={sparkTitle(t("Disk free"), free, (v) => `${v.toFixed(0)}%`, span)} />}
            </dd>
            <dt>{t("Load")}</dt>
            <dd>
              {h.load1.toFixed(2)}
              {h.cores ? ` ${tn(h.cores, "on {n} core", "on {n} cores")}` : ""}
              {h.uptime ? ` · ${t("up {time}", { time: h.uptime })}` : ""}
              {history && <Sparkline values={loads} min={loadRange.min} max={loadRange.max} color="var(--ctp-blue)" thresholds={loadThresholds(h.cores)} title={sparkTitle(t("Load"), loads, (v) => v.toFixed(2), span)} />}
            </dd>
            <BodyRows health={h} span={span} />
            {m.awake?.on && (
              <>
                <dt>{t("Awake")}</dt>
                <dd className={awakeBad(m.awake) ? "health-awake-bad" : undefined}>{awakeWords(m.awake)}</dd>
              </>
            )}
            {(m.limits?.windows ?? []).length > 0 && (
              <>
                <dt>{t("Plan limits")}</dt>
                <dd>
                  {m.limits!.windows!.map((w) => (
                    <div key={`${w.agent}|${w.name}`} className={limitTone(w.used) || forecastTone(w) ? `health-limit is-${limitTone(w.used) || forecastTone(w)}` : "health-limit"}>
                      {limitLine(w, Date.now())}
                    </div>
                  ))}
                </dd>
              </>
            )}
            <dt>{t("Read")}</dt>
            <dd>
              {ago(h.at)}
              {span ? <span className="dim"> · {t("{span} kept", { span })}</span> : null}
            </dd>
          </dl>
        ) : (
          <div className="dim">{t("Not read yet.")}</div>
        )}
        {list.length > 0 && (
          <ul className="health-list">
            {list.map((c) => (
              <li key={c.key} className={c.bad ? "bad" : ""}>
                <span className="health-concern">{c.text}</span>
                {c.update && (
                  <button type="button" className="btn sm" onClick={() => update(c.update!)}>
                    {updateLabel(c.update)}
                  </button>
                )}
              </li>
            ))}
          </ul>
        )}
        {!viewOnly && (
          <div className="health-foot">
            <button type="button" className="linkish" onClick={() => update("")}>
              <Icon name="refresh-cw" size={12} /> {t("All updates…")}
            </button>
          </div>
        )}
        {h?.error && h.claude && <div className="dim">{share.redact(h.error)}</div>}
      </Popover>
    </>
  );
}
