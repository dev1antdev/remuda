// The form that makes or edits a schedule: what runs — a prompt to the agent in a workspace folder, the standup as a
// digest to the person's devices, or a fresh agent started in a folder with a prompt — when (WhenFields: a preset that
// writes the cron, or cron by hand), and the kind's own fields.

import { useEffect, useMemo, useState } from "react";
import { denyDefault, whyNoDeny } from "@/features/agents/deny";
import { DenySwitch } from "@/features/agents/DenySwitch";
import { fetchRecipes, type Recipe } from "@/features/recipes/api";
import { draftOf, recipeWords } from "@/features/recipes/recipes";
import type { Machine } from "@/features/roster/api";
import { connected, folderOptions, machineOptions } from "@/features/roster/options";
import { choiceFor, CUSTOM, EMPTY_DRAFT, hasModelFlag, modelChoices, modelOf, modelPlaceholder } from "@/features/roster/startOptions";
import { useRoster } from "@/features/roster/useRoster";
import RetiredNotice from "@/features/usage/RetiredNotice";
import { useQuery } from "@/lib/query";
import { Icon, Logo, Notice, Select, useSubmit } from "@/ui";
import { agentSchedule, SCOUT_PROMPT } from "./agent";
import { putSchedule, type DigestSince, type Schedule, type ScheduleKind } from "./api";
import { cronFor, DEFAULT_CRON, parseCron, type When } from "./cron";
import { digestSchedule, kindOf, SINCE_OPTIONS } from "./digest";
import { WhenFields } from "./WhenFields";
import { t } from "@/lib/i18n";

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
  // an agent schedule saved before the switch keeps it off; a new one, or another kind made an agent one, starts on
  const [deny, setDeny] = useState(() => denyDefault(!!value.id && kindOf(value) === "agent", value.deny_prompts));
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
  const [when, setWhen] = useState(() => parseCron(value.cron ?? DEFAULT_CRON));
  const patchWhen = (patch: Partial<When>) => setWhen((was) => ({ ...was, ...patch }));
  const [cron, setCron] = useState(value.cron ?? DEFAULT_CRON);
  useEffect(() => {
    if (!dir && folders[0]) setDir(folders[0].value);
  }, [folders, dir]);
  useEffect(() => {
    const written = cronFor(when);
    if (written) setCron(written);
  }, [when]);
  const folder = dir === OTHER_FOLDER ? typedDir.trim() : dir;
  const { busy, err, run } = useSubmit(async () => {
    await putSchedule(
      kind === "digest"
        ? digestSchedule({ id: value.id, label, cron, since, enabled: value.enabled })
        : kind === "agent"
          ? agentSchedule({ id: value.id, machine, dir: folder, label, cron, prompt, agent, model: modelOf(agent, { ...EMPTY_DRAFT, ...model }), args, close, deny_prompts: deny, enabled: value.enabled })
          : { id: value.id, kind: "prompt", machine, dir: folder, label, cron, prompt, enabled: value.enabled ?? true },
    );
    onSaved();
  });
  const folderIcon = <Icon name="folder" size={14} />;
  const folderRows = [...(folders.some((f) => f.value === dir) || !dir || dir === OTHER_FOLDER ? folders : [{ value: dir, label: dir, icon: folderIcon }, ...folders]), { value: OTHER_FOLDER, label: t("Another folder…"), icon: folderIcon }];
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
      <nav className="schedule-kind" aria-label={t("What runs")}>
        <button type="button" className={kind === "prompt" ? "on" : ""} onClick={() => setKind("prompt")}>
          <Icon name="send" size={14} /> {t("Prompt to an agent")}
        </button>
        <button type="button" className={kind === "digest" ? "on" : ""} onClick={() => setKind("digest")}>
          <Icon name="sun" size={14} /> {t("Standup digest")}
        </button>
        <button type="button" className={kind === "agent" ? "on" : ""} onClick={() => setKind("agent")}>
          <Icon name="play" size={14} /> {t("Start an agent")}
        </button>
      </nav>
      {kind !== "digest" && (
        <div className="schedule-row">
          {several && (
            <label>
              {t("Machine")}
              <Select value={machine} onChange={setMachine} options={machineOptions(machines)} />
            </label>
          )}
          <label className="grow">
            {t("Folder")}
            <Select value={dir} onChange={setDir} placeholder={t("Pick a workspace")} options={folderRows} />
          </label>
          {dir === OTHER_FOLDER && (
            <label className="grow">
              {t("Folder path")}
              <input value={typedDir} onChange={(e) => setTypedDir(e.target.value)} placeholder="/home/me/project" spellCheck={false} />
            </label>
          )}
        </div>
      )}
      {kind === "agent" && (
        <div className="schedule-row">
          {agentRecipes.length > 0 && (
            <label>
              {t("Recipe")}
              <Select
                value={recipeId}
                placeholder={t("Fill from a recipe")}
                onChange={(id) => {
                  const r = agentRecipes.find((x) => x.id === id);
                  if (r) pickRecipe(r);
                }}
                options={agentRecipes.map((r) => ({ value: r.id ?? "", label: r.name, hint: recipeWords(r, agents), icon: <Logo agent={r.agent} /> }))}
              />
            </label>
          )}
          <label>
            {t("Agent")}
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
              {t("Model")}
              <Select value={model.choice} onChange={(choice) => setModel({ ...model, choice })} options={choices} />
            </label>
          )}
          {typedModel && (
            <label>
              {choices.length > 0 ? t("Model id") : t("Model")}
              <input value={model.custom} placeholder={modelPlaceholder(agent)} spellCheck={false} onChange={(e) => setModel({ ...model, custom: e.target.value })} />
            </label>
          )}
          <label className="grow">
            {t("Extra arguments")}
            <input value={args} placeholder={t("--permission-mode plan")} spellCheck={false} onChange={(e) => setArgs(e.target.value)} />
          </label>
        </div>
      )}
      {kind === "agent" && (
        <RetiredNotice agent={agent} model={modelOf(agent, { ...EMPTY_DRAFT, ...model })} args={args} machine={machine} onUse={(id) => setModel({ choice: CUSTOM, custom: id })} />
      )}
      <WhenFields
        when={when}
        onWhen={patchWhen}
        cron={cron}
        onCron={(typed) => {
          setCron(typed);
          patchWhen({ preset: "custom" });
        }}
      />
      {kind === "digest" ? (
        <div className="schedule-row">
          <label className="grow">
            <span>
              {t("Name")} <span className="dim">{t("(optional)")}</span>
            </span>
            <input value={label} onChange={(e) => setLabel(e.target.value)} placeholder={t("Standup digest")} />
          </label>
          <label>
            {t("Covers")}
            <Select value={since} onChange={setSince} options={SINCE_OPTIONS} />
          </label>
        </div>
      ) : (
        <label>
          <span>
            {t("Name")} <span className="dim">{t("(optional)")}</span>
          </span>
          <input value={label} onChange={(e) => setLabel(e.target.value)} placeholder={kind === "agent" ? t("Scout") : t("Morning status")} />
        </label>
      )}
      {kind === "digest" ? (
        <p className="schedule-note">
          One push to every device that gets remuda's alerts: how many agents finished, how many wait for you, what it cost, and a line per agent. A tap
          opens the Standup. Quiet hours hold it; a period with nothing in it sends nothing.
        </p>
      ) : (
        <label>
          {t("Prompt")}
          <textarea
            rows={4}
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder={kind === "agent" ? SCOUT_PROMPT : t("Run the tests and sum up what failed. If everything passes, say so in one line.")}
            required
          />
        </label>
      )}
      {kind === "agent" && (
        <>
          <label className="schedule-check">
            <input type="checkbox" checked={close} onChange={(e) => setClose(e.target.checked)} />
            {t("Close the tab when the agent has answered")}
          </label>
          <DenySwitch
            checked={deny}
            onChange={setDeny}
            why={whyNoDeny([{ id: agent, label: agents.find((a) => a.id === agent)?.label ?? agent }])}
            className="schedule-check"
            hintClass="schedule-note"
          />
          <p className="schedule-note">
            {t(
              "Each run opens a fresh tab in the folder's workspace (or a workspace on the folder) and hands the agent the prompt once it is up. While the agent the last run started still works or asks something, the next run is skipped. A question the agent asks is yours to answer: nothing closes while it waits. The conversation stays readable through the search after the tab is gone.",
            )}
          </p>
        </>
      )}
      {err && <Notice tone="error">{err}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button type="submit" className="btn primary" disabled={busy || !ready}>
          {value.id ? t("Save") : t("Create")}
        </button>
      </div>
    </form>
  );
}
