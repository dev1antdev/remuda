import { describe, expect, it } from "vitest";
import type { Repo } from "./api";
import { MAX_RECENT, pickerRows, remember } from "./repos";

const repo = (name: string): Repo => ({ repo: name, folder: `/${name}`, dirs: [`/${name}`], links: [] });
const label = (id: string) => ({ local: "This Mac", gog: "GOG" })[id] ?? id;

describe("the recent repositories", () => {
  it("put the one picked first, once, at most six", () => {
    let recent = [{ machine: "local", repo: "acme/a" }];
    recent = remember(recent, { machine: "local", repo: "acme/b" });
    recent = remember(recent, { machine: "local", repo: "ACME/a" });
    expect(recent.map((r) => r.repo)).toEqual(["ACME/a", "acme/b"]);
    for (let i = 0; i < 10; i++) recent = remember(recent, { machine: "gog", repo: `acme/r${i}` });
    expect(recent).toHaveLength(MAX_RECENT);
  });
});

describe("the picker", () => {
  const base = { picked: null, machine: "local", following: "acme/widgets", recent: [{ machine: "gog", repo: "acme/api" }, { machine: "local", repo: "acme/widgets" }], repos: [repo("acme/widgets"), repo("acme/docs")], filter: "", machineLabel: label };
  it("follows the pane first, then the recent ones, then the machine's", () => {
    const rows = pickerRows(base);
    expect(rows[0]).toEqual({ kind: "follow", hint: "acme/widgets", checked: true });
    expect(rows.map((r) => (r.kind === "heading" ? `# ${r.text}` : r.kind === "repo" ? r.repo : "follow"))).toEqual(["follow", "# Recent", "acme/api", "acme/widgets", "# On This Mac", "acme/docs"]);
    const api = rows.find((r) => r.kind === "repo" && r.repo === "acme/api");
    expect(api).toMatchObject({ machine: "gog", hint: "GOG" });
  });
  it("marks the one picked, and a filter narrows every section", () => {
    const rows = pickerRows({ ...base, picked: { machine: "local", repo: "acme/docs" }, filter: "DOC" });
    expect(rows).toEqual([{ kind: "heading", text: "On This Mac" }, { kind: "repo", machine: "local", repo: "acme/docs", hint: "", checked: true }]);
  });
  it("says when the pane is in no GitHub repository", () => {
    expect(pickerRows({ ...base, following: "" })[0]).toMatchObject({ kind: "follow", hint: "No GitHub repository in the pane's folder" });
  });
});
