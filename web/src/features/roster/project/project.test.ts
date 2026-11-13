import { describe, expect, it } from "vitest";
import { actionHint, actionProblem, primary, sameSettings, setupLine, whenWords, withAction, withoutAction, type Action } from "./project";

const dev: Action = { id: "a1", name: "Dev", command: "npm run dev", place: "tab" };
const test: Action = { id: "a2", name: "Test", command: "go test ./...", folder: "server", place: "down", icon: "circle-check" };

describe("the run actions", () => {
  it("runs the last one used, else the first", () => {
    expect(primary([dev, test], "a2")).toBe(test);
    expect(primary([dev, test], "gone")).toBe(dev);
    expect(primary([], "a1")).toBeUndefined();
  });
  it("edits the list in place", () => {
    expect(withAction([dev], { ...dev, name: "Serve" }).map((a) => a.name)).toEqual(["Serve"]);
    expect(withAction([dev], { ...test, id: "" }).length).toBe(2);
    expect(withoutAction([dev, test], "a1")).toEqual([test]);
  });
  it("says where an action runs", () => {
    expect(actionHint(dev)).toBe("npm run dev · a new tab");
    expect(actionHint(test)).toBe("go test ./... · server/ · split down");
  });
  it("says why an action cannot be saved", () => {
    expect(actionProblem({ name: "", command: "x" })).toBe("Name the action.");
    expect(actionProblem({ name: "Dev", command: " " })).toBe("Give the command it runs.");
    expect(actionProblem({ name: "Dev", command: "a\nb" })).toBe("One line: join commands with && or put them in a script.");
    expect(actionProblem({ name: "Dev", command: "make", folder: "../x" })).toBe("A folder inside the checkout, like web.");
    expect(actionProblem({ name: "Dev", command: "make", folder: "web" })).toBe("");
  });
});

describe("the setup", () => {
  it("is said on one line, the team's first", () => {
    expect(setupLine("pnpm i\n", "ln -s a b\n\ncp x y")).toBe("pnpm i · ln -s a b · cp x y");
    expect(setupLine(undefined, " ")).toBe("");
    expect(whenWords("ask")).toBe("Ask every time");
    expect(whenWords("")).toBe("Run by default");
  });
  it("tells an edit that changed nothing", () => {
    expect(sameSettings({ base: "", actions: [] }, {})).toBe(true);
    expect(sameSettings({ base: "main" }, {})).toBe(false);
  });
});
