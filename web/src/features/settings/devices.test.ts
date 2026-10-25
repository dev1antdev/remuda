import { describe, expect, it } from "vitest";
import { deviceLine, revokeQuestion } from "./devices";

const now = Date.parse("2026-10-25T12:00:00Z");
const device = { name: "Chrome on macOS", kind: "Chrome on macOS", last_seen: "2026-10-25T09:00:00Z", ip: "192.168.1.4", this: false };

describe("deviceLine", () => {
  it("says when a device was seen and where from", () => {
    expect(deviceLine(device, now)).toBe("seen 3 h ago · from 192.168.1.4");
  });
  it("names the kind when the person renamed the device", () => {
    expect(deviceLine({ ...device, name: "the laptop" }, now)).toBe("Chrome on macOS · seen 3 h ago · from 192.168.1.4");
  });
  it("leaves out an unknown address", () => {
    expect(deviceLine({ ...device, ip: undefined, last_seen: new Date(now - 10_000).toISOString() }, now)).toBe("seen just now");
  });
});

describe("revokeQuestion", () => {
  it("asks about signing this very device out", () => {
    expect(revokeQuestion({ name: "x", this: true })).toEqual({ text: "Sign this device out? The token is needed to sign in again.", label: "Sign out" });
  });
  it("names another device", () => {
    expect(revokeQuestion({ name: "the phone", this: false })).toEqual({ text: "Sign “the phone” out? It needs the token to sign in again.", label: "Revoke" });
  });
});
