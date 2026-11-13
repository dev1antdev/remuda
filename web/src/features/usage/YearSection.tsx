// YearSection is the Spend tab's year: the last 365 days as one SVG of squares, toned by spend or responses, weeks as
// columns, today at the right end, and the year's figures under it. A hover, a tap or the arrow keys name a day. The
// server keeps the days that ended, so a look reads only today and the days it never saw; while it reads, the
// squares stand empty and the figures say "…".

import { useEffect, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import type { Machine } from "@/features/roster/api";
import { useSharing } from "@/features/roster/useSharing";
import { folderOf } from "@/features/roster/workspaces";
import { t, tc, tn } from "@/lib/i18n";
import { usePref } from "@/lib/prefs";
import { useQuery } from "@/lib/query";
import { fmtDay, whenShort } from "@/lib/time";
import { fetchUsageYear } from "./api";
import { tok, usd } from "./format";
import { cellAt, dayWords, heatGrid, HEAT_KEY, lengthWords, LEVELS, monthWords, readingWords, valueOf, weekdayWords, yearView, hiddenFolder, type HeatCell, type HeatMetric } from "./heatmap";

const CELL = 10;
const GAP = 3;
const PITCH = CELL + GAP;
const LEFT = 28; // the weekdays' column
const TOP = 16; // the months' row
const CUT = 1.5; // a square's corners, cut so it reads as rounded at this size

const squarePath = (cells: HeatCell[]) =>
  cells
    .map((c) => {
      const x = LEFT + c.col * PITCH;
      const y = TOP + c.row * PITCH;
      const side = CELL - 2 * CUT;
      return `M${x + CUT} ${y}h${side}l${CUT} ${CUT}v${side}l${-CUT} ${CUT}h${-side}l${-CUT} ${-CUT}v${-side}z`;
    })
    .join("");

export default function YearSection({ machines, machine }: { machines: Machine[]; machine: string }) {
  const tz = Intl.DateTimeFormat().resolvedOptions().timeZone;
  const [metric, setMetric] = usePref<HeatMetric>(HEAT_KEY, "cost");
  const [pace, setPace] = useState(60_000);
  const { data, error } = useQuery(() => fetchUsageYear(tz), [tz], { every: pace, keep: true });
  const busy = !!data?.machines.some((m) => m.busy);
  useEffect(() => setPace(busy ? 2000 : 60_000), [busy]);
  const share = useSharing();
  const view = useMemo(() => {
    if (!data) return null;
    const sessions = new Map(machines.map((m) => [m.id, m.sessions.map((s) => ({ dir: s.dir ?? "", folder: folderOf(s) }))]));
    return yearView(data, machine, (machineId, dir) => hiddenFolder(share.pref, sessions.get(machineId) ?? [], machineId, dir), metric);
  }, [data, machine, machines, share.pref, metric]);
  const grid = useMemo(() => (data && view ? heatGrid(data.from, data.to, (date) => valueOf(view.days.get(date), metric)) : null), [data, view, metric]);
  const [picked, setPicked] = useState<string | null>(null);
  const scroll = useRef<HTMLDivElement>(null);
  const width = LEFT + (grid?.weeks ?? 53) * PITCH - GAP;
  const height = TOP + 7 * PITCH - GAP;
  useLayoutEffect(() => {
    if (scroll.current) scroll.current.scrollLeft = scroll.current.scrollWidth;
  }, [grid?.weeks]);
  const byLevel = useMemo(() => Array.from({ length: LEVELS }, (_, level) => squarePath(grid?.cells.filter((c) => c.level === level) ?? [])), [grid]);
  const today = grid?.cells[grid.cells.length - 1];
  const shownCell = picked && grid ? grid.cells.find((c) => c.date === picked) : undefined;
  const reading = view?.reading ?? 0;
  const waiting = !view || reading > 0;
  const pickAt = (clientX: number, clientY: number, svg: SVGSVGElement) => {
    if (!grid) return;
    const box = svg.getBoundingClientRect();
    const col = Math.floor((clientX - box.left - LEFT) / PITCH);
    const row = Math.floor((clientY - box.top - TOP) / PITCH);
    setPicked(cellAt(grid, col, row)?.date ?? null);
  };
  const onKey = (e: KeyboardEvent) => {
    if (!grid || !today || e.altKey || e.ctrlKey || e.metaKey) return;
    const steps: Record<string, [number, number]> = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] };
    const step = steps[e.key];
    if (e.key === "Escape" && picked) {
      e.stopPropagation();
      setPicked(null);
      return;
    }
    if (!step) return;
    e.preventDefault();
    const from = shownCell ?? today;
    setPicked((cellAt(grid, from.col + step[0], from.row + step[1]) ?? from).date);
  };
  const figure = (label: string, value: string, unit = "") => (
    <div className="usage-year-figure">
      <span className="usage-year-label">{label}</span>
      <span className="usage-year-value">
        {waiting ? "…" : value}
        {!waiting && unit && <span className="usage-year-unit">{unit}</span>}
      </span>
    </div>
  );
  const f = view?.figures;
  const status = !view ? t("Reading the year…") : reading > 0 ? readingWords(reading) : view.readAt ? t("Through today, read at {time}", { time: whenShort(view.readAt) }) : t("Through today");
  return (
    <section className="usage-year">
      <div className="usage-year-head">
        <h3>{t("The last year")}</h3>
        <div className="fv-tabs usage-heat-metric" role="tablist" aria-label={t("What tones a day")}>
          <button type="button" role="tab" aria-selected={metric === "cost"} className={metric === "cost" ? "on" : ""} onClick={() => setMetric("cost")}>
            {t("Spend")}
          </button>
          <button type="button" role="tab" aria-selected={metric === "turns"} className={metric === "turns" ? "on" : ""} onClick={() => setMetric("turns")}>
            {t("Responses")}
          </button>
        </div>
        <span className="usage-year-status">{status}</span>
      </div>
      {error && !data && <div className="usage-year-note is-error">{error}</div>}
      <div className="usage-heat-scroll" ref={scroll} tabIndex={0} role="group" aria-label={t("A square a day for the last year; the arrow keys walk the days")} onKeyDown={onKey}>
        <svg
          className={`usage-heat${waiting ? " is-reading" : ""}`}
          width={width}
          height={height}
          viewBox={`0 0 ${width} ${height}`}
          onPointerMove={(e) => e.pointerType === "mouse" && pickAt(e.clientX, e.clientY, e.currentTarget)}
          onPointerLeave={(e) => e.pointerType === "mouse" && setPicked(null)}
          onClick={(e) => pickAt(e.clientX, e.clientY, e.currentTarget)}
        >
          {grid?.months.map((m) => (
            <text key={m.date} className="usage-heat-month" x={LEFT + m.col * PITCH} y={TOP - 5}>
              {monthWords(m.date)}
            </text>
          ))}
          {[0, 2, 4].map((row) => (
            <text key={row} className="usage-heat-weekday" x={0} y={TOP + row * PITCH + CELL - 1}>
              {weekdayWords(row)}
            </text>
          ))}
          {byLevel.map((d, level) => d && <path key={level} className={`usage-heat-l${level}`} d={d} />)}
          {today && <rect className="usage-heat-today" x={LEFT + today.col * PITCH - 0.5} y={TOP + today.row * PITCH - 0.5} width={CELL + 1} height={CELL + 1} rx={2} />}
          {shownCell && <rect className="usage-heat-picked" x={LEFT + shownCell.col * PITCH - 1} y={TOP + shownCell.row * PITCH - 1} width={CELL + 2} height={CELL + 2} rx={2.5} />}
        </svg>
      </div>
      <div className="usage-heat-line" aria-live="polite">
        {shownCell ? (
          <span className="usage-heat-day">{dayWords(shownCell.date, view?.days.get(shownCell.date))}</span>
        ) : (
          <span className="usage-heat-legend">
            {tc("heat", "Less")}
            {Array.from({ length: LEVELS }, (_, level) => (
              <span key={level} className={`usage-heat-swatch usage-heat-l${level}`} />
            ))}
            {tc("heat", "More")}
          </span>
        )}
      </div>
      <div className="usage-year-figures">
        {figure(t("Favourite model"), f?.favourite ?? "—")}
        {figure(t("Total tokens"), tok(f?.tokens ?? 0))}
        {figure(t("Sessions"), (f?.sessions ?? 0).toLocaleString())}
        {figure(t("Longest session"), f?.longest ? lengthWords(f.longest) : "—")}
        {figure(t("Active days"), String(f?.active ?? 0), ` / ${f?.span ?? 365}`)}
        {figure(t("Current streak"), tn(f?.current ?? 0, "{n} day", "{n} days"))}
        {figure(t("Longest streak"), tn(f?.best ?? 0, "{n} day", "{n} days"))}
        {figure(t("Busiest day"), f?.busiest ? `${fmtDay(f.busiest.date)} · ${metric === "cost" ? usd(f.busiest.cost) : tn(f.busiest.turns, "{n} response", "{n} responses")}` : "—")}
      </div>
      {view && view.hidden > 0 && <div className="usage-year-note">{tn(view.hidden, "{n} folder hidden while sharing the screen is left out", "{n} folders hidden while sharing the screen are left out")}</div>}
      {view?.away.map((label) => (
        <div key={`away-${label}`} className="usage-year-note">
          {t("{machine} is away: its days as they were last read", { machine: share.redact(label) })}
        </div>
      ))}
      {view?.failed.map((m) => (
        <div key={`failed-${m.label}`} className="usage-year-note is-error">
          {t("{machine}: its days could not be read — {error}", { machine: share.redact(m.label), error: m.error })}
        </div>
      ))}
    </section>
  );
}
