// One scene's fields in the list: its name, and a set-up's steps — one open at a time, moved up and down, added from
// scratch or from the workspaces open now — or a tidy's choices.

import { useState } from "react";
import type { Recipe } from "@/features/recipes/api";
import { connected } from "@/features/roster/options";
import { useRoster } from "@/features/roster/useRoster";
import { Icon, Tip } from "@/ui";
import type { Scene, Step } from "./api";
import StepForm from "./StepForm";
import TidyForm from "./TidyForm";
import { freshStep, MAX_STEPS, moveStep, stepsOfWorkspaces, stepWhere, stepWords } from "./scenes";
import { t } from "@/lib/i18n";

export default function SceneForm({ scene, recipes, onChange, onDone, onRemove }: { scene: Scene; recipes: Recipe[]; onChange: (s: Scene) => void; onDone: () => void; onRemove: () => void }) {
  const { machines, agents } = useRoster();
  const steps = scene.steps ?? [];
  const [open, setOpen] = useState<number | null>(steps.length === 0 ? null : 0);
  const setSteps = (next: Step[]) => onChange({ ...scene, steps: next });
  const addStep = () => {
    setSteps([...steps, freshStep(connected(machines)[0]?.id ?? "local", agents)]);
    setOpen(steps.length);
  };
  const addOpen = () => {
    const known = new Set(steps.map((s) => `${s.machine}|${s.dir}`));
    const more = stepsOfWorkspaces(machines).filter((s) => !known.has(`${s.machine}|${s.dir}`));
    setSteps([...steps, ...more].slice(0, MAX_STEPS));
    setOpen(null);
  };
  const removeStep = (i: number) => {
    setSteps(steps.filter((_, j) => j !== i));
    setOpen(null);
  };
  const move = (i: number, by: -1 | 1) => {
    setSteps(moveStep(steps, i, by));
    if (open === i) setOpen(i + by);
  };
  return (
    <div className="scene-form">
      <label>
        {t("Name")}
        <input autoFocus value={scene.name} placeholder={scene.kind === "tidy" ? t("Evening") : t("Morning")} onChange={(e) => onChange({ ...scene, name: e.target.value })} />
      </label>
      {scene.kind === "tidy" ? (
        <TidyForm tidy={scene.tidy ?? { shells: true, idle: false, compact: false, worktrees: false }} machines={machines} onChange={(tidy) => onChange({ ...scene, tidy })} />
      ) : (
        <div className="scene-steps">
          <span className="scene-steps-head">{t("Steps, run one after another")}</span>
          {steps.map((step, i) =>
            open === i ? (
              <StepForm key={i} step={step} recipes={recipes} onChange={(next) => setSteps(steps.map((s, j) => (j === i ? next : s)))} onDone={() => setOpen(null)} />
            ) : (
              <div key={i} className="scene-step">
                <span className="scene-step-n">{i + 1}</span>
                <button type="button" className="linkish scene-step-text" onClick={() => setOpen(i)}>
                  <strong>{stepWhere(step, machines)}</strong>
                  <span className="scene-words">{stepWords(step, agents, recipes)}</span>
                </button>
                <Tip text={t("Earlier")}>
                  <button type="button" className="icon-btn sm" aria-label={t("Earlier")} disabled={i === 0} onClick={() => move(i, -1)}>
                    <Icon name="chevron-up" size={12} />
                  </button>
                </Tip>
                <Tip text={t("Later")}>
                  <button type="button" className="icon-btn sm" aria-label={t("Later")} disabled={i === steps.length - 1} onClick={() => move(i, 1)}>
                    <Icon name="chevron-down" size={12} />
                  </button>
                </Tip>
                <Tip text={t("Remove the step")}>
                  <button type="button" className="icon-btn sm danger" aria-label={t("Remove the step")} onClick={() => removeStep(i)}>
                    <Icon name="trash" size={12} />
                  </button>
                </Tip>
              </div>
            ),
          )}
          <div className="scene-add">
            <button type="button" className="btn sm" disabled={steps.length >= MAX_STEPS} onClick={addStep}>
              <Icon name="plus" size={12} /> {t("Add a step")}
            </button>
            <button type="button" className="btn sm" disabled={steps.length >= MAX_STEPS} onClick={addOpen}>
              <Icon name="layers" size={12} /> {t("Add the workspaces open now")}
            </button>
          </div>
        </div>
      )}
      <div className="recipe-form-actions">
        <button type="button" className="btn sm" onClick={onDone}>
          <Icon name="check" size={12} /> {t("Done")}
        </button>
        <button type="button" className="btn sm ghost" onClick={onRemove}>
          <Icon name="trash" size={12} /> {t("Remove the scene")}
        </button>
      </div>
    </div>
  );
}
