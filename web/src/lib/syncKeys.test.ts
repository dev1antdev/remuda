import { describe, expect, it } from "vitest";
import { isShared, merge, plan } from "./syncKeys";

describe("isShared", () => {
  it("names the keys that travel", () => {
    for (const key of ["theme", "theme.dark", "theme.accent", "radar", "wall", "review.split", "chat.timeline", "footprint", "tasks.view", "standup.period", "code.follow"]) expect(isShared(key), key).toBe(true);
  });
  it("takes every panel width by its prefix, not the bare prefix", () => {
    expect(isShared("panel.sidebar")).toBe(true);
    expect(isShared("panel.tasks.detail")).toBe(true);
    expect(isShared("panel.")).toBe(false);
    expect(isShared("panels")).toBe(false);
  });
  it("takes every folder's pins by their prefix, not the bare prefix", () => {
    expect(isShared("pins:local|/Users/me/remuda")).toBe(true);
    expect(isShared("pins:gog|/home/me/x")).toBe(true);
    expect(isShared("pins:")).toBe(false);
    expect(isShared("pins")).toBe(false);
  });
  it("keeps the device's own keys at home", () => {
    for (const key of ["alerts", "alerts.voice", "alerts.quiet", "prompt.history:local|/x", "feed.seen", "release.seen", "wall.hidden", "collapsed", "wsFolded", "termFont", "text.size", "send.targets", "mode", "prefs.sync", "__stamps", "theme.other"]) expect(isShared(key), key).toBe(false);
    for (const key of ["density", "font.mono"]) expect(isShared(key), key).toBe(true);
  });
});

describe("merge", () => {
  it("keeps the newer stamp and the remote copy on a tie", () => {
    const older = { value: "dark", at: "2026-10-25T10:00:00.000Z" };
    const newer = { value: "light", at: "2026-10-25T11:00:00.000Z" };
    expect(merge(older, newer)).toBe(newer);
    expect(merge(newer, older)).toBe(newer);
    const tie = { value: "system", at: "2026-10-25T11:00:00.000Z" };
    expect(merge(newer, tie)).toBe(tie);
  });
  it("reads a missing stamp as the oldest of all", () => {
    const stampless = { value: "dark", at: "" };
    const remote = { value: "light", at: "2000-01-01T00:00:00.000Z" };
    expect(merge(stampless, remote)).toBe(remote);
  });
});

describe("plan", () => {
  const now = "2026-10-25T12:00:00.000Z";
  it("pushes what only this device has, stamping a stampless one now", () => {
    const out = plan({ theme: { value: "light", at: "" }, "panel.sidebar": { value: 300, at: "2026-10-25T09:00:00.000Z" } }, {}, now);
    expect(out.apply).toEqual({});
    expect(out.push).toEqual({ theme: { value: "light", at: now }, "panel.sidebar": { value: 300, at: "2026-10-25T09:00:00.000Z" } });
  });
  it("applies what only the server has", () => {
    const remote = { wall: { value: { size: "l" }, at: "2026-10-25T09:00:00.000Z" } };
    expect(plan({}, remote, now)).toEqual({ apply: remote, push: {} });
  });
  it("lets the newer side win and leaves a settled key alone", () => {
    const local = { theme: { value: "dark", at: "2026-10-25T11:00:00.000Z" }, radar: { value: { order: "recent" }, at: "2026-10-25T08:00:00.000Z" }, footprint: { value: true, at: "2026-10-25T10:00:00.000Z" } };
    const remote = { theme: { value: "light", at: "2026-10-25T10:00:00.000Z" }, radar: { value: { order: "herdr" }, at: "2026-10-25T09:00:00.000Z" }, footprint: { value: true, at: "2026-10-25T10:00:00.000Z" } };
    const out = plan(local, remote, now);
    expect(out.push).toEqual({ theme: local.theme });
    expect(out.apply).toEqual({ radar: remote.radar });
  });
  it("applies a tie whose value differs, so both sides end the same", () => {
    const out = plan({ theme: { value: "dark", at: "2026-10-25T10:00:00.000Z" } }, { theme: { value: "light", at: "2026-10-25T10:00:00.000Z" } }, now);
    expect(out.apply.theme?.value).toBe("light");
    expect(out.push).toEqual({});
  });
  it("ignores keys that do not travel, from either side", () => {
    const out = plan({ "alerts.voice": { value: { on: true }, at: "" }, termFont: { value: 14, at: now } }, { "prompt.history:x": { value: ["a"], at: now }, evil: { value: 1, at: now } }, now);
    expect(out).toEqual({ apply: {}, push: {} });
  });
});
