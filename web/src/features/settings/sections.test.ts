import { describe, expect, it } from "vitest";
import { listedIds, SECTION_IDS, sectionGroups, shownSection, stepSection } from "./sections";

describe("sectionGroups", () => {
  it("lists every section once, this device's before the cockpit's", () => {
    const groups = sectionGroups(false, true);
    expect(groups.map((group) => group.id)).toEqual(["device", "cockpit"]);
    expect([...listedIds(groups)].sort()).toEqual([...SECTION_IDS].sort());
    expect(listedIds(groups).slice(0, 3)).toEqual(["look", "sidebar", "general"]);
  });
  it("leaves a viewer this device's sections alone", () => {
    expect(listedIds(sectionGroups(true))).toEqual(["look", "sidebar", "general"]);
  });
  it("names and marks every section", () => {
    for (const group of sectionGroups(false)) {
      expect(group.label).not.toBe("");
      for (const section of group.sections) {
        expect(section.label, section.id).not.toBe("");
        expect(section.icon, section.id).not.toBe("");
      }
    }
  });
});

describe("shownSection", () => {
  const groups = sectionGroups(false);
  it("draws the section asked for while the list has it", () => {
    expect(shownSection(groups, "backups")).toBe("backups");
  });
  it("falls back to the first for a section nobody lists, or a kept value of another shape", () => {
    expect(shownSection(sectionGroups(true), "backups")).toBe("look");
    expect(shownSection(groups, "nowhere")).toBe("look");
    expect(shownSection(groups, 7)).toBe("look");
    expect(shownSection(groups, undefined)).toBe("look");
  });
});

describe("stepSection", () => {
  const ids = listedIds(sectionGroups(false));
  it("steps either way with the arrows, round and round", () => {
    expect(stepSection(ids, "look", "ArrowDown")).toBe("sidebar");
    expect(stepSection(ids, "sidebar", "ArrowUp")).toBe("look");
    expect(stepSection(ids, "look", "ArrowRight")).toBe("sidebar");
    expect(stepSection(ids, "sidebar", "ArrowLeft")).toBe("look");
    expect(stepSection(ids, "look", "ArrowUp")).toBe("devices");
    expect(stepSection(ids, "devices", "ArrowDown")).toBe("look");
  });
  it("goes to the ends with Home and End", () => {
    expect(stepSection(ids, "jira", "Home")).toBe("look");
    expect(stepSection(ids, "jira", "End")).toBe("devices");
  });
  it("leaves any other key, and a section the list lacks, alone", () => {
    expect(stepSection(ids, "jira", "Enter")).toBeNull();
    expect(stepSection(listedIds(sectionGroups(true)), "jira", "ArrowDown")).toBeNull();
  });
});
