import { describe, expect, it } from "vitest";
import { ACCENTS } from "@/lib/theme";
import { iconURL, initialOf, tileColour } from "./folderFace";

describe("a folder's tile", () => {
  it("takes the name's first letter or digit", () => {
    expect(initialOf("games-db")).toBe("G");
    expect(initialOf(".dotfiles")).toBe("D");
    expect(initialOf("2026-plans")).toBe("2");
    expect(initialOf("ёлка")).toBe("Ё");
    expect(initialOf("---")).toBe("?");
    expect(initialOf("")).toBe("?");
  });

  it("keeps one of the accents for a name, the same every time", () => {
    expect(ACCENTS).toContain(tileColour("user-profiles"));
    expect(tileColour("user-profiles")).toBe(tileColour("user-profiles"));
    const spread = new Set(["api", "web", "games-db", "patrons", "sections", "roofr", "remuda"].map(tileColour));
    expect(spread.size).toBeGreaterThan(2);
  });

  it("reads a picture from the icon route", () => {
    expect(iconURL("ab12")).toBe("/api/icons/ab12");
  });
});
