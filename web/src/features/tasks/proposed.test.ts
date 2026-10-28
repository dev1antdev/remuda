import { describe, expect, it } from "vitest";
import type { Project, Status } from "./api";
import { acceptColumn, proposedTotal, proposedWords } from "./proposed";

const status = (id: number, title: string, flags: Partial<Status> = {}): Status => ({
  id,
  project_id: 1,
  title,
  hex_color: "",
  kind: "open",
  position: id,
  on_new: false,
  on_start: false,
  on_finish: false,
  on_agent: false,
  ...flags,
});
const project = (statuses: Status[], proposed = 0, is_archived = false): Project => ({
  id: 1,
  title: "p",
  description: "",
  hex_color: "",
  parent_project_id: 0,
  is_archived,
  is_favorite: false,
  position: 1,
  created: "",
  updated: "",
  statuses,
  fields: [],
  proposed,
});

describe("acceptColumn", () => {
  it("is the new-task column", () => {
    const p = project([status(1, "Proposed", { on_agent: true }), status(2, "New", { on_new: true }), status(3, "Done", { kind: "done" })]);
    expect(acceptColumn(p)?.title).toBe("New");
  });
  it("falls back to the first open column that is not the proposals' own", () => {
    const p = project([status(1, "Proposed", { on_agent: true }), status(2, "Doing"), status(3, "Done", { kind: "done" })]);
    expect(acceptColumn(p)?.title).toBe("Doing");
  });
  it("is nothing without a project or a column to go to", () => {
    expect(acceptColumn(undefined)).toBeUndefined();
    expect(acceptColumn(project([status(1, "Proposed", { on_agent: true }), status(3, "Done", { kind: "done" })]))).toBeUndefined();
  });
});

describe("proposedTotal", () => {
  it("sums the projects' counts, an archived project left out", () => {
    expect(proposedTotal([project([], 3), project([], 2), project([], 9, true)])).toBe(5);
    expect(proposedTotal([])).toBe(0);
  });
});

describe("proposedWords", () => {
  it("says how many wait", () => {
    expect(proposedWords(1)).toBe("1 task proposed by an agent");
    expect(proposedWords(12)).toBe("12 tasks proposed by agents");
    expect(proposedWords(0)).toBe("0 tasks proposed by agents");
  });
});
