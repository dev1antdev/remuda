import { describe, expect, it } from "vitest";
import { viewerCommand, viewerMenu, viewerPlaces, viewerToasts } from "./viewOnly";

const modes = [{ mode: "herdr" }, { mode: "tasks" }, { mode: "code" }, { mode: "review" }];
const panels = [{ panel: "usage" }, { panel: "schedules" }, { panel: "activity" }, { panel: "server" }];

describe("viewerPlaces", () => {
  it("keeps every place for the full page", () => {
    expect(viewerPlaces(modes, false)).toEqual(modes);
    expect(viewerPlaces(panels, false)).toEqual(panels);
  });
  it("keeps the agents and the reports for a viewer", () => {
    expect(viewerPlaces(modes, true).map((p) => p.mode)).toEqual(["herdr"]);
    expect(viewerPlaces(panels, true).map((p) => p.panel)).toEqual(["usage", "activity", "server"]);
    expect(viewerCommand("server"), "the palette's Server row, which the rail must match").toBe(true);
  });
});

describe("viewerCommand", () => {
  it("keeps moving about, what a lens shows, the reports and the look", () => {
    for (const id of ["next-needs", "mode:herdr", "lens:chat", "lens:git", "find", "recap", "export", "timelapse", "full", "keys", "theme-dark", "theme-dark-nord", "theme-light-solarized-light", "text-large", "lang-ru", "open-lens-chat"]) expect(viewerCommand(id), id).toBe(true);
  });
  it("drops every command that types, starts, closes, renames or saves on the server", () => {
    for (const id of ["workspace:local", "tab", "worktree", "rename-workspace", "rename-tab", "restart-pane", "close-workspace", "handover", "model", "broadcast", "alerts", "start-recipe", "recipes", "schedules", "settings", "machine", "import-issue", "search", "mode:tasks", "mode:code", "mode:review", "lens:changes"]) expect(viewerCommand(id), id).toBe(false);
  });
});

describe("viewerToasts", () => {
  it("takes the Resume and the install off their toasts and leaves out the Tailscale login, the previews and the plan limits' offers", () => {
    const toasts = [
      { id: "offline" },
      { id: "serves-gog|w1:p1|5173|2026-11-12T13:58:00Z", action: "Preview" },
      { id: "plan-offer-a1b2", action: "Run on Codex" },
      { id: "release", action: "Download & install" },
      { id: "recover-gog", action: "Resume" },
      { id: "tailscale", action: "Log in" },
      { id: "stale", action: "Reload" },
    ];
    expect(viewerToasts(toasts)).toEqual([{ id: "offline" }, { id: "release", action: undefined }, { id: "recover-gog", action: undefined }, { id: "stale", action: "Reload" }]);
  });
});

describe("viewerMenu", () => {
  it("keeps a menu's first group, up to its first separator", () => {
    expect(viewerMenu([{ label: "Open PR #15" }, { sep: true }, { label: "Hand the failed checks to the agent" }])).toEqual([{ label: "Open PR #15" }]);
    expect(viewerMenu([{ label: "Open PR #15" }])).toEqual([{ label: "Open PR #15" }]);
  });
});
