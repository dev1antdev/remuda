// Usage: what the agents cost over a period, on every machine — by day,
// by project, by model, the sessions that cost the most. API-equivalent
// prices from Claude Code's own catalog; on a subscription a measure, not
// a bill.

import { useEffect, useMemo, useState } from "react";
import { fetchUsage, type Machine, type UsageMachine, type UsageTotals } from "./api";
import { errMsg } from "./prefs";
import Select from "./Select";
import Icon from "./icons";

const PERIODS = [
  { value: "1", label: "Today" },
  { value: "7", label: "Last 7 days" },
  { value: "30", label: "Last 30 days" },
  { value: "90", label: "Last 90 days" },
];

export const usd = (n: number) => (n >= 100 ? `$${Math.round(n)}` : n >= 10 ? `$${n.toFixed(1)}` : `$${n.toFixed(2)}`);
export const tok = (n: number) => (n >= 1e9 ? `${(n / 1e9).toFixed(1)}B` : n >= 1e6 ? `${(n / 1e6).toFixed(1)}M` : n >= 1e3 ? `${Math.round(n / 1e3)}k` : String(n));

export default function Usage({ machines, onClose, onOpenSession }: {
  machines: Machine[];
  onClose: () => void;
  onOpenSession: (machine: string, project: string, session: string, dir: string) => void;
}) {
  const [days, setDays] = useState("7");
  const [machine, setMachine] = useState("all");
  const [data, setData] = useState<{ since: string; machines: UsageMachine[] } | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const tz = Intl.DateTimeFormat().resolvedOptions().timeZone;
  useEffect(() => {
    let gone = false;
    setBusy(true);
    fetchUsage(Number(days), machine === "all" ? "" : machine, tz).then(
      (d) => {
        if (gone) return;
        setData(d);
        setErr("");
        setBusy(false);
      },
      (e) => {
        if (gone) return;
        setErr(errMsg(e));
        setBusy(false);
      },
    );
    return () => {
      gone = true;
    };
  }, [days, machine, tz]);
  const label = (id: string) => machines.find((m) => m.id === id)?.label ?? id;

  // everything summed over the machines that answered
  const sum = useMemo(() => {
    const zero = (): UsageTotals => ({ input: 0, output: 0, cache_read: 0, cache_write: 0, cache_write_1h: 0, cost: 0, turns: 0, unknown: 0 });
    const add = (a: UsageTotals, b: UsageTotals) => {
      a.input += b.input;
      a.output += b.output;
      a.cache_read += b.cache_read;
      a.cache_write += b.cache_write + (b.cache_write_1h ?? 0); // the 1h tier is a cache write too
      a.cost += b.cost;
      a.turns += b.turns;
      a.unknown += b.unknown;
    };
    const total = zero();
    const byDay = new Map<string, UsageTotals>();
    const byProject = new Map<string, UsageTotals & { machine: string; dir: string; project: string; sessions: number }>();
    const byModel = new Map<string, UsageTotals & { name: string }>();
    const sessions: { machine: string; project: string; session: string; dir: string; model: string; cost: number; turns: number; last: string }[] = [];
    const perMachine: { id: string; label: string; cost: number; turns: number; error?: string }[] = [];
    for (const m of data?.machines ?? []) {
      const r = m.report;
      if (!r) {
        perMachine.push({ id: m.id, label: m.label, cost: 0, turns: 0, error: m.error });
        continue;
      }
      perMachine.push({ id: m.id, label: m.label, cost: r.total.cost, turns: r.total.turns });
      add(total, r.total);
      for (const d of r.days) {
        const cur = byDay.get(d.date) ?? zero();
        add(cur, d);
        byDay.set(d.date, cur);
      }
      for (const p of r.projects) {
        const k = `${m.id}|${p.project}`;
        const cur = byProject.get(k) ?? { ...zero(), machine: m.id, dir: p.dir, project: p.project, sessions: 0 };
        add(cur, p);
        cur.sessions += p.sessions;
        byProject.set(k, cur);
      }
      for (const x of r.models) {
        const cur = byModel.get(x.model) ?? { ...zero(), name: x.name };
        add(cur, x);
        byModel.set(x.model, cur);
      }
      for (const s of r.sessions) sessions.push({ machine: m.id, project: s.project, session: s.session, dir: s.dir, model: s.model, cost: s.cost, turns: s.turns, last: s.last });
    }
    const dayList = [...byDay.entries()].sort((a, b) => a[0].localeCompare(b[0]));
    // every day of the period, empty ones too
    const n = Number(days);
    const all: { date: string; cost: number; turns: number }[] = [];
    for (let i = n - 1; i >= 0; i--) {
      const d = new Date();
      d.setDate(d.getDate() - i);
      const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
      const t = byDay.get(key);
      all.push({ date: key, cost: t?.cost ?? 0, turns: t?.turns ?? 0 });
    }
    return {
      total,
      days: all,
      dayList,
      projects: [...byProject.values()].sort((a, b) => b.cost - a.cost),
      models: [...byModel.entries()].map(([model, t]) => ({ model, ...t })).sort((a, b) => b.cost - a.cost),
      sessions: sessions.sort((a, b) => b.cost - a.cost).slice(0, 25),
      perMachine,
    };
  }, [data, days]);
  const maxDay = Math.max(1e-9, ...sum.days.map((d) => d.cost));
  const fmtDay = (iso: string) => {
    const [y, m, d] = iso.split("-").map(Number);
    return new Date(y!, m! - 1, d!).toLocaleDateString([], { day: "numeric", month: "short" });
  };

  return (
    <div className="modal-scrim" onClick={onClose}>
      <div className="modal usage" onClick={(e) => e.stopPropagation()}>
        <div className="schedules-head">
          <h2>
            <Icon name="chart" size={16} /> Usage
          </h2>
          <Select value={days} onChange={setDays} options={PERIODS} />
          {machines.filter((m) => m.state === "connected").length > 1 && (
            <Select
              value={machine}
              onChange={setMachine}
              options={[{ value: "all", label: "All machines" }, ...machines.filter((m) => m.state === "connected").map((m) => ({ value: m.id, label: m.label, icon: <span className={`machine-state is-${m.state}`} /> }))]}
            />
          )}
          {busy && <span className="search-busy" />}
          <button className="icon-btn" onClick={onClose} title="Close">
            <Icon name="x" size={16} />
          </button>
        </div>
        <p className="modal-hint">
          What the agents' transcripts say they cost, at Claude Code's own API prices — on a subscription a measure of where the work went, not a bill. Each
          machine reads its own history.
        </p>
        {err && <div className="chat-error">{err}</div>}
        {sum.perMachine.filter((m) => m.error).map((m) => (
          <div key={m.id} className="chat-error">
            {m.label}: {m.error}
          </div>
        ))}
        <div className="usage-cards">
          <div className="usage-card">
            <span className="usage-card-n">{usd(sum.total.cost)}</span>
            <span className="usage-card-l">API-equivalent</span>
          </div>
          <div className="usage-card">
            <span className="usage-card-n">{sum.total.turns.toLocaleString()}</span>
            <span className="usage-card-l">responses</span>
          </div>
          <div className="usage-card">
            <span className="usage-card-n">{tok(sum.total.output)}</span>
            <span className="usage-card-l">tokens written</span>
          </div>
          <div className="usage-card">
            <span className="usage-card-n">{tok(sum.total.input + sum.total.cache_read + sum.total.cache_write)}</span>
            <span className="usage-card-l">tokens read{sum.total.cache_read > 0 && ` · ${Math.round((100 * sum.total.cache_read) / Math.max(1, sum.total.input + sum.total.cache_read + sum.total.cache_write))}% from cache`}</span>
          </div>
        </div>
        {sum.total.unknown > 0 && <div className="placeholder small">{sum.total.unknown} responses came from a model with no price here and are counted, not priced.</div>}
        <div className="usage-chart" aria-label="cost per day">
          {sum.days.map((d) => (
            <div key={d.date} className="usage-bar" title={`${fmtDay(d.date)}: ${usd(d.cost)} · ${d.turns} responses`}>
              <div className="usage-bar-fill" style={{ height: `${Math.max(d.cost > 0 ? 3 : 0, (100 * d.cost) / maxDay)}%` }} />
              {(sum.days.length <= 14 || d === sum.days[0] || d === sum.days[sum.days.length - 1]) && <span className="usage-bar-l">{fmtDay(d.date)}</span>}
            </div>
          ))}
        </div>
        <div className="usage-tables">
          <section>
            <h3>Projects</h3>
            {sum.projects.length === 0 && <div className="placeholder small">Nothing in this period.</div>}
            {sum.projects.slice(0, 12).map((p) => (
              <div key={`${p.machine}|${p.project}`} className="usage-row" title={p.dir || p.project}>
                <span className="usage-row-name">
                  {p.dir ? (p.dir.split("/").filter(Boolean).pop() ?? p.project) : p.project}
                  {sum.perMachine.length > 1 && <span className="usage-row-sub"> · {label(p.machine)}</span>}
                </span>
                <span className="usage-row-meta">{p.sessions} session{p.sessions === 1 ? "" : "s"}</span>
                <span className="usage-row-cost">{usd(p.cost)}</span>
              </div>
            ))}
          </section>
          <section>
            <h3>Models</h3>
            {sum.models.map((m) => (
              <div key={m.model} className="usage-row">
                <span className="usage-row-name">{m.name}</span>
                <span className="usage-row-meta">{tok(m.output)} out · {tok(m.input + m.cache_read + m.cache_write)} in</span>
                <span className="usage-row-cost">{usd(m.cost)}</span>
              </div>
            ))}
            {sum.perMachine.length > 1 && (
              <>
                <h3>Machines</h3>
                {sum.perMachine.map((m) => (
                  <div key={m.id} className="usage-row">
                    <span className="usage-row-name">{m.label}</span>
                    <span className="usage-row-meta">{m.error ? "no report" : `${m.turns} responses`}</span>
                    <span className="usage-row-cost">{m.error ? "—" : usd(m.cost)}</span>
                  </div>
                ))}
              </>
            )}
          </section>
          <section className="usage-sessions">
            <h3>Sessions</h3>
            {sum.sessions.map((s) => (
              <button key={`${s.machine}|${s.session}`} className="usage-row usage-session" onClick={() => onOpenSession(s.machine, s.project, s.session, s.dir)} title="Open the transcript">
                <span className="usage-row-name">
                  {s.dir.split("/").filter(Boolean).pop() ?? s.project}
                  <span className="usage-row-sub"> · {s.session.slice(0, 8)}{sum.perMachine.length > 1 ? ` · ${label(s.machine)}` : ""}</span>
                </span>
                <span className="usage-row-meta">
                  {s.turns} responses · {ago(s.last)}
                </span>
                <span className="usage-row-cost">{usd(s.cost)}</span>
              </button>
            ))}
          </section>
        </div>
      </div>
    </div>
  );
}

function ago(iso: string): string {
  const m = Math.round((Date.now() - new Date(iso).getTime()) / 60000);
  if (m < 1) return "just now";
  if (m < 60) return `${m} min ago`;
  const h = Math.round(m / 60);
  if (h < 48) return `${h} h ago`;
  return `${Math.round(h / 24)} days ago`;
}
