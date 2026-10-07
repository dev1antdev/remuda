// Search across every transcript on every machine: a query, a machine, a
// folder, a period — and the matching turns, newest first, each a jump to
// its place in the conversation.

import { useEffect, useMemo, useRef, useState } from "react";
import { searchTranscripts, type Machine, type SearchHit, type SearchResult } from "./api";
import { errMsg } from "./prefs";
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
  const input = useRef<HTMLInputElement>(null);
  const connected = machines.filter((m) => m.state === "connected");
  const label = (id: string) => machines.find((m) => m.id === id)?.label ?? id;

  // the folders known to the roster, on the chosen machine or all of them
  const folders = useMemo(() => {
    const seen = new Map<string, { name: string; machines: string[] }>();
    for (const m of machines) {
      if (machine !== "all" && m.id !== machine) continue;
      for (const s of m.sessions) {
        const d = s.dir;
        if (!d) continue;
        const cur = seen.get(d);
        if (cur) cur.machines.push(m.label);
        else seen.set(d, { name: s.name, machines: [m.label] });
      }
    }
    return [...seen.entries()]
      .sort((a, b) => a[1].name.localeCompare(b[1].name))
      .map(([d, v]) => ({ value: d, label: v.name, hint: machine === "all" && machines.length > 1 ? v.machines.join(", ") : d, icon: <Icon name="folder" size={14} /> }));
  }, [machines, machine]);

  // search as the query settles; a request still in flight is dropped
  useEffect(() => {
    const query = q.trim();
    if ([...query].length < 2) {
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
  }, [q, machine, dir, days]);

  return (
    <div className="modal-scrim" onClick={onClose}>
      <div className="modal search-dialog" onClick={(e) => e.stopPropagation()} onKeyDown={(e) => e.key === "Escape" && onClose()}>
        <div className="search-head">
          <Icon name="search" size={16} className="icon search-head-icon" />
          <input
            ref={input}
            autoFocus
            value={q}
            placeholder="Search every conversation…"
            spellCheck={false}
            onChange={(e) => setQ(e.target.value)}
          />
          {busy && <span className="search-busy" />}
          <button className="icon-btn" onClick={onClose} title="Close">
            <Icon name="x" size={16} />
          </button>
        </div>
        <div className="search-filters">
          {connected.length > 1 && (
            <Select
              value={machine}
              onChange={(v) => {
                setMachine(v);
                setDir("");
              }}
              options={[{ value: "all", label: "All machines" }, ...connected.map((m) => ({ value: m.id, label: m.label, icon: <span className={`machine-state is-${m.state}`} /> }))]}
            />
          )}
          <Select value={dir} onChange={setDir} options={[{ value: "", label: "All folders" }, ...folders]} />
          <Select value={days} onChange={setDays} options={PERIODS} />
        </div>
        <div className="search-list">
          {err && <div className="chat-error">{err}</div>}
          {res?.errors &&
            Object.entries(res.errors).map(([id, msg]) => (
              <div key={id} className="chat-error">
                {label(id)}: {msg}
              </div>
            ))}
          {!res && !err && (
            <div className="placeholder small">
              {[...q.trim()].length < 2 ? "What was said, by you or an agent, anywhere. Two characters to start." : "Searching…"}
            </div>
          )}
          {res && res.hits.length === 0 && <div className="placeholder small">Nothing matches.</div>}
          {res?.hits.map((h) => (
            <button key={`${h.machine}|${h.session}|${h.uuid}`} className="search-hit" title={h.dir} data-project={h.project} onClick={() => onOpen(h)}>
              <span className="search-hit-who">{h.role === "assistant" ? <Logo agent="claude" /> : <Icon name="user" size={15} />}</span>
              <span className="search-hit-body">
                <span className="search-hit-meta">
                  <b>{h.dir.split("/").filter(Boolean).pop() ?? h.dir}</b>
                  {machines.length > 1 && <span> · {label(h.machine)}</span>}
                  <span className="search-hit-when"> · {when(new Date(h.ts))}</span>
                </span>
                <span className="search-hit-snippet">{highlight(h.snippet, q.trim())}</span>
              </span>
            </button>
          ))}
          {res?.more && <div className="placeholder small">Only the newest 200 — narrow it down with a folder or a period.</div>}
        </div>
      </div>
    </div>
  );
}

function highlight(text: string, q: string) {
  if (!q) return text;
  const lower = text.toLowerCase();
  const needle = q.toLowerCase();
  const out: (string | JSX.Element)[] = [];
  let i = 0;
  let k = 0;
  for (;;) {
    const j = lower.indexOf(needle, i);
    if (j < 0) break;
    out.push(text.slice(i, j), <mark key={k++}>{text.slice(j, j + q.length)}</mark>);
    i = j + q.length;
  }
  out.push(text.slice(i));
  return out;
}

function when(d: Date): string {
  const now = new Date();
  const sameDay = d.toDateString() === now.toDateString();
  const time = d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  if (sameDay) return time;
  const days = Math.round((now.getTime() - d.getTime()) / 86400000);
  if (days < 7) return `${d.toLocaleDateString([], { weekday: "short" })} ${time}`;
  return d.toLocaleDateString([], { day: "numeric", month: "short", year: d.getFullYear() === now.getFullYear() ? undefined : "numeric" });
}
