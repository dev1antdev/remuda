// New agent from recipe: a saved recipe started in a folder — the recipe's own folder when it has one, else the folder
// it was started in last, else the one at hand — on a machine the roster knows; the recipe last used in a folder is
// remembered on the device, so the dialog starts on it.

import { useEffect, useMemo, useState } from "react";
import { connected, folderOptions, machineOptions } from "@/features/roster/options";
import { useRoster } from "@/features/roster/useRoster";
import { readPref, writePref } from "@/lib/prefs";
import { useQuery } from "@/lib/query";
import { FormDialog, Icon, Notice, Select, useSubmit } from "@/ui";
import { fetchRecipes, type Recipe } from "./api";
import RecipeMark from "./RecipeMark";
import { folderFor, lastRecipeKey, machineFor, pickFirst, recipeDirKey, recipeWords, startRequest, type StartRequest } from "./recipes";
import RecipesDialog from "./RecipesDialog";
import { t } from "@/lib/i18n";

const OTHER_FOLDER = "\u0000other"; // the folder Select's last row: a path typed by hand

export default function StartFromRecipeDialog({ machineId, dir, onClose, onStart }: { machineId?: string; dir?: string; onClose: () => void; onStart: (req: StartRequest) => Promise<void> }) {
  const { machines, agents } = useRoster();
  const recipes = useQuery(fetchRecipes, []);
  const list = recipes.data ?? [];
  const atHandMachine = machineId ?? connected(machines)[0]?.id ?? "local";
  const atHand = dir ?? "";
  const [recipeId, setRecipeId] = useState("");
  const [machine, setMachine] = useState(atHandMachine);
  const [folder, setFolder] = useState(atHand);
  const [typed, setTyped] = useState("");
  const [managing, setManaging] = useState(false);
  const recipe = list.find((r) => r.id === recipeId) ?? null;
  // the recipe picked places the dialog: its machine and folder, or what it was started in last
  const pick = (r: Recipe | null) => {
    setRecipeId(r?.id ?? "");
    if (!r) return;
    setMachine(machineFor(r, atHandMachine));
    setFolder(folderFor(r, readPref(recipeDirKey(r.id ?? ""), ""), atHand));
  };
  useEffect(() => {
    if (recipes.data && !recipeId) pick(pickFirst(recipes.data, readPref(lastRecipeKey(atHandMachine, atHand), "")));
  }, [recipes.data, recipeId]);
  const folders = useMemo(() => folderOptions(machines, machine), [machines, machine]);
  const folderIcon = <Icon name="folder" size={14} />;
  const listed = folders.some((f) => f.value === folder);
  const folderRows = [...(listed || !folder || folder === OTHER_FOLDER ? folders : [{ value: folder, label: folder, icon: folderIcon }, ...folders]), { value: OTHER_FOLDER, label: t("Another folder…"), icon: folderIcon }];
  const where = folder === OTHER_FOLDER ? typed.trim() : folder;
  const submit = useSubmit(async () => {
    if (!recipe) throw new Error("Pick a recipe");
    if (!where) throw new Error("A folder is required");
    await onStart(startRequest(recipe, machine, where));
    writePref(recipeDirKey(recipe.id ?? ""), where);
    writePref(lastRecipeKey(machine, where), recipe.id ?? "");
  });
  const several = connected(machines).length > 1;
  return (
    <FormDialog title={t("New agent from recipe")} icon="sparkles" onClose={onClose} submit={submit} submitLabel="Start">
      {recipes.error && <Notice tone="error">{recipes.error}</Notice>}
      {recipes.data && list.length === 0 && (
        <Notice
          tone="hint"
          action={
            <button type="button" className="btn sm" onClick={() => setManaging(true)}>
              {t("Add a recipe…")}
            </button>
          }
        >
          {t("No recipes yet: a recipe is an agent with its model, arguments and first prompt, saved under a name.")}
        </Notice>
      )}
      {list.length > 0 && (
        <label>
          {t("Recipe")}
          <Select value={recipeId} onChange={(id) => pick(list.find((r) => r.id === id) ?? null)} options={list.map((r) => ({ value: r.id ?? "", label: r.name, hint: recipeWords(r, agents), icon: <RecipeMark agent={r.agent} /> }))} />
        </label>
      )}
      {recipe && <p className="recipe-words start-recipe-words">{recipeWords(recipe, agents)}</p>}
      {several && (
        <label>
          {t("Machine")}
          <Select value={machine} onChange={setMachine} options={machineOptions(machines)} />
        </label>
      )}
      <label>
        {t("Folder")}
        <Select value={folder} onChange={setFolder} placeholder={t("Pick a workspace")} options={folderRows} />
      </label>
      {folder === OTHER_FOLDER && (
        <label>
          {t("Folder path")}
          <input autoFocus value={typed} onChange={(e) => setTyped(e.target.value)} placeholder="/home/me/project" spellCheck={false} />
        </label>
      )}
      <button type="button" className="linkish recipe-manage" onClick={() => setManaging(true)}>
        <Icon name="sliders" size={12} /> Manage recipes…
      </button>
      {managing && (
        <RecipesDialog
          onClose={() => setManaging(false)}
          onSaved={() => {
            setRecipeId("");
            recipes.reload();
          }}
        />
      )}
    </FormDialog>
  );
}
