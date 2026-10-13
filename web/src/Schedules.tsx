// Schedules: prompts sent to an agent on a timetable. A list with the next and
// last run, a switch per row, "Run now", and a form to make one — a machine, a
// workspace folder, when (presets that write cron, or cron by hand), the prompt.

import { useEffect, useMemo, useState } from "react";
import { deleteSchedule, fetchSchedules, putSchedule, runSchedule, type Machine, type Schedule } from "./api";
import { basename, errMsg } from "./prefs";
import { connected, folderOptions, machineLabel, machineOptions } from "./options";
import { ago, whenShort } from "./time";
import { Dialog, Notice, useSubmit } from "./ui";
import Select from "./Select";
import Icon from "./icons";

const NEVER = "0001-01-01T00:00:00Z"; // Go's zero time
const DEFAULT_CRON = "0 9 * * 1-5";

// PRESETS write their cron from the hour and minute; "custom" leaves the cron as typed
const PRESETS: { id: string; label: string; cron?: (hh: number, mm: number) => string }[] = [
  { id: "daily", label: "Every day", cron: (h, m) => `${m} ${h} * * *` },
  { id: "weekdays", label: "Weekdays", cron: (h, m) => `${m} ${h} * * 1-5` },
  { id: "hourly", label: "Every hour", cron: (_h, m) => `${m} * * * *` },
  { id: "custom", label: "Cron…" },
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
    let gone = false;
    fetchSchedules().then(
      (list) => !gone && setItems(list),
      (e) => !gone && setError(errMsg(e)),
    );
    return () => {
      gone = true;
    };
  }, [tick]);
  const refresh = () => setTick((t) => t + 1);
  const fail = (e: unknown) => setError(errMsg(e));

  // the switch flips at once; a failed save flips it back
  const toggle = (s: Schedule, enabled: boolean) => {
    setItems((list) => list?.map((x) => (x.id === s.id ? { ...x, enabled } : x)) ?? list);
    putSchedule({ ...s, enabled }).then(refresh, (e) => {
      fail(e);
      setItems((list) => list?.map((x) => (x.id === s.id ? { ...x, enabled: s.enabled } : x)) ?? list);
    });
  };

  return (
    <Dialog
      title="Schedules"
      icon="calendar"
      size="md"
      className="schedules"
      onClose={onClose}
      controls={
        <button type="button" className="btn sm" onClick={() => setEditing({ enabled: true })}>
          <Icon name="plus" size={12} /> New
        </button>
      }
    >
      <Notice tone="hint">
        A prompt sent to the agent running in a folder, on a timetable — in the remuda host's local time. A minute with no agent there, or an agent busy
        with a question, is skipped and noted.
      </Notice>
      {error && (
        <Notice tone="error" onDismiss={() => setError("")}>
          {error}
        </Notice>
      )}
      {items && items.length === 0 && !editing && <div className="empty">No schedules yet.</div>}
      <div className="schedules-list">
        {items?.map((s) => (
          <div key={s.id} className={`schedule${s.enabled ? "" : " off"}`}>
            <input type="checkbox" checked={s.enabled} title={s.enabled ? "On" : "Off"} aria-label={s.enabled ? "On" : "Off"} onChange={(e) => toggle(s, e.target.checked)} />
            <div className="schedule-copy">
              <div className="schedule-label">{s.label}</div>
              <div className="schedule-sub">
                <code>{s.cron}</code> · {machineLabel(machines, s.machine)} · {basename(s.dir)}
                {s.enabled && s.next && s.next !== NEVER && <span> · next {whenShort(s.next)}</span>}
                {s.last_run && s.last_run !== NEVER && (
                  <>
                    {" · "}
                    <span className={s.last_ok ? "ok" : "bad"}>
                      last {ago(s.last_run)}: {s.last_result}
                    </span>
                  </>
                )}
              </div>
            </div>
            <button type="button" className="icon-btn" title="Run now" onClick={() => void runSchedule(s.id ?? "").then(refresh, fail)}>
              <Icon name="zap" size={14} />
            </button>
            <button type="button" className="icon-btn" title="Edit" onClick={() => setEditing(s)}>
              <Icon name="sliders" size={14} />
            </button>
            <button
              type="button"
              className="icon-btn danger"
              title="Delete"
              onClick={() => ask(`Delete the schedule “${s.label}”?`, "Delete", () => void deleteSchedule(s.id ?? "").then(refresh, fail))}
            >
              <Icon name="trash" size={14} />
            </button>
          </div>
        ))}
      </div>
      {editing && (
        <Editor
          key={editing.id ?? "new"} // a different schedule is a fresh form
          machines={machines}
          value={editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            refresh();
          }}
        />
      )}
    </Dialog>
  );
}

function Editor({ machines, value, onClose, onSaved }: { machines: Machine[]; value: Partial<Schedule>; onClose: () => void; onSaved: () => void }) {
  const several = connected(machines).length > 1;
  const [machine, setMachine] = useState(value.machine ?? connected(machines)[0]?.id ?? "");
  const folders = useMemo(() => folderOptions(machines, machine), [machines, machine]);
  const [dir, setDir] = useState(value.dir ?? "");
  const [label, setLabel] = useState(value.label ?? "");
  const [prompt, setPrompt] = useState(value.prompt ?? "");
  const [initial] = useState(() => parseCron(value.cron ?? DEFAULT_CRON));
  const [preset, setPreset] = useState(initial.preset);
  const [hh, setHH] = useState(initial.hh);
  const [mm, setMM] = useState(initial.mm);
  const [cron, setCron] = useState(value.cron ?? DEFAULT_CRON);
  useEffect(() => {
    if (!dir && folders[0]) setDir(folders[0].value);
  }, [folders, dir]);
  useEffect(() => {
    const write = PRESETS.find((p) => p.id === preset)?.cron;
    if (write) setCron(write(hh, mm));
  }, [preset, hh, mm]);
  const pad = (n: number) => String(n).padStart(2, "0");
  const { busy, err, run } = useSubmit(async () => {
    await putSchedule({ id: value.id, machine, dir, label, cron, prompt, enabled: value.enabled ?? true });
    onSaved();
  });
  const folderIcon = <Icon name="folder" size={14} />;

  return (
    <form
      className="schedule-editor"
      onSubmit={(e) => {
        e.preventDefault();
        void run();
      }}
    >
      <div className="schedule-row">
        {several && (
          <label>
            Machine
            <Select value={machine} onChange={setMachine} options={machineOptions(machines)} />
          </label>
        )}
        <label className="grow">
          Folder
          <Select value={dir} onChange={setDir} placeholder="Pick a workspace" options={folders.some((f) => f.value === dir) || !dir ? folders : [{ value: dir, label: dir, icon: folderIcon }, ...folders]} />
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
            <input
              type="time"
              value={`${pad(hh)}:${pad(mm)}`}
              onChange={(e) => {
                const [h, m] = e.target.value.split(":").map(Number);
                if (h !== undefined && !Number.isNaN(h)) {
                  setHH(h);
                  setMM(m && !Number.isNaN(m) ? m : 0);
                }
              }}
            />
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
          <input
            value={cron}
            onChange={(e) => {
              setCron(e.target.value);
              setPreset("custom");
            }}
            placeholder="minute hour day month weekday"
            spellCheck={false}
          />
        </label>
      </div>
      <label>
        <span>
          Name <span className="dim">(optional)</span>
        </span>
        <input value={label} onChange={(e) => setLabel(e.target.value)} placeholder="Morning status" />
      </label>
      <label>
        Prompt
        <textarea rows={4} value={prompt} onChange={(e) => setPrompt(e.target.value)} placeholder="Run the tests and sum up what failed. If everything passes, say so in one line." required />
      </label>
      {err && <Notice tone="error">{err}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          Cancel
        </button>
        <button type="submit" className="btn primary" disabled={busy || !dir || !prompt.trim()}>
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
