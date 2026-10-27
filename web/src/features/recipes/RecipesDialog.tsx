// Recipes: the saved ways to start an agent — a name, the agent, its model and arguments, a first prompt, and a
// folder when the recipe belongs to one — edited as one list and saved whole. The dialog opens on its own from the
// palette, or inside a "make something" dialog to manage the list or to save what its Options hold under a name.

import { useEffect, useState } from "react";
import type { Machine } from "@/features/roster/api";
import { connected, machineOptions } from "@/features/roster/options";
import { CUSTOM, EMPTY_DRAFT, hasModelFlag, modelChoices, modelPlaceholder, type Draft } from "@/features/roster/startOptions";
import { useRoster } from "@/features/roster/useRoster";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { Dialog, Icon, Logo, Notice, Select } from "@/ui";
import { fetchRecipes, saveRecipes, type Recipe } from "./api";
import RecipeMark from "./RecipeMark";
import { draftOf, recipeOf, recipeWords } from "./recipes";
import { t } from "@/lib/i18n";

// Form is the row being edited, as the fields hold it
type Form = { name: string; agent: string; draft: Draft; dir: string; machine: string };

const formOf = (r: Recipe): Form => ({ name: r.name, agent: r.agent, draft: draftOf(r), dir: r.dir ?? "", machine: r.machine ?? "" });

export default function RecipesDialog({ initial, onClose, onSaved }: { initial?: Recipe; onClose: () => void; onSaved?: (list: Recipe[]) => void }) {
  const { machines, agents } = useRoster();
  // the save replaces the list whole, so nothing is saved until the current one has loaded
  const loaded = useQuery(fetchRecipes, []);
  const [list, setList] = useState<Recipe[] | null>(null);
  const [open, setOpen] = useState<number | null>(null);
  const [form, setForm] = useState<Form | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  useEffect(() => {
    if (!loaded.data || list !== null) return;
    const rows = initial ? [...loaded.data, initial] : loaded.data;
    setList(rows);
    if (initial) edit(rows.length - 1, initial);
  }, [loaded.data, list, initial]);

  const edit = (i: number, r: Recipe) => {
    setOpen(i);
    setForm(formOf(r));
  };
  // applied is the list with the open row's form folded back in
  const applied = (): Recipe[] => {
    const rows = list ?? [];
    if (open === null || !form) return rows;
    const was = rows[open];
    const r = { ...recipeOf(form.name.trim(), form.agent, form.draft, { dir: form.dir, machine: form.machine || connected(machines)[0]?.id }), id: was?.id };
    return rows.map((x, j) => (j === open ? r : x));
  };
  const fold = () => {
    setList(applied());
    setOpen(null);
    setForm(null);
  };
  const remove = (i: number) => {
    setList((l) => (l ?? []).filter((_, j) => j !== i));
    if (open === i) {
      setOpen(null);
      setForm(null);
    } else if (open !== null && open > i) setOpen(open - 1);
  };
  const add = () => {
    const rows = applied();
    const fresh: Recipe = { name: "", agent: agents.some((a) => a.id === "claude") ? "claude" : (agents[0]?.id ?? "shell") };
    setList([...rows, fresh]);
    edit(rows.length, fresh);
  };
  const save = () => {
    if (!list || busy) return;
    setBusy(true);
    setErr("");
    saveRecipes(applied())
      .then((saved) => {
        onSaved?.(saved);
        onClose();
      }, (e) => setErr(errMsg(e)))
      .finally(() => setBusy(false));
  };

  return (
    <Dialog
      title={t("Recipes")}
      icon="sparkles"
      size="md"
      as="form"
      className="recipes-dialog"
      onClose={onClose}
      onSubmit={(e) => {
        e.preventDefault();
        save();
      }}
    >
      <p className="modal-hint">{t("A recipe is an agent with its model, extra arguments and first prompt, saved under a name: one pick fills the New workspace dialog, starts an agent from the palette, or sets up a scout schedule. Shared by every device that uses this remuda.")}</p>
      {!list && !loaded.error && <Notice tone="hint">{t("Loading…")}</Notice>}
      <div className="recipe-list">
        {(list ?? []).map((r, i) =>
          open === i && form ? (
            <RecipeForm key={i} form={form} agents={agents} machines={machines} onChange={setForm} onDone={fold} onRemove={() => remove(i)} />
          ) : (
            <div key={i} className="recipe-row">
              <RecipeMark agent={r.agent} />
              <span className="recipe-row-text">
                <strong>{r.name || "Unnamed"}</strong>
                <span className="recipe-words">{recipeWords(r, agents)}</span>
              </span>
              <button
                type="button"
                className="btn sm"
                onClick={() => {
                  const rows = applied();
                  setList(rows);
                  edit(i, rows[i] ?? r);
                }}
              >
                {t("Edit")}
              </button>
              <button type="button" className="icon-btn danger" title={t("Remove")} aria-label={t("Remove")} onClick={() => remove(i)}>
                <Icon name="trash" size={14} />
              </button>
            </div>
          ),
        )}
        {list && list.length === 0 && <Notice tone="hint">{t("No recipes yet.")}</Notice>}
      </div>
      <button type="button" className="btn sm" disabled={!list} onClick={add}>
        <Icon name="plus" size={12} /> Add a recipe
      </button>
      {(err || loaded.error) && <Notice tone="error">{err || loaded.error}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button type="submit" className="btn primary" disabled={busy || !list}>
          {t("Save")}
        </button>
      </div>
    </Dialog>
  );
}

// RecipeForm is one recipe's fields: the name, the agent, its model and arguments, the first prompt, an optional folder
function RecipeForm({ form, agents, machines, onChange, onDone, onRemove }: {
  form: Form;
  agents: { id: string; label: string }[];
  machines: Machine[];
  onChange: (f: Form) => void;
  onDone: () => void;
  onRemove: () => void;
}) {
  const set = (patch: Partial<Form>) => onChange({ ...form, ...patch });
  const setDraft = (patch: Partial<Draft>) => set({ draft: { ...form.draft, ...patch } });
  const shell = form.agent === "" || form.agent === "shell";
  const choices = modelChoices(form.agent);
  const typedModel = hasModelFlag(form.agent) && (choices.length === 0 || form.draft.choice === CUSTOM);
  const several = connected(machines).length > 1;
  return (
    <div className="recipe-form">
      <label>
        {t("Name")}
        <input autoFocus value={form.name} placeholder={t("Planner")} onChange={(e) => set({ name: e.target.value })} />
      </label>
      <label>
        {t("Agent")}
        <Select
          value={shell ? "shell" : form.agent}
          onChange={(agent) => set({ agent, draft: { ...EMPTY_DRAFT, args: form.draft.args, prompt: form.draft.prompt } })}
          options={[...agents.map((a) => ({ value: a.id, label: a.label, icon: <Logo agent={a.id} /> })), { value: "shell", label: t("Plain shell"), icon: <Icon name="terminal" size={14} /> }]}
        />
      </label>
      {!shell && hasModelFlag(form.agent) && choices.length > 0 && (
        <label>
          {t("Model")}
          <Select value={form.draft.choice} onChange={(choice) => setDraft({ choice })} options={choices} />
        </label>
      )}
      {!shell && typedModel && (
        <label>
          {choices.length > 0 ? t("Model id") : t("Model")}
          <input value={form.draft.custom} placeholder={modelPlaceholder(form.agent)} spellCheck={false} onChange={(e) => setDraft({ custom: e.target.value })} />
        </label>
      )}
      {!shell && (
        <label>
          {t("Extra arguments")}
          <input value={form.draft.args} placeholder={t("--permission-mode plan")} spellCheck={false} onChange={(e) => setDraft({ args: e.target.value })} />
        </label>
      )}
      {!shell && (
        <label>
          First prompt <small>{t("sent once the agent is up")}</small>
          <textarea value={form.draft.prompt} rows={3} placeholder={t("What the agent should start on")} onChange={(e) => setDraft({ prompt: e.target.value })} />
        </label>
      )}
      <label>
        Folder <small>{t("optional: the recipe belongs to this folder")}</small>
        <input value={form.dir} placeholder={t("any folder")} spellCheck={false} onChange={(e) => set({ dir: e.target.value })} />
      </label>
      {form.dir.trim() && several && (
        <label>
          {t("Machine")}
          <Select value={form.machine || connected(machines)[0]?.id || ""} onChange={(machine) => set({ machine })} options={machineOptions(machines)} />
        </label>
      )}
      <div className="recipe-form-actions">
        <button type="button" className="btn sm" onClick={onDone}>
          <Icon name="check" size={12} /> Done
        </button>
        <button type="button" className="btn sm ghost" onClick={onRemove}>
          <Icon name="trash" size={12} /> Remove
        </button>
      </div>
    </div>
  );
}
