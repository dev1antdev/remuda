// One step of a set-up: the machine and folder it opens on, what runs there — an agent with its model, arguments and
// first prompt, a recipe, or a plain shell — its tab's name, and the layout of a workspace's tab captured from herdr.

import { useMemo, useState } from "react";
import type { Recipe } from "@/features/recipes/api";
import { draftOf, recipeWords } from "@/features/recipes/recipes";
import { folderOptions, machineOptions } from "@/features/roster/options";
import { CUSTOM, hasModelFlag, modelChoices, modelOf, modelPlaceholder, type Draft } from "@/features/roster/startOptions";
import { useRoster } from "@/features/roster/useRoster";
import { useSharing } from "@/features/roster/useSharing";
import RetiredNotice from "@/features/usage/RetiredNotice";
import { errMsg } from "@/lib/text";
import { Icon, Logo, Notice, Select } from "@/ui";
import { captureLayout, type Step } from "./api";
import { layoutWords, SHELL, sourceOf, withSource } from "./scenes";
import { t } from "@/lib/i18n";

const OTHER_FOLDER = "\u0000other"; // the folder Select's last row: a path typed by hand

export default function StepForm({ step, recipes, onChange, onDone }: { step: Step; recipes: Recipe[]; onChange: (s: Step) => void; onDone: () => void }) {
  const { machines, agents } = useRoster();
  const share = useSharing(); // under the screen-sharing mode the hidden machines and folders by their neutral names
  const kind = step.recipe || !step.agent ? "" : step.agent;
  const [draft, setDraft] = useState<Draft>(() => draftOf({ name: "", agent: kind, model: step.model, args: step.args, prompt: step.prompt }));
  const [typing, setTyping] = useState(false);
  const [capturing, setCapturing] = useState(false);
  const [err, setErr] = useState("");
  const set = (patch: Partial<Step>) => onChange({ ...step, ...patch });
  const setOptions = (patch: Partial<Draft>) => {
    const next = { ...draft, ...patch };
    setDraft(next);
    set({ model: modelOf(kind, next) || undefined, args: next.args.trim() || undefined, prompt: next.prompt.trim() || undefined });
  };
  const sources = [
    ...agents.map((a) => ({ value: `agent:${a.id}`, label: a.label, icon: <Logo agent={a.id} /> })),
    ...recipes.map((r) => ({ value: `recipe:${r.id}`, label: t("Recipe: {name}", { name: r.name }), hint: recipeWords(r, agents), icon: <Icon name="sparkles" size={14} /> })),
    { value: SHELL, label: t("Plain shell"), icon: <Icon name="terminal" size={14} /> },
  ];
  const folders = useMemo(() => folderOptions(machines, step.machine).map((f) => ({ ...f, label: share.redact(f.label), hint: f.hint && share.redact(f.hint) })), [machines, step.machine, share]);
  const listed = folders.some((f) => f.value === step.dir);
  const folderRows = [...(listed || !step.dir ? folders : [{ value: step.dir, label: share.redact(step.dir), icon: <Icon name="folder" size={14} /> }, ...folders]), { value: OTHER_FOLDER, label: t("Another folder…"), icon: <Icon name="folder" size={14} /> }];
  // the workspace open on the step's folder, whose active tab a capture reads
  const open = machines.find((m) => m.id === step.machine)?.sessions.find((s) => s.dir === step.dir && !s.scratch);
  const capture = () => {
    if (!open) return;
    setCapturing(true);
    setErr("");
    captureLayout(step.machine, open.id)
      .then((got) => set({ layout: got.layout }), (e) => setErr(errMsg(e)))
      .finally(() => setCapturing(false));
  };
  const choices = modelChoices(kind);
  const typedModel = hasModelFlag(kind) && (choices.length === 0 || draft.choice === CUSTOM);
  return (
    <div className="scene-step-form">
      <div className="scene-step-where">
        <label>
          {t("Machine")}
          <Select value={step.machine} onChange={(machine) => set({ machine, layout: undefined })} options={machineOptions(machines, { onlyConnected: false, label: share.machineLabel })} />
        </label>
        <label>
          {t("Folder")}
          <Select
            value={typing ? OTHER_FOLDER : step.dir}
            placeholder={t("Pick a workspace")}
            onChange={(dir) => {
              setTyping(dir === OTHER_FOLDER);
              if (dir !== OTHER_FOLDER) set({ dir });
            }}
            options={folderRows}
          />
        </label>
      </div>
      {typing && (
        <label>
          {t("Folder path")}
          <input autoFocus value={step.dir} onChange={(e) => set({ dir: e.target.value })} placeholder="/home/me/project" spellCheck={false} />
        </label>
      )}
      <label>
        {t("Runs")}
        <Select
          value={sourceOf(step)}
          onChange={(value) => {
            const next = withSource(step, value);
            setDraft(draftOf({ name: "", agent: next.agent ?? "", prompt: next.prompt }));
            onChange(next);
          }}
          options={sources}
        />
      </label>
      {kind && kind !== SHELL && hasModelFlag(kind) && choices.length > 0 && (
        <label>
          {t("Model")}
          <Select value={draft.choice} onChange={(choice) => setOptions({ choice })} options={choices} />
        </label>
      )}
      {kind && kind !== SHELL && typedModel && (
        <label>
          {choices.length > 0 ? t("Model id") : t("Model")}
          <input value={draft.custom} placeholder={modelPlaceholder(kind)} spellCheck={false} onChange={(e) => setOptions({ custom: e.target.value })} />
        </label>
      )}
      {kind && kind !== SHELL && (
        <>
          <label>
            {t("Extra arguments")}
            <input value={draft.args} placeholder={t("--permission-mode plan")} spellCheck={false} onChange={(e) => setOptions({ args: e.target.value })} />
          </label>
          <RetiredNotice agent={kind} model={modelOf(kind, draft)} args={draft.args} machine={step.machine} onUse={(model) => setOptions({ choice: CUSTOM, custom: model })} />
          <label>
            <span>
              {t("First prompt")} <small>{t("sent once the agent is up")}</small>
            </span>
            <textarea value={draft.prompt} rows={3} placeholder={t("What the agent should start on")} onChange={(e) => setOptions({ prompt: e.target.value })} />
          </label>
        </>
      )}
      <label>
        <span>
          {t("Tab name")} <small>{t("optional")}</small>
        </span>
        <input value={step.tab ?? ""} maxLength={40} placeholder={t("herdr's own")} onChange={(e) => set({ tab: e.target.value || undefined })} />
      </label>
      <div className="scene-layout">
        <Icon name="columns" size={14} />
        <span className="scene-words">{step.layout ? layoutWords(step.layout, sourceOf(step) !== SHELL) : t("One pane, as herdr opens a tab")}</span>
        {step.layout ? (
          <button type="button" className="linkish" onClick={() => set({ layout: undefined })}>
            {t("Remove the layout")}
          </button>
        ) : (
          <button type="button" className="btn sm" disabled={!open || capturing} onClick={capture}>
            {capturing ? t("Capturing…") : t("Capture this workspace's layout")}
          </button>
        )}
      </div>
      {!step.layout && !open && step.dir && <p className="modal-hint">{t("Open a workspace on this folder and lay its tab out in herdr, then capture it here.")}</p>}
      {err && <Notice tone="error">{err}</Notice>}
      <div className="recipe-form-actions">
        <button type="button" className="btn sm" onClick={onDone}>
          <Icon name="check" size={12} /> {t("Done")}
        </button>
      </div>
    </div>
  );
}
