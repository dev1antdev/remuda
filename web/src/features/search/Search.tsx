// Search across every transcript on every machine: a query, a machine, a
// folder, a period — and the matching turns, newest first, each a jump to
// its place in the conversation.

import { useEffect, useMemo, useState, type ReactElement } from "react";
import { searchTranscripts, type SearchHit, type SearchResult } from "@/features/search/api";
import type { Machine } from "@/features/roster/api";
import { basename, errMsg } from "@/lib/text";
import { connected, folderOptions, machineLabel, machineOptions } from "@/features/roster/options";
import { whenShort } from "@/lib/time";
import { Dialog, Notice } from "@/ui";
import Select from "@/ui/Select";
import Icon from "@/ui/icons";
import Logo from "@/ui/Logo";
import { t } from "@/lib/i18n";

const PERIODS = [
  { value: "0", label: t("Any time") },
  { value: "1", label: t("Today") },
  { value: "7", label: t("Last 7 days") },
  { value: "30", label: t("Last 30 days") },
];

export default function Search({ machines, initial = "", onClose, onOpen }: { machines: Machine[]; initial?: string; onClose: () => void; onOpen: (hit: SearchHit) => void }) {
  const [q, setQ] = useState(initial);
  const [machine, setMachine] = useState("all");
  const [dir, setDir] = useState("");
  const [days, setDays] = useState("0");
  const [res, setRes] = useState<SearchResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const folders = useMemo(() => folderOptions(machines, machine), [machines, machine]);
  const query = q.trim();
  const short = [...query].length < 2;

  useEffect(() => {
    if (short) {
      setRes(null);
      setErr("");
      setBusy(false);
      return;
    }
    const ctl = new AbortController();
    setBusy(true);
    const t = setTimeout(() => {
      searchTranscripts(query, { machine: machine === "all" ? "" : machine, dir, days: Number(days) || 0 }, ctl.signal)
        .then(
          (r) => {
            setRes(r);
            setErr("");
          },
          (e) => {
            if (ctl.signal.aborted) return;
            setErr(errMsg(e));
          },
        )
        .finally(() => {
          if (!ctl.signal.aborted) setBusy(false);
        });
    }, 350);
    return () => {
      clearTimeout(t);
      ctl.abort();
    };
  }, [query, short, machine, dir, days]);

  return (
    <Dialog onClose={onClose} className="search-dialog">
      <div className="search-head">
        <Icon name="search" size={16} className="search-head-icon" />
        <input autoFocus value={q} placeholder={t("Search every conversation…")} aria-label={t("Search every conversation")} spellCheck={false} onChange={(e) => setQ(e.target.value)} />
        {busy && <span className="search-busy" />}
        <button type="button" className="icon-btn" onClick={onClose} title={t("Close")} aria-label={t("Close")}>
          <Icon name="x" size={16} />
        </button>
      </div>
      <div className="search-filters">
        {connected(machines).length > 1 && (
          <Select
            value={machine}
            onChange={(v) => {
              setMachine(v);
              setDir("");
            }}
            options={machineOptions(machines, { all: t("All machines") })}
          />
        )}
        <Select value={dir} onChange={setDir} options={[{ value: "", label: t("All folders") }, ...folders]} />
        <Select value={days} onChange={setDays} options={PERIODS} />
      </div>
      <div className="search-list">
        {err && <Notice tone="error">{err}</Notice>}
        {res?.errors &&
          Object.entries(res.errors).map(([id, msg]) => (
            <Notice key={id} tone="error">
              {machineLabel(machines, id)}: {msg}
            </Notice>
          ))}
        {!res && !err && <div className="empty">{short ? t("What was said, by you or an agent, anywhere. Two characters to start.") : t("Searching…")}</div>}
        {res && res.hits.length === 0 && <div className="empty">{t("Nothing matches.")}</div>}
        {res?.hits.map((h) => (
          <button key={`${h.machine}|${h.session}|${h.uuid}`} type="button" className="search-hit" title={h.dir} data-project={h.project} onClick={() => onOpen(h)}>
            <span className="search-hit-who">{h.role === "assistant" ? <Logo agent={h.agent || "claude"} /> : <Icon name="user" size={14} />}</span>
            <span className="search-hit-body">
              <span className="search-hit-meta">
                <b>{basename(h.dir)}</b>
                {machines.length > 1 && <span> · {machineLabel(machines, h.machine)}</span>}
                <span className="search-hit-when"> · {whenShort(h.ts)}</span>
              </span>
              <span className="search-hit-snippet">{highlight(h.snippet, query)}</span>
            </span>
          </button>
        ))}
        {res?.more && <Notice tone="hint">{t("Only the newest 200 — narrow it down with a folder or a period.")}</Notice>}
      </div>
    </Dialog>
  );
}

// highlight marks the query's occurrences; none when lower-casing moved the offsets (a dotted İ grows), the snippet then shows plain
function highlight(text: string, q: string) {
  const lower = text.toLowerCase();
  const needle = q.toLowerCase();
  if (!needle || lower.length !== text.length) return text;
  const out: (string | ReactElement)[] = [];
  let i = 0;
  let k = 0;
  for (;;) {
    const j = lower.indexOf(needle, i);
    if (j < 0) break;
    out.push(text.slice(i, j), <mark key={k++}>{text.slice(j, j + needle.length)}</mark>);
    i = j + needle.length;
  }
  out.push(text.slice(i));
  return out;
}
