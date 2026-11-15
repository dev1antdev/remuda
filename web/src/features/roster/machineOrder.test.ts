import { describe, expect, it } from "vitest";
import { dropMark, inOrder, lowerHalf, movedBeside, sameOrder, stepped } from "./machineOrder";

const ids = ["mac", "gog", "roofr", "windows"];

describe("movedBeside", () => {
  it("puts the dragged machine before or after the one it is dropped on", () => {
    expect(movedBeside(ids, "windows", "mac", false)).toEqual(["windows", "mac", "gog", "roofr"]);
    expect(movedBeside(ids, "mac", "roofr", true)).toEqual(["gog", "roofr", "mac", "windows"]);
    expect(movedBeside(ids, "mac", "roofr", false)).toEqual(["gog", "mac", "roofr", "windows"]);
    expect(movedBeside(ids, "gog", "windows", true)).toEqual(["mac", "roofr", "windows", "gog"]);
  });
  it("leaves the order as it is for a drop on itself or on a machine the list lacks", () => {
    expect(movedBeside(ids, "gog", "gog", true)).toBe(ids);
    expect(movedBeside(ids, "gog", "pi", true)).toBe(ids);
    expect(movedBeside(ids, "pi", "gog", true)).toBe(ids);
  });
  it("names every machine once whatever the drop", () => {
    for (const dragged of ids) for (const target of ids) for (const after of [false, true]) expect([...movedBeside(ids, dragged, target, after)].sort()).toEqual([...ids].sort());
  });
});

describe("stepped", () => {
  it("moves a machine one place up or down", () => {
    expect(stepped(ids, "gog", -1)).toEqual(["gog", "mac", "roofr", "windows"]);
    expect(stepped(ids, "gog", 1)).toEqual(["mac", "roofr", "gog", "windows"]);
  });
  it("stays at either end, and for a machine the list lacks", () => {
    expect(stepped(ids, "mac", -1)).toBe(ids);
    expect(stepped(ids, "windows", 1)).toBe(ids);
    expect(stepped(ids, "pi", 1)).toBe(ids);
  });
});

describe("inOrder", () => {
  const machines = ids.map((id) => ({ id }));
  it("lists the machines as the order names them, the rest after as they came", () => {
    expect(inOrder(machines, ["windows", "gog"]).map((m) => m.id)).toEqual(["windows", "gog", "mac", "roofr"]);
    expect(inOrder(machines, ["roofr", "pi", "mac"]).map((m) => m.id)).toEqual(["roofr", "mac", "gog", "windows"]);
  });
  it("is the machines themselves without an order", () => {
    expect(inOrder(machines, null)).toBe(machines);
  });
});

describe("dropMark", () => {
  it("marks the block the pointer is over on the side the drop lands", () => {
    expect(dropMark(ids, { id: "windows", over: "mac", after: false }, "mac")).toBe("before");
    expect(dropMark(ids, { id: "mac", over: "roofr", after: true }, "roofr")).toBe("after");
    expect(dropMark(ids, { id: "mac", over: "roofr", after: true }, "gog")).toBeNull();
  });
  it("marks nothing on the machine dragged, nor where the drop would change nothing", () => {
    expect(dropMark(ids, { id: "gog", over: "gog", after: true }, "gog")).toBeNull();
    expect(dropMark(ids, { id: "gog", over: "mac", after: true }, "mac")).toBeNull();
    expect(dropMark(ids, { id: "gog", over: "roofr", after: false }, "roofr")).toBeNull();
    expect(dropMark(ids, null, "mac")).toBeNull();
    expect(dropMark(ids, { id: "gog", over: null, after: false }, "mac")).toBeNull();
  });
});

describe("the small ones", () => {
  it("tells the lower half of a box", () => {
    expect(lowerHalf(149, 100, 100)).toBe(false);
    expect(lowerHalf(151, 100, 100)).toBe(true);
  });
  it("tells two orders apart", () => {
    expect(sameOrder(ids, [...ids])).toBe(true);
    expect(sameOrder(ids, ["gog", "mac", "roofr", "windows"])).toBe(false);
    expect(sameOrder(ids, ids.slice(1))).toBe(false);
  });
});
