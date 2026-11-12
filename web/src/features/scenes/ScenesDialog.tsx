// Scenes: the morning set-ups and the evening tidies, edited as one list and saved whole. A saved scene runs from its
// row — the run dialog shows what it will do first — and from the palette and the sidebar's + menu.

import { useEffect, useState } from "react";
import { fetchRecipes } from "@/features/recipes/api";
import { connected } from "@/features/roster/options";
import { useRoster } from "@/features/roster/useRoster";
import { useSharing } from "@/features/roster/useSharing";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { Dialog, Icon, Notice, Tip } from "@/ui";
import { fetchScenes, saveScenes, type Scene } from "./api";
import SceneForm from "./SceneForm";
import { freshStep, newStartScene, newTidyScene, sceneWords, whyNotSave } from "./scenes";
import { t } from "@/lib/i18n";

export default function ScenesDialog({ onClose, onRun }: { onClose: () => void; onRun: (id: string) => void }) {
  const { machines, agents } = useRoster();
  const share = useSharing(); // under the screen-sharing mode the hidden machines and folders by their neutral names
  // the save replaces the list whole, so nothing is saved until the current one has loaded
  const loaded = useQuery(fetchScenes, []);
  const recipes = useQuery(fetchRecipes, []).data ?? [];
  const [list, setList] = useState<Scene[] | null>(null);
  const [open, setOpen] = useState<number | null>(null);
  const [dirty, setDirty] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [saved, setSaved] = useState(false);
  useEffect(() => {
    if (loaded.data && list === null) setList(loaded.data);
  }, [loaded.data, list]);

  const change = (next: Scene[]) => {
    setList(next);
    setDirty(true);
    setSaved(false);
  };
  const add = (scene: Scene) => {
    const rows = [...(list ?? []), scene];
    change(rows);
    setOpen(rows.length - 1);
  };
  const remove = (i: number) => {
    change((list ?? []).filter((_, j) => j !== i));
    setOpen(open === i ? null : open !== null && open > i ? open - 1 : open);
  };
  const blocked = (list ?? []).map(whyNotSave).find(Boolean) ?? "";
  // save keeps the list and answers it as saved, with its ids; null when it could not
  const save = async (): Promise<Scene[] | null> => {
    if (!list || busy || blocked) return null;
    setBusy(true);
    setErr("");
    try {
      const rows = await saveScenes(list);
      setList(rows);
      setDirty(false);
      setSaved(true);
      setOpen(null);
      return rows;
    } catch (e) {
      setErr(errMsg(e));
      return null;
    } finally {
      setBusy(false);
    }
  };
  // run opens the run dialog on a scene, saving the list first when it changed
  const run = async (i: number) => {
    const rows = dirty ? await save() : list;
    const id = rows?.[i]?.id;
    if (id) onRun(id);
  };

  return (
    <Dialog
      title={t("Scenes")}
      icon="sun"
      size="md"
      as="form"
      className="scenes-dialog"
      onClose={onClose}
      onSubmit={(e) => {
        e.preventDefault();
        void save();
      }}
    >
      <p className="modal-hint">{t("A scene is your morning set-up in one press — workspaces and tabs across your machines, each with its agent, model, first prompt and layout — or your evening tidy, each action shown before it runs. Shared by every device that uses this remuda.")}</p>
      {!list && !loaded.error && <Notice tone="hint">{t("Loading…")}</Notice>}
      <div className="scene-list">
        {(list ?? []).map((scene, i) =>
          open === i ? (
            <SceneForm key={i} scene={scene} recipes={recipes} onChange={(next) => change((list ?? []).map((s, j) => (j === i ? next : s)))} onDone={() => setOpen(null)} onRemove={() => remove(i)} />
          ) : (
            <div key={i} className="scene-row">
              <Icon name={scene.kind === "tidy" ? "moon" : "sun"} size={16} />
              <span className="scene-row-text">
                <strong>{scene.name || t("Unnamed")}</strong>
                <span className="scene-words">{share.redact(sceneWords(scene, agents, recipes, machines))}</span>
              </span>
              <Tip text={dirty ? t("Saves the list, then shows what it will do") : t("Shows what it will do, then runs it")}>
                <button type="button" className="btn sm" disabled={busy || (dirty && !!blocked)} onClick={() => void run(i)}>
                  <Icon name="play" size={12} /> {t("Run…")}
                </button>
              </Tip>
              <button type="button" className="btn sm" onClick={() => setOpen(i)}>
                {t("Edit")}
              </button>
              <Tip text={t("Remove")}>
                <button type="button" className="icon-btn danger" aria-label={t("Remove")} onClick={() => remove(i)}>
                  <Icon name="trash" size={14} />
                </button>
              </Tip>
            </div>
          ),
        )}
        {list && list.length === 0 && <Notice tone="hint">{t("No scenes yet: a morning that starts your agents, an evening that tidies after them.")}</Notice>}
      </div>
      <div className="scene-add">
        <button type="button" className="btn sm" disabled={!list} onClick={() => add({ ...newStartScene(), steps: [freshStep(connected(machines)[0]?.id ?? "local", agents)] })}>
          <Icon name="sun" size={12} /> {t("Add a set-up")}
        </button>
        <button type="button" className="btn sm" disabled={!list} onClick={() => add(newTidyScene())}>
          <Icon name="moon" size={12} /> {t("Add a tidy")}
        </button>
      </div>
      {blocked && dirty && <Notice tone="hint">{blocked}</Notice>}
      {saved && !dirty && <Notice tone="success">{t("Saved.")}</Notice>}
      {(err || loaded.error) && <Notice tone="error">{err || loaded.error}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          {t("Close")}
        </button>
        <button type="submit" className="btn primary" disabled={busy || !list || !dirty || !!blocked}>
          {busy ? t("Saving…") : t("Save")}
        </button>
      </div>
    </Dialog>
  );
}
