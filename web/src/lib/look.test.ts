import { describe, expect, it } from "vitest";
import css from "../styles.css?raw";
import { densityOf, MONO_FONTS, monoName, monoOf, monoStack, SYSTEM_MONO, TEXT_SCALE, TEXT_SIZES, textScaleOf, textSizeOf } from "./look";

// scaleRule is the --text-scale the stylesheet gives a text size through data-text on the root
function scaleRule(size: string): number | null {
  const block = new RegExp(`:root\\[data-text="${size}"\\]\\s*\\{([^}]*)\\}`).exec(css);
  if (!block) return null;
  const m = /--text-scale:\s*([0-9.]+);/.exec(block[1]!);
  return m ? Number(m[1]) : null;
}

describe("the type scale as tokens", () => {
  it("has no literal font size left outside the tokens", () => {
    const literals = css.match(/font-size: *\d/g) ?? [];
    expect(literals).toEqual([]);
    const shorthands = css.match(/\bfont: *\d/g) ?? [];
    expect(shorthands).toEqual([]);
  });

  it("declares every step of the scale times --text-scale", () => {
    for (const step of ["10", "11", "12", "13", "13-5", "15", "18", "20"]) {
      expect(css).toContain(`--fs-${step}: calc(${step.replace("-", ".")}px * var(--text-scale));`);
    }
    expect(css).toContain("--text-scale: 1;");
  });

  it("gives each text size the scale the module says", () => {
    for (const size of TEXT_SIZES) {
      if (size === "normal") continue;
      expect(scaleRule(size)).toBe(TEXT_SCALE[size]);
    }
    expect(textScaleOf("normal")).toBe(1);
  });

  it("tightens the density only with a mouse on a wide screen", () => {
    const compact = /@media \(min-width: 921px\) and \(pointer: fine\)\s*\{\s*:root\[data-density="compact"\]\s*\{([^}]*)\}/.exec(css);
    expect(compact).not.toBeNull();
    expect(compact![1]).toContain("--ctl-tighten: 4px;");
    expect(compact![1]).toContain("--row-h:");
  });
});

describe("the prefs", () => {
  it("keep only known text sizes and densities", () => {
    expect(textSizeOf("large")).toBe("large");
    expect(textSizeOf("huge")).toBe("normal");
    expect(textSizeOf(undefined)).toBe("normal");
    expect(densityOf("compact")).toBe("compact");
    expect(densityOf("dense")).toBe("comfortable");
  });

  it("keep only fonts on offer", () => {
    expect(monoOf("JetBrains Mono")).toBe("JetBrains Mono");
    expect(monoOf("Comic Sans")).toBe("");
    expect(monoOf(3)).toBe("");
    expect(MONO_FONTS.length).toBeGreaterThan(8);
  });
});

describe("the monospace stack", () => {
  it("is the system stack without a pick and the family before it with one", () => {
    expect(monoStack("")).toBe(SYSTEM_MONO);
    expect(monoStack("Fira Code")).toBe(`"Fira Code", ${SYSTEM_MONO}`);
    expect(monoName("")).toBe("System monospace");
    expect(monoName("Menlo")).toBe("Menlo");
  });

  it("is what the stylesheet's own token starts from", () => {
    expect(css).toContain(`--mono: ${SYSTEM_MONO};`);
  });
});
