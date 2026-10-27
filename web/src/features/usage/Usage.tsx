// Usage: what the agents cost over a period, on every machine — by day, by
// project, by model, the sessions that cost the most. API-equivalent prices
// from Claude Code's own catalog; on a subscription a measure, not a bill.

import { useMemo, useState } from "react";
import { actions } from "@/app/store";
import { fetchUsage, type BudgetStatus, type UsageTotals } from "@/features/usage/api";
import type { Machine } from "@/features/roster/api";
import { useRoster } from "@/features/roster/useRoster";
import { basename } from "@/lib/text";
import { connected, machineLabel, machineOptions } from "@/features/roster/options";
import { ago, fmtDay } from "@/lib/time";
import { Dialog, Icon, Notice, Tip } from "@/ui";
import { useQuery } from "@/lib/query";
import Select from "@/ui/Select";
import { budgetTone, budgetWords, missingWords, ratio, scopeWords } from "./budgets";
import { t } from "@/lib/i18n";

const PERIODS = [
  { value: "1", label: t("Today") },
  { value: "7", label: t("Last 7 days") },
  { value: "30", label: t("Last 30 days") },
  { value: "90", label: t("Last 90 days") },
];

import { tok, usd } from "./format";
import { plural } from "@/lib/text";

export default function Usage({ machines, onClose, onOpenSession }: {
  machines: Machine[];
  onClose: () => void;
  onOpenSession: (machine: string, project: string, session: string, dir: string) => void;
}) {
  const [days, setDays] = useState("7");
  const [machine, setMachine] = useState("all");
  const { budgets } = useRoster();
  const tz = Intl.DateTimeFormat().resolvedOptions().timeZone;
  const { data, error, loading } = useQuery(() => fetchUsage(Number(days), machine === "all" ? "" : machine, tz), [days, machine, tz], { keep: true });
  const several = connected(machines).length > 1;

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
      projects: [...byProject.values()].sort((a, b) => b.cost - a.cost),
      models: [...byModel.entries()].map(([model, t]) => ({ model, ...t })).sort((a, b) => b.cost - a.cost),
      sessions: sessions.sort((a, b) => b.cost - a.cost).slice(0, 25),
      perMachine,
    };
  }, [data, days]);
  const maxDay = Math.max(1e-9, ...sum.days.map((d) => d.cost));
  const read = sum.total.input + sum.total.cache_read + sum.total.cache_write;
  const who = (id: string) => (sum.perMachine.length > 1 ? ` · ${machineLabel(machines, id)}` : "");

  return (
    <Dialog
      title={t("Usage")}
      icon="chart"
      size="lg"
      className="usage"
      onClose={onClose}
      controls={
        <>
          <Select value={days} onChange={setDays} options={PERIODS} />
          {several && <Select value={machine} onChange={setMachine} options={machineOptions(machines, { all: t("All machines") })} />}
          {loading && <span className="search-busy" />}
        </>
      }
    >
      <Notice tone="hint">
        What the agents' transcripts say they cost, at Claude Code's own API prices — on a subscription a measure of where the work went, not a bill. Each
        machine reads its own history.
      </Notice>
      {error && <Notice tone="error">{error}</Notice>}
      <Budgets budgets={budgets ?? []} machines={machines} />
      {sum.perMachine
        .filter((m) => m.error)
        .map((m) => (
          <Notice key={m.id} tone="error">
            {m.label}: {m.error}
          </Notice>
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
          <span className="usage-card-l">{t("tokens written")}</span>
        </div>
        <div className="usage-card">
          <span className="usage-card-n">{tok(read)}</span>
          <span className="usage-card-l">tokens read{sum.total.cache_read > 0 && ` · ${Math.round((100 * sum.total.cache_read) / Math.max(1, read))}% from cache`}</span>
        </div>
      </div>
      {sum.total.unknown > 0 && <Notice tone="hint">{sum.total.unknown} responses came from a model with no price here and are counted, not priced.</Notice>}
      <div className="usage-chart" aria-label={t("cost per day")}>
        {sum.days.map((d) => (
          <div key={d.date} className="usage-bar" title={`${fmtDay(d.date)}: ${usd(d.cost)} · ${plural(d.turns, "response")}`}>
            <div className="usage-bar-fill" style={{ height: `${Math.max(d.cost > 0 ? 3 : 0, (100 * d.cost) / maxDay)}%` }} />
            {(sum.days.length <= 14 || d === sum.days[0] || d === sum.days[sum.days.length - 1]) && <span className="usage-bar-l">{fmtDay(d.date)}</span>}
          </div>
        ))}
      </div>
      <div className="usage-tables">
        <section>
          <h3>{t("Projects")}</h3>
          {sum.projects.length === 0 && <div className="empty">{t("Nothing in this period.")}</div>}
          {sum.projects.slice(0, 12).map((p) => (
            <div key={`${p.machine}|${p.project}`} className="usage-row" title={p.dir || p.project}>
              <span className="usage-row-name">
                {p.dir ? basename(p.dir) : p.project}
                <span className="usage-row-sub">{who(p.machine)}</span>
              </span>
              <span className="usage-row-meta">{plural(p.sessions, "session")}</span>
              <span className="usage-row-cost">{usd(p.cost)}</span>
            </div>
          ))}
        </section>
        <section>
          <h3>{t("Models")}</h3>
          {sum.models.map((m) => (
            <div key={m.model} className="usage-row">
              <span className="usage-row-name">{m.name}</span>
              <span className="usage-row-meta">
                {tok(m.output)} out · {tok(m.input + m.cache_read + m.cache_write)} in
              </span>
              <span className="usage-row-cost">{usd(m.cost)}</span>
            </div>
          ))}
          {sum.perMachine.length > 1 && (
            <>
              <h3>{t("Machines")}</h3>
              {sum.perMachine.map((m) => (
                <div key={m.id} className="usage-row">
                  <span className="usage-row-name">{m.label}</span>
                  <span className="usage-row-meta">{m.error ? t("no report") : plural(m.turns, "response")}</span>
                  <span className="usage-row-cost">{m.error ? "—" : usd(m.cost)}</span>
                </div>
              ))}
            </>
          )}
        </section>
        <section className="usage-sessions">
          <h3>{t("Sessions")}</h3>
          {sum.sessions.map((s) => (
            <button key={`${s.machine}|${s.session}`} type="button" className="usage-row" onClick={() => onOpenSession(s.machine, s.project, s.session, s.dir)} title={t("Open the transcript")}>
              <span className="usage-row-name">
                {s.dir ? basename(s.dir) : s.project}
                <span className="usage-row-sub">
                  {" "}
                  · {s.session.slice(0, 8)}
                  {who(s.machine)}
                </span>
              </span>
              <span className="usage-row-meta">
                {plural(s.turns, "response")} · {ago(s.last)}
              </span>
              <span className="usage-row-cost">{usd(s.cost)}</span>
            </button>
          ))}
        </section>
      </div>
    </Dialog>
  );
}

// Budgets is the panel's first section: every limit with a thin bar of its period's spend, and the way to add one.
function Budgets({ budgets, machines }: { budgets: BudgetStatus[]; machines: Machine[] }) {
  const label = (id: string) => machineLabel(machines, id);
  return (
    <section className="budgets">
      <h3>
        {t("Budgets")}
        <button type="button" className="btn sm" onClick={() => actions.openDialog({ kind: "budget", budget: null })}>
          <Icon name="plus" size={12} /> Add a budget
        </button>
      </h3>
      {budgets.length === 0 && <div className="empty">{t("No budgets yet: a limit per day, week or month, on everything or one machine or folder. The usage pill turns amber at 80 %, a push comes at 80 % and at the limit.")}</div>}
      {budgets.map((b) => {
        const r = ratio(b);
        const tone = budgetTone(r);
        const missing = missingWords(b, label);
        return (
          <div key={b.id} className={`budget-row${tone ? ` is-${tone}` : ""}`} title={missing || undefined}>
            <span className="budget-scope">
              {scopeWords(b, label)}
              {b.note && <span className="usage-row-sub"> · {b.note}</span>}
              {missing && <span className="usage-row-sub"> · {b.missing?.length === 1 ? t("1 machine") : `${b.missing?.length} machines`} not counted</span>}
            </span>
            <span className="budget-bar" aria-hidden="true">
              <span style={{ width: `${Math.min(100, 100 * r)}%` }} />
            </span>
            <span className="budget-words">{budgetWords(b)}</span>
            <Tip text="Edit this budget">
              <button type="button" className="icon-btn sm" aria-label={t("Edit this budget")} onClick={() => actions.openDialog({ kind: "budget", budget: b })}>
                <Icon name="edit" size={14} />
              </button>
            </Tip>
          </div>
        );
      })}
    </section>
  );
}
