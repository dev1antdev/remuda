// A machine's vitals: the ⓘ beside its name, amber when something wants a look, and the popover with the
// versions against the newest known, disk, load, and a button per update on offer.

import { useCallback, useEffect, useState } from "react";
import UpdatesDialog from "@/features/updates/Updates";
import { errMsg } from "@/lib/text";
import { ago } from "@/lib/time";
import { newer } from "@/lib/version";
import { Icon, useDismiss } from "@/ui";
import { refreshHealth, type Health, type Latest, type Machine } from "./api";
import { concerns, updateLabel } from "./concerns";
import { Sparkline } from "./Sparkline";
import { DISK_THRESHOLDS, loadThresholds, rangeOf, sparkTitle, spanWords } from "./spark";

export function HealthButton({ machine: m, latest, serverVersion }: { machine: Machine; latest?: Latest; serverVersion?: string }) {
  const [open, setOpen] = useState<{ x: number; y: number } | null>(null);
  const [busy, setBusy] = useState(false);
  const [fresh, setFresh] = useState<{ health: Health | null; latest: Latest } | null>(null);
  const [updateWhat, setUpdateWhat] = useState<string | null>(null);
  const close = useCallback(() => setOpen(null), []);
  useDismiss(!!open, close, [".health-pop", `[data-health-btn="${CSS.escape(m.id)}"]`]);
  // a manual read stands until the collector's next one
  useEffect(() => setFresh(null), [m.health?.at]);
  const h = fresh?.health ?? m.health;
  const l = fresh?.latest ?? latest;
  const list = concerns(h, l, serverVersion, m.id !== "local");
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
    setOpen(null);
    setUpdateWhat(what);
  };
  return (
    <>
      <button
        type="button"
        className={`icon-btn${worst ? ` health-${worst}` : ""}${open ? " on" : ""}`}
        data-health-btn={m.id}
        title={list.length ? list.map((c) => c.text).join("\n") : h ? `${m.label}: all good — ${h.os ?? ""}` : "Vitals not read yet"}
        aria-expanded={!!open}
        onClick={(e) => {
          e.stopPropagation();
          const r = e.currentTarget.getBoundingClientRect();
          setOpen(open ? null : { x: Math.min(r.left, window.innerWidth - 300), y: r.bottom + 4 });
        }}
      >
        <Icon name={list.length ? "alert-triangle" : "info"} size={14} />
        {list.length > 0 && <span className="health-count">{list.length}</span>}
      </button>
      {updateWhat !== null && <UpdatesDialog machine={m} what={updateWhat || undefined} onClose={() => setUpdateWhat(null)} onDone={() => void refresh()} />}
      {open && (
        <div className="row-menu health-pop" role="dialog" aria-label={`${m.label} vitals`} style={{ left: open.x, top: open.y }} onClick={(e) => e.stopPropagation()}>
          <div className="health-head">
            <b>{m.label}</b>
            <span className="dim">{h?.os}</span>
            <button type="button" className="icon-btn sm" title="Read again now" disabled={busy} onClick={() => void refresh()}>
              <Icon name="refresh-cw" size={12} className={busy ? "spin" : undefined} />
            </button>
          </div>
          {h ? (
            <dl className="health-rows">
              <dt>Claude Code</dt>
              <dd>
                {h.claude ?? "—"}
                {h.claude && l?.claude && newer(h.claude, l.claude) ? <span className="health-new"> → {l.claude}</span> : null}
              </dd>
              {h.codex && (
                <>
                  <dt>Codex</dt>
                  <dd>
                    {h.codex}
                    {l?.codex && newer(h.codex, l.codex) ? <span className="health-new"> → {l.codex}</span> : null}
                  </dd>
                </>
              )}
              {h.gemini && (
                <>
                  <dt>Gemini CLI</dt>
                  <dd>
                    {h.gemini}
                    {l?.gemini && newer(h.gemini, l.gemini) ? <span className="health-new"> → {l.gemini}</span> : null}
                  </dd>
                </>
              )}
              {h.kimi && (
                <>
                  <dt>Kimi Code</dt>
                  <dd>
                    {h.kimi}
                    {l?.kimi && newer(h.kimi, l.kimi) ? <span className="health-new"> → {l.kimi}</span> : null}
                  </dd>
                </>
              )}
              <dt>herdr</dt>
              <dd>
                {h.herdr ?? "—"}
                {h.herdr_server && h.herdr !== h.herdr_server ? <span className="health-new"> · server {h.herdr_server}</span> : null}
                {h.herdr && l?.herdr && newer(h.herdr, l.herdr) ? <span className="health-new"> → {l.herdr}</span> : null}
              </dd>
              {h.outdated_hooks?.length ? (
                <>
                  <dt>herdr hooks</dt>
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
                  <dt>herdr plugins</dt>
                  <dd>{h.plugins.map((p) => p.id).join(", ")}</dd>
                </>
              ) : null}
              <dt>remuda</dt>
              <dd>
                {h.remuda ?? (m.id === "local" ? serverVersion : "not deployed")}
                {m.id !== "local" && h.remuda ? <span className="dim"> · {h.remuda_running ? "running, terminals go through it" : "not running"}</span> : null}
                {remudaHave && l?.remuda && newer(remudaHave, l.remuda) ? <span className="health-new"> → {l.remuda}</span> : null}
              </dd>
              <dt>Disk</dt>
              <dd>
                {h.disk_free_gb.toFixed(0)} GB free · {h.disk_used_pct}% used
                {history && <Sparkline values={free} min={0} max={100} color="var(--ctp-green)" thresholds={DISK_THRESHOLDS} title={sparkTitle("Disk free", free, (v) => `${v.toFixed(0)}%`, span)} />}
              </dd>
              <dt>Load</dt>
              <dd>
                {h.load1.toFixed(2)}
                {h.cores ? ` on ${h.cores} cores` : ""}
                {h.uptime ? ` · up ${h.uptime}` : ""}
                {history && <Sparkline values={loads} min={loadRange.min} max={loadRange.max} color="var(--ctp-blue)" thresholds={loadThresholds(h.cores)} title={sparkTitle("Load", loads, (v) => v.toFixed(2), span)} />}
              </dd>
              <dt>Read</dt>
              <dd>
                {ago(h.at)}
                {span ? <span className="dim"> · {span} kept</span> : null}
              </dd>
            </dl>
          ) : (
            <div className="dim">Not read yet.</div>
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
          <div className="health-foot">
            <button type="button" className="linkish" onClick={() => update("")}>
              <Icon name="refresh-cw" size={12} /> All updates…
            </button>
          </div>
          {h?.error && h.claude && <div className="dim">{h.error}</div>}
        </div>
      )}
    </>
  );
}
