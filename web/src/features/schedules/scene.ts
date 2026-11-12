// The pure part of a scene schedule — one of the scenes run on a timetable: a fresh record, and the words its row says.

import type { Scene } from "@/features/scenes/api";
import type { Schedule } from "./api";
import { t } from "@/lib/i18n";

// sceneSchedule is a scene schedule as the form hands it over: the scene and the timetable, nothing of its own to run
export const sceneSchedule = (s: Partial<Schedule> & { scene: string }): Schedule => ({
  id: s.id,
  kind: "scene",
  machine: "",
  dir: "",
  label: s.label ?? "",
  cron: s.cron ?? "",
  prompt: "",
  scene: s.scene,
  enabled: s.enabled ?? true,
});

// sceneScheduleWords is a scene schedule's row: which scene it runs and what that does on a timetable
export function sceneScheduleWords(s: Pick<Schedule, "scene">, scenes: Scene[]): string {
  const scene = scenes.find((x) => x.id === s.scene);
  if (!scene) return t("a scene that is gone");
  return scene.kind === "tidy" ? t("tidies with “{name}”, only while no agent works or asks", { name: scene.name }) : t("starts the scene “{name}”", { name: scene.name });
}
