import { afterEach, describe, expect, it, vi } from "vitest";
import type { Terminal } from "@xterm/xterm";
import { installCopy, trimRows } from "./copy";
import type { Selection } from "./selection";

describe("trimRows", () => {
  it("drops each row's trailing spaces, tabs and no-break spaces, the rows kept", () => {
    expect(trimRows("a   \nb \t\nc")).toBe("a\nb\nc");
    expect(trimRows("  indented   ")).toBe("  indented");
    expect(trimRows("")).toBe("");
  });
});

type Keys = (ev: KeyboardEvent) => boolean;

// settled waits for the clipboard promise and the onCopied that follows it
const settled = () => new Promise((resolve) => setTimeout(resolve, 0));

// install wires installCopy to a fake terminal and hands back the one key handler xterm would hold
function install({ xtermSelected = false, remembered = "", mac = false }: { xtermSelected?: boolean; remembered?: string; mac?: boolean }) {
  let keys: Keys = () => true;
  const term = {
    attachCustomKeyEventHandler: (handler: Keys) => (keys = handler),
    hasSelection: () => xtermSelected,
    getSelection: () => (xtermSelected ? "on screen" : ""),
  } as unknown as Terminal;
  const mount = { addEventListener: () => {}, removeEventListener: () => {} } as unknown as HTMLElement;
  const layer = { contains: () => false } as unknown as HTMLElement;
  const selection = { active: () => remembered !== "", text: () => remembered } as unknown as Selection;
  const sent: string[] = [];
  const copied: number[] = [];
  installCopy(term, mount, { layer, selection, sendInput: (data) => sent.push(data), onCopied: (n) => copied.push(n), mac });
  return { keys: (ev: Partial<KeyboardEvent>) => keys({ type: "keydown", key: "", ctrlKey: false, metaKey: false, shiftKey: false, altKey: false, code: "", preventDefault: () => {}, ...ev } as KeyboardEvent), sent, copied };
}

describe("installCopy", () => {
  afterEach(() => vi.unstubAllGlobals());
  const stubBrowser = () => {
    vi.stubGlobal("window", { getSelection: () => null });
    vi.stubGlobal("navigator", { clipboard: { writeText: async () => {} } });
  };

  it("lets Ctrl+C through as the interrupt when nothing is selected on screen, a remembered selection scrolled away too", async () => {
    stubBrowser();
    const { keys, copied } = install({});
    expect(keys({ key: "c", ctrlKey: true })).toBe(true);
    const away = install({ remembered: "kept rows" });
    expect(away.keys({ key: "c", ctrlKey: true })).toBe(true);
    expect(away.keys({ key: "C", ctrlKey: true, shiftKey: true })).toBe(false);
    await settled();
    expect(copied).toEqual([]);
    expect(away.copied).toEqual([9]);
  });

  it("copies xterm's own selection on Ctrl+C, and on Cmd+C with a Mac", async () => {
    stubBrowser();
    const linux = install({ xtermSelected: true });
    expect(linux.keys({ key: "c", ctrlKey: true })).toBe(false);
    const mac = install({ xtermSelected: true, mac: true });
    expect(mac.keys({ key: "c", metaKey: true })).toBe(false);
    await settled();
    expect(linux.copied).toEqual([9]);
    expect(mac.copied).toEqual([9]);
  });

  it("keeps the find bar's key from xterm: ⌘F on a Mac, Ctrl+Shift+F elsewhere", () => {
    const mac = install({ mac: true });
    expect(mac.keys({ code: "KeyF", key: "f", metaKey: true })).toBe(false);
    expect(mac.keys({ code: "KeyF", key: "f", ctrlKey: true, shiftKey: true })).toBe(true);
    const linux = install({});
    expect(linux.keys({ code: "KeyF", key: "F", ctrlKey: true, shiftKey: true })).toBe(false);
    expect(linux.keys({ code: "KeyF", key: "f", metaKey: true })).toBe(true);
    expect(linux.keys({ code: "KeyF", key: "f" })).toBe(true);
  });

  it("sends Shift+Enter as Alt+Enter and swallows every phase of it", () => {
    const { keys, sent } = install({});
    expect(keys({ key: "Enter", shiftKey: true })).toBe(false);
    expect(keys({ type: "keyup", key: "Enter", shiftKey: true })).toBe(false);
    expect(sent).toEqual(["\x1b\r"]);
  });
});
