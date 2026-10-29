import { describe, expect, it } from "vitest";
import { viewerPlaces } from "./viewOnly";

const modes = [{ mode: "herdr" }, { mode: "wall" }, { mode: "tasks" }, { mode: "code" }, { mode: "review" }];
const panels = [{ panel: "standup" }, { panel: "usage" }, { panel: "schedules" }, { panel: "activity" }];

describe("viewerPlaces", () => {
  it("keeps every place for the full page", () => {
    expect(viewerPlaces(modes, false)).toEqual(modes);
    expect(viewerPlaces(panels, false)).toEqual(panels);
  });
  it("keeps the agents, the Wall and the reports for a viewer", () => {
    expect(viewerPlaces(modes, true).map((p) => p.mode)).toEqual(["herdr", "wall"]);
    expect(viewerPlaces(panels, true).map((p) => p.panel)).toEqual(["standup", "usage", "activity"]);
  });
});
