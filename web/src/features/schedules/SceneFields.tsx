// A scene schedule's own fields: the scene it runs, its name, and what a run does on a timetable.

import { useScenes } from "@/features/scenes/api";
import { Icon, Notice, Select } from "@/ui";
import { t } from "@/lib/i18n";

export function SceneFields({ scene, onScene, label, onLabel }: { scene: string; onScene: (id: string) => void; label: string; onLabel: (label: string) => void }) {
  const scenes = useScenes(true).filter((s) => s.id);
  const picked = scenes.find((s) => s.id === scene);
  return (
    <>
      {scenes.length === 0 ? (
        <Notice tone="hint">{t("No scenes yet: make one in Scenes… first, from the palette or the sidebar's + menu.")}</Notice>
      ) : (
        <div className="schedule-row">
          <label className="grow">
            {t("Scene")}
            <Select
              value={scene}
              placeholder={t("Pick a scene")}
              onChange={onScene}
              options={scenes.map((s) => ({ value: s.id ?? "", label: s.name, icon: <Icon name={s.kind === "tidy" ? "moon" : "sun"} size={14} /> }))}
            />
          </label>
          <label className="grow">
            <span>
              {t("Name")} <span className="dim">{t("(optional)")}</span>
            </span>
            <input value={label} onChange={(e) => onLabel(e.target.value)} placeholder={picked?.name ?? t("Morning")} />
          </label>
        </div>
      )}
      <p className="schedule-note">
        {picked?.kind === "tidy"
          ? t("At each run the tidy does everything its plan holds then — only while no agent on the machines it covers works or asks you something; otherwise the run is skipped. The run's outcome is on this row.")
          : t("At each run every step of the scene starts as you saved it: its workspaces and tabs, its agents and their first prompts. A machine away is skipped and said.")}
      </p>
    </>
  );
}
