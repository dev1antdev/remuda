// Search across every transcript on every machine: a query, a machine, a
// folder, a period — and the matching turns, newest first, each a jump to
// its place in the conversation.

import { useEffect, useMemo, useState, type ReactElement } from "react";
import { searchTranscripts, type Machine, type SearchHit, type SearchResult } from "./api";
import { basename, errMsg } from "./prefs";
import { connected, folderOptions, machineLabel, machineOptions } from "./options";
import { whenShort } from "./time";
import { Dialog, Notice } from "./ui";
import Select from "./Select";
import Icon from "./icons";
import Logo from "./Logo";

const PERIODS = [
  { value: "0", label: "Any time" },
  { value: "1", label: "Today" },
  { value: "7", label: "Last 7 days" },
  { value: "30", label: "Last 30 days" },
];

export default function Search({ machines, onClose, onOpen }: { machines: Machine[]; onClose: () => void; onOpen: (hit: SearchHit) => void }) {
  const [q, setQ] = useState("");
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
        <input autoFocus value={q} placeholder="Search every conversation…" aria-label="Search every conversation" spellCheck={false} onChange={(e) => setQ(e.target.value)} />
        {busy && <span className="search-busy" />}
        <button type="button" className="icon-btn" onClick={onClose} title="Close" aria-label="Close">
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
            options={machineOptions(machines, { all: "All machines" })}
          />
        )}
        <Select value={dir} onChange={setDir} options={[{ value: "", label: "All folders" }, ...folders]} />
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
        {!res && !err && <div className="empty">{short ? "What was said, by you or an agent, anywhere. Two characters to start." : "Searching…"}</div>}
        {res && res.hits.length === 0 && <div className="empty">Nothing matches.</div>}
        {res?.hits.map((h) => (
          <button key={`${h.machine}|${h.session}|${h.uuid}`} type="button" className="search-hit" title={h.dir} data-project={h.project} onClick={() => onOpen(h)}>
            <span className="search-hit-who">{h.role === "assistant" ? <Logo agent="claude" /> : <Icon name="user" size={14} />}</span>
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
        {res?.more && <Notice tone="hint">Only the newest 200 — narrow it down with a folder or a period.</Notice>}
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
