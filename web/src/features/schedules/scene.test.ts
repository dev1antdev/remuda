import { describe, expect, it } from "vitest";
import { kindOf } from "./digest";
import { resultWords } from "./runs";
import { sceneSchedule, sceneScheduleWords } from "./scene";

describe("scene schedules", () => {
  const scenes = [
    { id: "s1", name: "Morning", kind: "start" as const, steps: [] },
    { id: "s2", name: "Evening", kind: "tidy" as const, tidy: { shells: true, idle: false, compact: false, worktrees: false } },
  ];

  it("hand over their scene and nothing of their own", () => {
    const s = sceneSchedule({ scene: "s1", cron: "0 9 * * 1-5" });
    expect(s).toMatchObject({ kind: "scene", scene: "s1", machine: "", dir: "", prompt: "", enabled: true });
    expect(kindOf(s)).toBe("scene");
  });

  it("say what the scene does, or that it is gone", () => {
    expect(sceneScheduleWords({ scene: "s1" }, scenes)).toBe("starts the scene “Morning”");
    expect(sceneScheduleWords({ scene: "s2" }, scenes)).toBe("tidies with “Evening”, only while no agent works or asks");
    expect(sceneScheduleWords({ scene: "s9" }, scenes)).toBe("a scene that is gone");
  });

  it("say why a tidy waited", () => {
    expect(resultWords({ skipped: "at-work", last_result: "skipped: an agent works or asks on GOG" })).toBe("skipped: an agent works or asks there, so nothing was tidied");
  });
});
