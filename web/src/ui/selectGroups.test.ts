import { describe, expect, it } from "vitest";
import { groupsOf, shownOrder } from "./selectGroups";

const voice = (value: string, group: string, disabled = false) => ({ value, label: value, group, disabled });

describe("a Select's groups", () => {
  // the browser lists its voices by name, the alerts panel groups them by language
  const voices = [voice("Albert", "en-US"), voice("Alice", "it-IT"), voice("Amélie", "fr-CA"), voice("Daniel", "en-GB"), voice("Fred", "en-US"), voice("Luca", "it-IT", true)];

  it("gather the options under their group, the groups in the order they first come", () => {
    expect(groupsOf(voices).map((g) => [g.name, g.items.map((o) => o.value)])).toEqual([
      ["en-US", ["Albert", "Fred"]],
      ["it-IT", ["Alice", "Luca"]],
      ["fr-CA", ["Amélie"]],
      ["en-GB", ["Daniel"]],
    ]);
  });
  it("walk the enabled options in the order they are drawn, so the keys and the pointer land on the option under them", () => {
    const enabled = shownOrder(groupsOf(voices)).filter((o) => !o.disabled);
    expect(enabled.map((o) => o.value)).toEqual(["Albert", "Fred", "Alice", "Amélie", "Daniel"]);
  });
  it("keep options without a group in one run, as given", () => {
    const plain: { value: number; label: string; group?: string }[] = [{ value: 1, label: "one" }, { value: 2, label: "two" }];
    expect(shownOrder(groupsOf(plain)).map((o) => o.value)).toEqual([1, 2]);
  });
});
