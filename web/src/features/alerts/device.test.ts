import { describe, expect, it } from "vitest";
import { blockedHelp, browserOf, deliveryWords, receivedWords, systemHelp, systemOf } from "./device";

const UA = {
  firefoxMac: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:159.0) Gecko/20100101 Firefox/159.0",
  chromeWin: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36",
  edgeWin: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36 Edg/141.0.0.0",
  safariMac: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/19.0 Safari/605.1.15",
  iphone: "Mozilla/5.0 (iPhone; CPU iPhone OS 19_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/19.0 Mobile/15E148 Safari/604.1",
  linux: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36",
};

describe("browserOf and systemOf", () => {
  it("tells the browsers apart, Edge and Chrome before Safari", () => {
    expect(browserOf(UA.firefoxMac)).toBe("Firefox");
    expect(browserOf(UA.chromeWin)).toBe("Chrome");
    expect(browserOf(UA.edgeWin)).toBe("Edge");
    expect(browserOf(UA.safariMac)).toBe("Safari");
    expect(browserOf("curl/8")).toBe("browser");
  });
  it("tells the systems apart, an iPad that says Mac by its touch points", () => {
    expect(systemOf(UA.firefoxMac)).toBe("macOS");
    expect(systemOf(UA.safariMac, 5)).toBe("iOS");
    expect(systemOf(UA.iphone)).toBe("iOS");
    expect(systemOf(UA.chromeWin)).toBe("Windows");
    expect(systemOf(UA.linux)).toBe("Linux");
  });
});

describe("the help", () => {
  it("names where the browser and the system let notifications through", () => {
    expect(blockedHelp("Firefox")).toMatch(/Permissions → Send notifications → Allow/);
    expect(blockedHelp("Edge")).toMatch(/^Edge blocks/);
    expect(systemHelp("Firefox", "macOS")).toMatch(/System Settings → Notifications → Firefox/);
    expect(systemHelp("Chrome", "Windows")).toMatch(/Settings → System → Notifications: Chrome on/);
    expect(systemHelp("browser", "Linux")).toMatch(/let the browser show them/);
  });
});

describe("the last push sent and received", () => {
  const now = Date.parse("2026-11-05T12:10:00Z");
  it("says when, what, and where it stopped", () => {
    expect(deliveryWords(null, now)).toBe("No push sent to this browser since remuda started.");
    expect(deliveryWords({ at: "2026-11-05T12:05:00Z", kind: "input", title: "api needs you" }, now)).toBe("Last push sent 5 min ago: api needs you — the push service took it.");
    expect(deliveryWords({ at: "2026-11-05T12:05:00Z", kind: "input", title: "api needs you", error: "push service answered 403" }, now)).toMatch(/refused: push service answered 403$/);
    expect(receivedWords(undefined, now)).toBe("No push received here yet.");
    expect(receivedWords({ at: now - 120_000, title: "web finished", kind: "done", shown: "os" }, now)).toBe("Last push received here 2 min ago: web finished — handed to the system to show.");
    expect(receivedWords({ at: now, title: "web finished", kind: "done", shown: "page" }, now)).toMatch(/shown on the page/);
  });
});
