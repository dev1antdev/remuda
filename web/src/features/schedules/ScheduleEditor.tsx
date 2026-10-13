// The form that makes or edits a schedule: a machine, a workspace folder, when (a preset that writes the cron, or cron by hand), the prompt.

import { useEffect, useMemo, useState } from "react";
import type { Machine } from "@/features/roster/api";
import { connected, folderOptions, machineOptions } from "@/features/roster/options";
import { Icon, Notice, Select, useSubmit } from "@/ui";
import { putSchedule, type Schedule } from "./api";
import { cronFor, DEFAULT_CRON, parseCron, PRESETS } from "./cron";

export function ScheduleEditor({ machines, value, onClose, onSaved }: { machines: Machine[]; value: Partial<Schedule>; onClose: () => void; onSaved: () => void }) {
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
    const written = cronFor(preset, hh, mm);
    if (written) setCron(written);
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
