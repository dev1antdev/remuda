// The form that makes or edits a schedule: what runs — a prompt to the agent in a workspace folder, the standup as a
// digest to the person's devices, or a fresh agent started in a folder with a prompt — when (a preset that writes the
// cron, or cron by hand), and the kind's own fields.

import { useEffect, useMemo, useState } from "react";
import { fetchRecipes, type Recipe } from "@/features/recipes/api";
import { draftOf, recipeWords } from "@/features/recipes/recipes";
import type { Machine } from "@/features/roster/api";
import { connected, folderOptions, machineOptions } from "@/features/roster/options";
import { choiceFor, CUSTOM, EMPTY_DRAFT, hasModelFlag, modelChoices, modelOf, modelPlaceholder } from "@/features/roster/startOptions";
import { useRoster } from "@/features/roster/useRoster";
import { useQuery } from "@/lib/query";
import { Icon, Logo, Notice, Select, useSubmit } from "@/ui";
import { agentSchedule, SCOUT_PROMPT } from "./agent";
import { putSchedule, type DigestSince, type Schedule, type ScheduleKind } from "./api";
import { cronFor, DEFAULT_CRON, minuteOnly, parseCron, PRESETS } from "./cron";
import { digestSchedule, kindOf, SINCE_OPTIONS } from "./digest";

const OTHER_FOLDER = "\u0000other"; // the folder Select's last row: a path typed by hand

export function ScheduleEditor({ machines, value, onClose, onSaved }: { machines: Machine[]; value: Partial<Schedule>; onClose: () => void; onSaved: () => void }) {
  const { agents } = useRoster();
  const several = connected(machines).length > 1;
  const [kind, setKind] = useState<ScheduleKind>(kindOf(value));
  const [machine, setMachine] = useState(value.machine ?? connected(machines)[0]?.id ?? "");
  const folders = useMemo(() => folderOptions(machines, machine), [machines, machine]);
  const [dir, setDir] = useState(value.dir ?? "");
  const [typedDir, setTypedDir] = useState(value.dir && !folders.some((f) => f.value === value.dir) ? value.dir : "");
  const [label, setLabel] = useState(value.label ?? "");
  const [prompt, setPrompt] = useState(value.prompt ?? "");
  const [since, setSince] = useState<DigestSince>(value.since ?? "midnight");
  const [agent, setAgent] = useState(value.agent ?? (agents.some((a) => a.id === "claude") ? "claude" : (agents[0]?.id ?? "")));
  const [model, setModel] = useState(() => choiceFor(agent, value.model ?? ""));
  const [args, setArgs] = useState(value.args ?? "");
  const [close, setClose] = useState(value.close ?? true);
  const recipes = useQuery(fetchRecipes, []);
  const [recipeId, setRecipeId] = useState("");
  // a recipe fills the agent's fields, which stay the person's to change
  const pickRecipe = (r: Recipe) => {
    setRecipeId(r.id ?? "");
    const filled = draftOf(r);
    setAgent(r.agent);
    setModel({ choice: filled.choice, custom: filled.custom });
    setArgs(filled.args);
    setPrompt(filled.prompt);
  };
  const agentRecipes = (recipes.data ?? []).filter((r) => r.agent !== "" && r.agent !== "shell");
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
  const folder = dir === OTHER_FOLDER ? typedDir.trim() : dir;
  const { busy, err, run } = useSubmit(async () => {
    await putSchedule(
      kind === "digest"
        ? digestSchedule({ id: value.id, label, cron, since, enabled: value.enabled })
        : kind === "agent"
          ? agentSchedule({ id: value.id, machine, dir: folder, label, cron, prompt, agent, model: modelOf(agent, { ...EMPTY_DRAFT, ...model }), args, close, enabled: value.enabled })
          : { id: value.id, kind: "prompt", machine, dir: folder, label, cron, prompt, enabled: value.enabled ?? true },
    );
    onSaved();
  });
  const folderIcon = <Icon name="folder" size={14} />;
  const folderRows = [...(folders.some((f) => f.value === dir) || !dir || dir === OTHER_FOLDER ? folders : [{ value: dir, label: dir, icon: folderIcon }, ...folders]), { value: OTHER_FOLDER, label: "Another folder…", icon: folderIcon }];
  const choices = modelChoices(agent);
  const typedModel = hasModelFlag(agent) && (choices.length === 0 || model.choice === CUSTOM);
  const ready = kind === "digest" ? cron.trim() !== "" : !!folder && prompt.trim() !== "" && (kind !== "agent" || !!agent);

  return (
    <form
      className="schedule-editor"
      onSubmit={(e) => {
        e.preventDefault();
        void run();
      }}
    >
      <nav className="schedule-kind" aria-label="What runs">
        <button type="button" className={kind === "prompt" ? "on" : ""} onClick={() => setKind("prompt")}>
          <Icon name="send" size={14} /> Prompt to an agent
        </button>
        <button type="button" className={kind === "digest" ? "on" : ""} onClick={() => setKind("digest")}>
          <Icon name="sun" size={14} /> Standup digest
        </button>
        <button type="button" className={kind === "agent" ? "on" : ""} onClick={() => setKind("agent")}>
          <Icon name="play" size={14} /> Start an agent
        </button>
      </nav>
      {kind !== "digest" && (
        <div className="schedule-row">
          {several && (
            <label>
              Machine
              <Select value={machine} onChange={setMachine} options={machineOptions(machines)} />
            </label>
          )}
          <label className="grow">
            Folder
            <Select value={dir} onChange={setDir} placeholder="Pick a workspace" options={folderRows} />
          </label>
          {dir === OTHER_FOLDER && (
            <label className="grow">
              Folder path
              <input value={typedDir} onChange={(e) => setTypedDir(e.target.value)} placeholder="/home/me/project" spellCheck={false} />
            </label>
          )}
        </div>
      )}
      {kind === "agent" && (
        <div className="schedule-row">
          {agentRecipes.length > 0 && (
            <label>
              Recipe
              <Select
                value={recipeId}
                placeholder="Fill from a recipe"
                onChange={(id) => {
                  const r = agentRecipes.find((x) => x.id === id);
                  if (r) pickRecipe(r);
                }}
                options={agentRecipes.map((r) => ({ value: r.id ?? "", label: r.name, hint: recipeWords(r, agents), icon: <Logo agent={r.agent} /> }))}
              />
            </label>
          )}
          <label>
            Agent
            <Select
              value={agent}
              onChange={(next) => {
                setAgent(next);
                setModel(choiceFor(next, ""));
              }}
              options={agents.map((a) => ({ value: a.id, label: a.label, icon: <Logo agent={a.id} /> }))}
            />
          </label>
          {hasModelFlag(agent) && choices.length > 0 && (
            <label>
              Model
              <Select value={model.choice} onChange={(choice) => setModel({ ...model, choice })} options={choices} />
            </label>
          )}
          {typedModel && (
            <label>
              {choices.length > 0 ? "Model id" : "Model"}
              <input value={model.custom} placeholder={modelPlaceholder(agent)} spellCheck={false} onChange={(e) => setModel({ ...model, custom: e.target.value })} />
            </label>
          )}
          <label className="grow">
            Extra arguments
            <input value={args} placeholder="--permission-mode plan" spellCheck={false} onChange={(e) => setArgs(e.target.value)} />
          </label>
        </div>
      )}
      <div className="schedule-row">
        <label>
          When
          <Select value={preset} onChange={setPreset} options={PRESETS.map((p) => ({ value: p.id, label: p.label }))} />
        </label>
        {!minuteOnly(preset) && preset !== "custom" && (
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
        {minuteOnly(preset) && (
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
      {kind === "digest" ? (
        <div className="schedule-row">
          <label className="grow">
            <span>
              Name <span className="dim">(optional)</span>
            </span>
            <input value={label} onChange={(e) => setLabel(e.target.value)} placeholder="Standup digest" />
          </label>
          <label>
            Covers
            <Select value={since} onChange={setSince} options={SINCE_OPTIONS} />
          </label>
        </div>
      ) : (
        <label>
          <span>
            Name <span className="dim">(optional)</span>
          </span>
          <input value={label} onChange={(e) => setLabel(e.target.value)} placeholder={kind === "agent" ? "Scout" : "Morning status"} />
        </label>
      )}
      {kind === "digest" ? (
        <p className="schedule-note">
          One push to every device that gets remuda's alerts: how many agents finished, how many wait for you, what it cost, and a line per agent. A tap
          opens the Standup. Quiet hours hold it; a period with nothing in it sends nothing.
        </p>
      ) : (
        <label>
          Prompt
          <textarea
            rows={4}
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder={kind === "agent" ? SCOUT_PROMPT : "Run the tests and sum up what failed. If everything passes, say so in one line."}
            required
          />
        </label>
      )}
      {kind === "agent" && (
        <>
          <label className="schedule-check">
            <input type="checkbox" checked={close} onChange={(e) => setClose(e.target.checked)} />
            Close the tab when the agent has answered
          </label>
          <p className="schedule-note">
            Each run opens a fresh tab in the folder's workspace (or a workspace on the folder) and hands the agent the prompt once it is up. A question
            the agent asks is yours to answer: nothing closes while it waits. The conversation stays readable through the search after the tab is gone.
          </p>
        </>
      )}
      {err && <Notice tone="error">{err}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          Cancel
        </button>
        <button type="submit" className="btn primary" disabled={busy || !ready}>
          {value.id ? "Save" : "Create"}
        </button>
      </div>
    </form>
  );
}
