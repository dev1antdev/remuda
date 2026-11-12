// The Server place: the vitals of the host a remuda serving the page only runs on (remuda -server) — no machine of the
// fleet, so no ⓘ in the sidebar — its load, disk, memory, temperature, clock and throttling with the day's sparklines,
// its versions and its tailnet address, what wants a look, and the way to its updates and to every machine's.

import { useEffect, useState } from "react";
import { actions } from "@/app/store";
import UpdatesDialog from "@/features/updates/Updates";
import { errMsg } from "@/lib/text";
import { ago } from "@/lib/time";
import { Dialog, Icon, Notice } from "@/ui";
import { refreshHealth, type Health, type Machine } from "./api";
import { BodyRows } from "./BodyRows";
import { hostConcerns } from "./concerns";
import { Sparkline } from "./Sparkline";
import { DISK_THRESHOLDS, loadThresholds, rangeOf, sparkTitle, spanWords } from "./spark";
import { useRoster } from "./useRoster";
import { t, tn } from "@/lib/i18n";

export default function ServerPanel({ server, onClose }: { server: Machine; onClose: () => void }) {
  const { latest, version, tailscale, viewOnly } = useRoster();
  const [fresh, setFresh] = useState<Health | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [updates, setUpdates] = useState(false);
  useEffect(() => setFresh(null), [server.health?.at]);
  const h = fresh ?? server.health;
  const refresh = async () => {
    setBusy(true);
    setError("");
    try {
      setFresh((await refreshHealth(server.id)).health);
    } catch (e) {
      setError(errMsg(e));
    } finally {
      setBusy(false);
    }
  };
  const list = hostConcerns(h, latest, version, viewOnly);
  const history = h?.history && h.history.length >= 2 ? h.history : null;
  const span = history ? spanWords(history[0]!.at, history[history.length - 1]!.at) : "";
  const loads = history ? history.map((s) => s.load) : [];
  const free = history ? history.map((s) => s.disk_free) : [];
  const loadRange = rangeOf(loads, 0, h?.cores ?? 0);
  return (
    <Dialog title={t("Server · {name}", { name: server.label })} icon="server" size="md" onClose={onClose} className="server-panel">
      {updates && <UpdatesDialog machine={server} onClose={() => setUpdates(false)} onDone={() => void refresh()} />}
      <p className="modal-hint">{t("This remuda serves the page only: the machine it runs on is none of the cockpit's, its agents run on the machines in the sidebar.")}</p>
      {error && <Notice tone="error">{error}</Notice>}
      {list.length > 0 && (
        <ul className="server-concerns">
          {list.map((c) => (
            <li key={c.key} className={c.bad ? "is-bad" : "is-warn"}>
              <Icon name="alert-triangle" size={14} /> {c.text}
            </li>
          ))}
        </ul>
      )}
      {h ? (
        <dl className="health-rows server-rows">
          <dt>{t("System")}</dt>
          <dd>{[h.model, h.os, h.arch].filter(Boolean).join(" · ") || "—"}</dd>
          <dt>{t("Load")}</dt>
          <dd>
            {h.load1.toFixed(2)}
            {h.cores ? ` ${tn(h.cores, "on {n} core", "on {n} cores")}` : ""}
            {h.uptime ? ` · ${t("up {time}", { time: h.uptime })}` : ""}
            {history && <Sparkline values={loads} min={loadRange.min} max={loadRange.max} color="var(--ctp-blue)" thresholds={loadThresholds(h.cores)} title={sparkTitle(t("Load"), loads, (v) => v.toFixed(2), span)} />}
          </dd>
          <dt>{t("Disk")}</dt>
          <dd>
            {t("{free} GB free · {used}% used", { free: h.disk_free_gb.toFixed(0), used: h.disk_used_pct })}
            {history && <Sparkline values={free} min={0} max={100} color="var(--ctp-green)" thresholds={DISK_THRESHOLDS} title={sparkTitle(t("Disk free"), free, (v) => `${v.toFixed(0)}%`, span)} />}
          </dd>
          <BodyRows health={h} span={span} />
          <dt>remuda</dt>
          <dd>{version ?? "—"}</dd>
          <dt>herdr</dt>
          <dd>{h.herdr ? t("{version} installed, not used here", { version: h.herdr }) : t("not needed here")}</dd>
          {tailscale && (
            <>
              <dt>Tailscale</dt>
              <dd>{tailscale.state === "running" && tailscale.url ? tailscale.url : tailscale.state}</dd>
            </>
          )}
          <dt>{t("Read")}</dt>
          <dd>
            {ago(h.at)}
            {span ? <span className="dim"> · {t("{span} kept", { span })}</span> : null}
          </dd>
        </dl>
      ) : (
        <Notice tone="hint">{t("Not read yet.")}</Notice>
      )}
      {!viewOnly && (
        <div className="modal-actions">
          <button type="button" className="btn" disabled={busy} onClick={() => void refresh()}>
            <Icon name="refresh-cw" size={14} className={busy ? "spin" : undefined} /> {t("Read again now")}
          </button>
          <button type="button" className="btn" onClick={() => setUpdates(true)}>
            {t("Updates…")}
          </button>
          <button type="button" className="btn" onClick={() => actions.openDialog({ kind: "fleet-setup" })}>
            {t("Fleet setup…")}
          </button>
          <button type="button" className="btn primary" onClick={() => actions.openDialog({ kind: "update-everywhere" })}>
            {t("Update everywhere…")}
          </button>
        </div>
      )}
    </Dialog>
  );
}
