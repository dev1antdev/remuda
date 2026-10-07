// Schedules: prompts sent to an agent on a timetable. A list with the next
// and last run, a switch per row, "Run now", and a dialog to make one — a
// machine, a workspace folder, when (presets that write cron, or cron by
// hand), and the prompt itself.

import { useEffect, useMemo, useState } from "react";
import { deleteSchedule, fetchSchedules, putSchedule, runSchedule, type Machine, type Schedule } from "./api";
import { errMsg } from "./prefs";
import Select from "./Select";
import Icon from "./icons";

const PRESETS: { id: string; label: string; cron: (hh: number, mm: number) => string }[] = [
  { id: "daily", label: "Every day", cron: (h, m) => `${m} ${h} * * *` },
  { id: "weekdays", label: "Weekdays", cron: (h, m) => `${m} ${h} * * 1-5` },
  { id: "hourly", label: "Every hour", cron: (_h, m) => `${m} * * * *` },
  { id: "custom", label: "Cron…", cron: (h, m) => `${m} ${h} * * *` },
];

export default function Schedules({ machines, onClose, ask }: {
  machines: Machine[];
  onClose: () => void;
  ask: (text: string, label: string, run: () => void) => void;
}) {
  const [items, setItems] = useState<Schedule[] | null>(null);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState<Partial<Schedule> | null>(null);
  const [tick, setTick] = useState(0);
  useEffect(() => {
    fetchSchedules().then(setItems, (e) => setError(errMsg(e)));
  }, [tick]);
  const refresh = () => setTick((t) => t + 1);
  const machineLabel = (id: string) => machines.find((m) => m.id === id)?.label ?? id;

  return (
    <div className="modal-scrim" onClick={onClose}>
      <div className="modal schedules" onClick={(e) => e.stopPropagation()}>
        <div className="schedules-head">
          <h2>
            <Icon name="calendar" size={16} /> Schedules
          </h2>
          <button className="sidebar-head-btn" onClick={() => setEditing({ enabled: true })}>
            <Icon name="plus" size={13} /> New
          </button>
          <button className="icon-btn" onClick={onClose} title="Close">
            <Icon name="x" size={16} />
          </button>
        </div>
        <p className="modal-hint">
          A prompt sent to the agent running in a folder, on a timetable — in this machine's local time. A minute with no agent there, or an agent busy with a
          question, is skipped and noted.
        </p>
        {error && <div className="chat-error">{error}</div>}
        {items && items.length === 0 && !editing && <div className="placeholder small">No schedules yet.</div>}
        <div className="schedules-list">
          {items?.map((s) => (
            <div key={s.id} className={`schedule${s.enabled ? "" : " off"}`}>
              <input type="checkbox" checked={s.enabled} title={s.enabled ? "On" : "Off"} onChange={(e) => void putSchedule({ ...s, enabled: e.target.checked }).then(refresh, (er) => setError(errMsg(er)))} />
              <div className="schedule-copy">
                <div className="schedule-label">{s.label}</div>
                <div className="schedule-sub">
                  <code>{s.cron}</code> · {machineLabel(s.machine)} · {s.dir.split("/").filter(Boolean).pop()}
                  {s.last_run && s.last_run !== "0001-01-01T00:00:00Z" && (
                    <>
                      {" · "}
                      <span className={s.last_ok ? "ok" : "bad"}>
                        last {ago(new Date(s.last_run))}: {s.last_result}
                      </span>
                    </>
                  )}
                </div>
              </div>
              <button className="icon-btn" title="Run now" onClick={() => void runSchedule(s.id ?? "").then(refresh, (er) => setError(errMsg(er)))}>
                <Icon name="zap" size={14} />
              </button>
              <button className="icon-btn" title="Edit" onClick={() => setEditing(s)}>
                <Icon name="sliders" size={14} />
              </button>
              <button
                className="icon-btn"
                title="Delete"
                onClick={() => ask(`Delete the schedule “${s.label}”?`, "Delete", () => void deleteSchedule(s.id ?? "").then(refresh, (er) => setError(errMsg(er))))}
              >
                <Icon name="trash" size={14} />
              </button>
            </div>
          ))}
        </div>
        {editing && (
          <Editor
            machines={machines}
            value={editing}
            onClose={() => setEditing(null)}
            onSaved={() => {
              setEditing(null);
              refresh();
            }}
          />
        )}
      </div>
    </div>
  );
}

function Editor({ machines, value, onClose, onSaved }: { machines: Machine[]; value: Partial<Schedule>; onClose: () => void; onSaved: () => void }) {
  const connected = machines.filter((m) => m.state === "connected");
  const [machine, setMachine] = useState(value.machine ?? connected[0]?.id ?? "");
  const folders = useMemo(() => {
    const m = machines.find((x) => x.id === machine);
    const seen = new Map<string, string>();
    for (const s of m?.sessions ?? []) if (s.dir && !seen.has(s.dir)) seen.set(s.dir, s.name);
    return [...seen.entries()].map(([dir, name]) => ({ value: dir, label: name, hint: dir, icon: <Icon name="folder" size={14} /> }));
  }, [machines, machine]);
  const [dir, setDir] = useState(value.dir ?? "");
  const [label, setLabel] = useState(value.label ?? "");
  const [prompt, setPrompt] = useState(value.prompt ?? "");
  const initial = parseCron(value.cron ?? "0 9 * * 1-5");
  const [preset, setPreset] = useState(initial.preset);
  const [hh, setHH] = useState(initial.hh);
  const [mm, setMM] = useState(initial.mm);
  const [cron, setCron] = useState(value.cron ?? "0 9 * * 1-5");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  useEffect(() => {
    if (!dir && folders[0]) setDir(folders[0].value);
  }, [folders, dir]);
  useEffect(() => {
    if (preset !== "custom") setCron(PRESETS.find((p) => p.id === preset)!.cron(hh, mm));
  }, [preset, hh, mm]);
  const pad = (n: number) => String(n).padStart(2, "0");

  return (
    <form
      className="schedule-editor"
      onSubmit={(e) => {
        e.preventDefault();
        setBusy(true);
        setErr("");
        putSchedule({ id: value.id, machine, dir, label, cron, prompt, enabled: value.enabled ?? true })
          .then(onSaved, (er) => setErr(errMsg(er)))
          .finally(() => setBusy(false));
      }}
    >
      <div className="schedule-row">
        {connected.length > 1 && (
          <label>
            Machine
            <Select value={machine} onChange={setMachine} options={connected.map((m) => ({ value: m.id, label: m.label, icon: <span className={`machine-state is-${m.state}`} /> }))} />
          </label>
        )}
        <label className="grow">
          Folder
          <Select value={dir} onChange={setDir} placeholder="Pick a workspace" options={folders.some((f) => f.value === dir) || !dir ? folders : [{ value: dir, label: dir, icon: <Icon name="folder" size={14} /> }, ...folders]} />
        </label>
      </div>
      <div className="schedule-row">
        <label>
          When
          <Select value={preset} onChange={setPreset} options={PRESETS.map((p) => ({ value: p.id, label: p.label }))} />
        </label>
        {preset !== "hourly" && preset !== "custom" && (
          <label>
            At
            <input type="time" value={`${pad(hh)}:${pad(mm)}`} onChange={(e) => { const [h, m] = e.target.value.split(":").map(Number); if (h !== undefined && !Number.isNaN(h)) { setHH(h); setMM(m && !Number.isNaN(m) ? m : 0); } }} />
          </label>
        )}
        {preset === "hourly" && (
          <label>
            Minute
            <input type="number" min={0} max={59} value={mm} onChange={(e) => setMM(Math.max(0, Math.min(59, Number(e.target.value) || 0)))} />
          </label>
        )}
        <label className="grow">
          Cron
          <input value={cron} onChange={(e) => { setCron(e.target.value); setPreset("custom"); }} placeholder="minute hour day month weekday" spellCheck={false} />
        </label>
      </div>
      <label>
        Name <span className="dim">(optional)</span>
        <input value={label} onChange={(e) => setLabel(e.target.value)} placeholder="Morning status" />
      </label>
      <label>
        Prompt
        <textarea rows={4} value={prompt} onChange={(e) => setPrompt(e.target.value)} placeholder="Run the tests and sum up what failed. If everything passes, say so in one line." required />
      </label>
      {err && <div className="chat-error">{err}</div>}
      <div className="modal-actions">
        <button type="button" onClick={onClose}>
          Cancel
        </button>
        <button type="submit" className="primary" disabled={busy || !dir || !prompt.trim()}>
          {value.id ? "Save" : "Create"}
        </button>
      </div>
    </form>
  );
}

// a cron a preset would have written → that preset, with its time
function parseCron(cron: string): { preset: string; hh: number; mm: number } {
  const f = cron.trim().split(/\s+/);
  const mm = Number(f[0]);
  const hh = Number(f[1]);
  if (f.length === 5 && Number.isInteger(mm) && f[2] === "*" && f[3] === "*") {
    if (f[1] === "*") return { preset: "hourly", hh: 9, mm };
    if (Number.isInteger(hh) && f[4] === "*") return { preset: "daily", hh, mm };
    if (Number.isInteger(hh) && f[4] === "1-5") return { preset: "weekdays", hh, mm };
  }
  return { preset: "custom", hh: Number.isInteger(hh) ? hh : 9, mm: Number.isInteger(mm) ? mm : 0 };
}

function ago(d: Date): string {
  const m = Math.round((Date.now() - d.getTime()) / 60000);
  if (m < 1) return "just now";
  if (m < 60) return `${m} min ago`;
  const h = Math.round(m / 60);
  if (h < 48) return `${h} h ago`;
  return `${Math.round(h / 24)} days ago`;
}
