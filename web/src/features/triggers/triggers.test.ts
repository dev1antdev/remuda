import { describe, expect, it } from "vitest";
import type { Machine } from "@/features/roster/api";
import type { Rule } from "./api";
import { draftCheck, looksAt, MAX_SEEN, newRule, offerKey, offersToShow, offerWords, rememberOffer, ruleWords, scopeOf, withScope } from "./triggers";

const rule = (more: Partial<Rule>): Rule => ({ name: "Deployed", match: "substring", pattern: "DEPLOYED", action: "push", enabled: true, ...more });
const label = (id: string) => (id === "gog" ? "GOG" : id);

describe("triggers", () => {
  it("says a rule in a line", () => {
    expect(ruleWords(rule({}), label)).toBe("“DEPLOYED” · everywhere · push");
    expect(ruleWords(rule({ match: "regex", pattern: "ready in \\d+ms", machine: "gog", dir: "/home/me/api", action: "task", column: "Testing", enabled: false }), label)).toBe(
      "/ready in \\d+ms/ · in /home/me/api on GOG · task → Testing · off",
    );
    expect(ruleWords(rule({ machine: "gog", action: "feed" }), label)).toBe("“DEPLOYED” · everywhere on GOG · activity");
  });

  it("knows where a rule looks", () => {
    expect(scopeOf(rule({}))).toBe("everywhere");
    expect(scopeOf(rule({ machine: "gog" }))).toBe("machine");
    expect(scopeOf(rule({ machine: "gog", dir: "/w" }))).toBe("folder");
    const folder = rule({ machine: "gog", dir: "/home/me/api" });
    expect(looksAt(folder, "gog", "/home/me/api")).toBe(true);
    expect(looksAt(folder, "gog", "/home/me/api/web")).toBe(true);
    expect(looksAt(folder, "gog", "/home/me/api-v2")).toBe(false);
    expect(looksAt(folder, "local", "/home/me/api")).toBe(false);
    expect(looksAt(rule({}), "local", "")).toBe(true);
    expect(looksAt(rule({ machine: "gog" }), "gog", "/anything")).toBe(true);
  });

  it("moves a rule between scopes", () => {
    const fresh = newRule("gog", "/home/me/api");
    expect(fresh).toMatchObject({ machine: "gog", dir: "/home/me/api", enabled: true });
    expect(newRule()).toMatchObject({ machine: "", dir: "" });
    expect(withScope(fresh, "everywhere", "gog", "/home/me/api")).toMatchObject({ machine: "", dir: "" });
    expect(withScope(fresh, "machine", "gog", "/home/me/api")).toMatchObject({ machine: "gog", dir: "" });
    expect(withScope(newRule(), "folder", "gog", "/home/me/api")).toMatchObject({ machine: "gog", dir: "/home/me/api" });
  });

  it("checks the form before it saves", () => {
    expect(draftCheck(rule({}))).toBe("");
    expect(draftCheck(rule({ name: " " }))).toBe("Name the trigger.");
    expect(draftCheck(rule({ pattern: "" }))).toBe("Say what to look for.");
    expect(draftCheck(rule({ pattern: "a\nb" }))).toContain("one line at a time");
    expect(draftCheck(rule({ action: "task" }))).toBe("Name the column the task moves to.");
    expect(draftCheck(rule({ machine: "gog", dir: "src" }))).toBe("The folder is an absolute path.");
  });

  it("offers each fresh dev server once", () => {
    const now = Date.parse("2026-11-12T14:00:00Z");
    const pane = (id: string, at?: string) => ({ id, title: "", agent: "", cwd: "/w", focused: false, status: "shell", cols: 80, rows: 24, ...(at ? { serves: { port: 5173, label: "vite", at } } : {}) });
    const machines = [
      { id: "gog", label: "GOG", state: "connected", sessions: [{ id: "w1", name: "web", tabs: [{ id: "w1:t1", panes: [pane("w1:p1", "2026-11-12T13:58:00Z"), pane("w1:p2", "2026-11-12T13:00:00Z"), pane("w1:p3")] }] }] },
      { id: "away", label: "Away", state: "error", sessions: [{ id: "w1", name: "api", tabs: [{ id: "w1:t1", panes: [pane("w1:p1", "2026-11-12T13:59:00Z")] }] }] },
    ] as unknown as Machine[];
    const shown = offersToShow(machines, [], now);
    expect(shown.map((o) => o.key)).toEqual(["gog|w1:p1|5173|2026-11-12T13:58:00Z"]);
    expect(offerWords(shown[0]!)).toBe("web serves on :5173 (vite)");
    expect(offersToShow(machines, rememberOffer([], shown[0]!.key), now)).toEqual([]);
    expect(offerKey("gog", "w1:p1", { port: 3000, at: "x" })).toBe("gog|w1:p1|3000|x");
    let seen: string[] = [];
    for (let i = 0; i < MAX_SEEN + 5; i++) seen = rememberOffer(seen, `k${i}`);
    expect(seen.length).toBe(MAX_SEEN);
    expect(seen.at(-1)).toBe(`k${MAX_SEEN + 4}`);
  });
});
