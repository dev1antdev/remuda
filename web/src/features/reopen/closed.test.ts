import { describe, expect, it } from "vitest";
import type { Closed } from "./api";
import { closedTitle, closedWhere, justClosed, offerWords, reopenKeyAsked, type KeyPress } from "./closed";

const closed = (over: Partial<Closed>): Closed => ({ id: "x", machine: "local", pane: "w1:p1", agent: "claude", session: "s", dir: "/work/api", title: "Fix the queue", at: "2026-11-04T09:00:00Z", ...over });

describe("a closed agent's words", () => {
  it("is named by its conversation, else its agent", () => {
    expect(closedTitle(closed({}))).toBe("Fix the queue");
    expect(closedTitle(closed({ title: " " }))).toBe("Claude Code");
    expect(closedTitle(closed({ title: "", agent: "zzz" }))).toBe("zzz");
  });

  it("says where it lived, the machine only when it is not this one", () => {
    expect(closedWhere(closed({ tab: "review", workspace: "api" }), "This Mac")).toBe("review · api");
    expect(closedWhere(closed({ workspace: "api", machine: "gog" }), "GOG")).toBe("api · GOG");
  });

  it("finds the entries a close made", () => {
    const list = [closed({ id: "a", pane: "w1:p1" }), closed({ id: "b", pane: "w1:p2", machine: "gog" }), closed({ id: "c", pane: "w1:p3" })];
    expect(justClosed(list, "local", ["w1:p1", "w1:p2"]).map((c) => c.id)).toEqual(["a"]);
    expect(justClosed(list, "local", [])).toEqual([]);
  });

  it("puts the toast in a line", () => {
    expect(offerWords([closed({})])).toBe("Closed Fix the queue");
    expect(offerWords([closed({}), closed({ id: "y" })])).toBe("Closed 2 agents");
  });
});

describe("reopenKeyAsked", () => {
  const press = (over: Partial<KeyPress> = {}): KeyPress => ({ code: "KeyZ", altKey: true, metaKey: false, ctrlKey: false, shiftKey: false, target: { closest: () => null }, ...over });
  const inside = (selector: string): KeyPress["target"] => ({ closest: (s: string) => (s === selector ? {} : null) });

  it("is Alt+Z by key code and nothing more", () => {
    for (const mac of [true, false]) {
      expect(reopenKeyAsked(press(), false, mac)).toBe(true);
      expect(reopenKeyAsked(press({ code: "KeyY" }), false, mac)).toBe(false);
      expect(reopenKeyAsked(press({ altKey: false }), false, mac)).toBe(false);
      expect(reopenKeyAsked(press({ shiftKey: true }), false, mac)).toBe(false);
      expect(reopenKeyAsked(press({ metaKey: true }), false, mac)).toBe(false);
      expect(reopenKeyAsked(press(), true, mac)).toBe(false);
      expect(reopenKeyAsked(press({ target: inside(".xterm") }), false, mac)).toBe(false);
    }
  });

  it("leaves a field its Option+Z on a Mac", () => {
    const field = press({ target: inside("input, textarea, select, [contenteditable]") });
    expect(reopenKeyAsked(field, false, true)).toBe(false);
    expect(reopenKeyAsked(field, false, false)).toBe(true);
  });
});
