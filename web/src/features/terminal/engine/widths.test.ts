import { describe, expect, it } from "vitest";
import type { Terminal, IUnicodeVersionProvider } from "@xterm/xterm";
import { installWidths } from "./widths";

// The width xterm gives a string, as its own UnicodeService counts it: each codepoint's width, less the width of the cell
// it joins.
function cells(provider: IUnicodeVersionProvider, text: string): number {
  let total = 0;
  let preceding = 0;
  for (const char of text) {
    const info = provider.charProperties(char.codePointAt(0) ?? 0, preceding);
    const width = (info >> 1) & 0b11;
    const joins = info & 1;
    total += width - (joins ? (preceding >> 1) & 0b11 : 0);
    preceding = info;
  }
  return total;
}

function widths(): IUnicodeVersionProvider {
  let active: IUnicodeVersionProvider | undefined;
  const term = {
    unicode: {
      register: (provider: IUnicodeVersionProvider) => (active = provider),
      set activeVersion(_: string) {},
    },
  } as unknown as Terminal;
  installWidths(term);
  if (!active) {
    throw new Error("installWidths registered no provider");
  }
  return active;
}

describe("the core's cell widths", () => {
  it("counts an emoji with a variation selector as the two cells the core does", () => {
    const provider = widths();
    expect(provider.version).toBe("11");
    expect(cells(provider, "✍️")).toBe(2);
    expect(cells(provider, "❤️")).toBe(2);
  });

  it("keeps the tables' widths for everything else", () => {
    const provider = widths();
    expect(cells(provider, "✍")).toBe(1);
    expect(cells(provider, "😀")).toBe(2);
    expect(cells(provider, "a")).toBe(1);
    expect(cells(provider, "日本")).toBe(4);
  });

  it("does not widen a wide emoji that a variation selector follows", () => {
    expect(cells(widths(), "😀️")).toBe(2);
  });
});
