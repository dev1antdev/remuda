import { describe, expect, it } from "vitest";
import type { Health } from "./api";
import { concerns, updateLabel } from "./concerns";

const health = (over: Partial<Health>): Health => ({ at: "2026-10-13T10:00:00Z", disk_free_gb: 100, disk_used_pct: 40, load1: 0.5, ...over }) as Health;
const keys = (list: ReturnType<typeof concerns>) => list.map((c) => c.key);

describe("concerns", () => {
  it("is empty without vitals, and when everything is current", () => {
    expect(concerns(undefined, undefined, "v0.5.0", false)).toEqual([]);
    expect(concerns(health({ claude: "2.1.0", herdr: "0.9.1" }), { at: "", claude: "2.1.0", herdr: "0.9.1" }, "v0.5.0", false)).toEqual([]);
  });
  it("offers an update per agent and for herdr, and a restart for a lagging herdr server", () => {
    const h = health({ claude: "2.1.0", codex: "0.40.0", herdr: "0.9.1", herdr_server: "0.9.0" });
    const list = concerns(h, { at: "", claude: "2.2.0", codex: "0.41.0", herdr: "0.9.1" }, "v0.5.0", false);
    expect(keys(list)).toEqual(["claude", "codex", "herdr-server"]);
    expect(list.map((c) => c.update)).toEqual(["claude", "codex", "restart-herdr"]);
  });
  it("asks for herdr's hooks to be installed again when the installed herdr writes newer ones", () => {
    const h = health({ herdr: "0.9.5", outdated_hooks: [{ agent: "claude", have: "v10", want: "v11" }, { agent: "codex", have: "v8", want: "v10" }, { agent: "copilot" }] });
    expect(concerns(h, { at: "", herdr: "0.9.5" }, "v0.6.0", false)).toEqual([{ key: "hooks", text: "herdr's hooks for Claude Code, Codex and Copilot are outdated", update: "hooks" }]);
    expect(concerns(health({ herdr: "0.9.5", outdated_hooks: [{ agent: "qodercli" }] }), undefined, "v0.6.0", false)[0]?.text).toBe("herdr's hooks for Qoder are outdated");
    expect(concerns(health({ herdr: "0.9.5", outdated_hooks: [], plugins: [{ id: "herdr.push", source: "x/y" }] }), undefined, "v0.6.0", false)).toEqual([]);
  });
  it("compares a remote's remuda with this one, and both with the newest release", () => {
    const remote = concerns(health({ remuda: "v0.4.0" }), { at: "", remuda: "v0.5.0" }, "v0.5.0", true);
    expect(keys(remote)).toEqual(["ui", "release"]);
    expect(keys(concerns(health({}), undefined, "v0.5.0", true))).toEqual(["ui"]);
    expect(keys(concerns(health({}), { at: "", remuda: "v0.6.0" }, "v0.5.0", false))).toEqual(["release"]);
  });
  it("flags a full disk, bad from 95%", () => {
    expect(concerns(health({ disk_used_pct: 91 }), undefined, undefined, false)).toMatchObject([{ key: "disk", bad: false }]);
    expect(concerns(health({ disk_used_pct: 96 }), undefined, undefined, false)).toMatchObject([{ key: "disk", bad: true }]);
    expect(keys(concerns(health({ disk_free_gb: 4 }), undefined, undefined, false))).toEqual(["disk"]);
  });
  it("shows a read that failed outright as bad", () => {
    expect(concerns(health({ error: "ssh: timeout" }), undefined, undefined, true)).toMatchObject([{ key: "err", bad: true }]);
  });
  it("labels the button by what the update does", () => {
    expect(updateLabel("claude")).toBe("Update");
    expect(updateLabel("remuda")).toBe("Deploy");
    expect(updateLabel("remuda-release")).toBe("Download & install");
    expect(updateLabel("restart-herdr")).toBe("Restart");
  });
});
