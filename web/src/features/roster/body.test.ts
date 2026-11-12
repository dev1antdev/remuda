import { describe, expect, it } from "vitest";
import type { Health } from "./api";
import { batteryLow, batteryWords, bodyConcerns, clockWords, memUsedPct, memWords, tempWords, throttleWords } from "./body";
import { concerns, hostConcerns, serverTone } from "./concerns";
import { hostMachine, startOn } from "./server";

// pi is rp-orange as its vitals read it: Ubuntu on a Raspberry Pi 4, the firmware's word unreadable without root
const pi: Health = {
  at: "2026-11-05T10:00:00Z",
  disk_free_gb: 107,
  disk_used_pct: 4,
  load1: 0.42,
  cores: 4,
  model: "Raspberry Pi 4 Model B Rev 1.5",
  mem_total_mb: 7798,
  mem_avail_mb: 6657,
  temp_c: 62.8,
  temp_crit_c: 110,
  freq_mhz: 1300,
  freq_max_mhz: 1800,
  throttle: { now: [] },
};

describe("a machine's body", () => {
  it("says the memory, the temperature and the clock", () => {
    expect(memWords(pi)).toBe("1.1 of 7.6 GB used · 6.5 GB available");
    expect(memWords({ ...pi, swap_total_mb: 1024, swap_used_mb: 282 })).toBe("1.1 of 7.6 GB used · 6.5 GB available · swap 0.3 of 1.0 GB");
    expect(memWords({ ...pi, mem_total_mb: 32768, mem_avail_mb: 10261 })).toBe("22 of 32 GB used · 10 GB available");
    expect(memUsedPct(pi)).toBe(14.6);
    expect(tempWords(pi)).toBe("62.8 °C · critical at 110 °C");
    expect(clockWords(pi)).toBe("1.3 of 1.8 GHz");
    expect(memWords({ ...pi, mem_total_mb: undefined }) + tempWords({ ...pi, temp_c: undefined }) + clockWords({ ...pi, freq_mhz: undefined })).toBe("");
  });
  it("says what the firmware held back, and when only the alarm was readable", () => {
    expect(throttleWords({ now: [] })).toBe("none now · under-voltage alarm only: the firmware's word needs root here (vcgencmd)");
    expect(throttleWords({ now: ["under-voltage", "throttled"], since: ["under-voltage", "throttled"], word: "0x50005" })).toBe("now: under-voltage, throttled · since boot: under-voltage, throttled");
    expect(throttleWords({ now: [], word: "0x0" })).toBe("none now · none since boot");
  });
  it("is well when nothing is held back, warm or tight", () => {
    expect(bodyConcerns(pi)).toEqual([]);
    expect(serverTone(pi)).toBe("");
  });
  it("calls under-voltage and heat bad, a capped clock, warmth and tight memory a look", () => {
    expect(serverTone({ ...pi, throttle: { now: ["under-voltage"] } })).toBe("bad");
    expect(serverTone({ ...pi, temp_c: 86 })).toBe("bad");
    expect(serverTone({ ...pi, temp_c: 78 })).toBe("warn");
    expect(serverTone({ ...pi, throttle: { now: [], since: ["throttled"], word: "0x40000" } })).toBe("warn");
    expect(serverTone({ ...pi, mem_avail_mb: 500 })).toBe("warn");
    expect(serverTone({ ...pi, mem_avail_mb: 200 })).toBe("bad");
    expect(serverTone({ ...pi, disk_used_pct: 96 })).toBe("bad");
    expect(serverTone(undefined)).toBe("");
  });
  it("puts the body's concerns in a machine's ⓘ beside the versions", () => {
    const list = concerns({ ...pi, temp_c: 88 }, undefined, undefined, true);
    expect(list.map((c) => [c.key, c.bad])).toEqual([["body-0", true]]);
  });
  it("lists for a server's host its release and its body, never the herdr still installed there", () => {
    const leftHerdr: Health = { ...pi, herdr: "0.9.5", herdr_server: "0.9.4", outdated_hooks: [{ agent: "claude", have: "1", want: "2" }], temp_c: 78 };
    const latest = { at: "2026-11-05T10:00:00Z", herdr: "0.9.6", remuda: "v0.12.6" };
    const keys = hostConcerns(leftHerdr, latest, "v0.12.5").map((c) => c.key);
    expect(keys).toEqual(["release", "body-0"]);
    expect(concerns(leftHerdr, latest, "v0.12.5", false).map((c) => c.key)).toContain("herdr");
    expect(hostConcerns(leftHerdr, latest, "v0.12.5", true).every((c) => !c.update)).toBe(true);
  });
});

describe("a machine's battery", () => {
  it("says how full it is and whether the machine runs on it", () => {
    expect(batteryWords({ percent: 85, discharging: true })).toBe("85 % · on battery");
    expect(batteryWords({ percent: 92, charging: true })).toBe("92 % · charging");
    expect(batteryWords({ percent: 100 })).toBe("100 % · on power");
  });
  it("is low on its own power from 20 % down, never on the charger", () => {
    expect(batteryLow({ percent: 20, discharging: true })).toBe(true);
    expect(batteryLow({ percent: 21, discharging: true })).toBe(false);
    expect(batteryLow({ percent: 5, charging: true })).toBe(false);
  });
});

describe("a server's host", () => {
  it("has the machine's shape the Updates dialog takes, always connected and with no workspaces", () => {
    expect(hostMachine({ id: "local", label: "This machine · rp-orange", health: pi })).toEqual({ id: "local", label: "This machine · rp-orange", state: "connected", sessions: [], health: pi });
  });
  it("starts an agent on this machine when it is one, else the first connected", () => {
    const remote = (id: string, state: "connected" | "error") => ({ id, label: id, state, sessions: [] });
    expect(startOn([remote("local", "connected"), remote("gog", "connected")])).toBe("local");
    expect(startOn([remote("mac", "error"), remote("gog", "connected")])).toBe("gog");
    expect(startOn([remote("mac", "error")])).toBe("mac");
    expect(startOn([])).toBe("local");
  });
});
