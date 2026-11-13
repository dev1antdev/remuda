import { describe, expect, it } from "vitest";
import { editorOption, onWindows, screenTerminalWords, systemWords } from "./system";
import { encodePath } from "@/features/search/api";

describe("system", () => {
  it("knows a Windows PC by the roster's os, and nothing before the roster says", () => {
    expect(onWindows({ os: "windows" })).toBe(true);
    expect(onWindows({ os: "darwin" })).toBe(false);
    expect(onWindows({})).toBe(false);
    expect(onWindows(null)).toBe(false);
  });
  it("names the system", () => {
    expect(systemWords("windows")).toBe("Windows");
    expect(systemWords("darwin")).toBe("macOS");
    expect(systemWords("linux")).toBe("Linux");
    expect(systemWords(undefined)).toBe("");
  });
  it("lists a Windows PC in the Code mode's pick with the reason, never as a pick", () => {
    const option = { value: "pc", label: "PC" };
    expect(editorOption(option, { os: "windows", host: "pc" })).toMatchObject({ disabled: true, hint: "no code-server for Windows" });
    expect(editorOption(option, { os: "linux", host: "gog" })).toEqual({ value: "pc", label: "PC", hint: "gog" });
  });
  it("says where a live terminal is", () => {
    expect(screenTerminalWords("pc")).toContain("herdr --remote pc");
    expect(screenTerminalWords("")).toContain("herdr --remote <pc>");
  });
  it("matches a search hit's project to a Windows folder as Claude Code names it", () => {
    expect(encodePath("C:\\Users\\me\\app")).toBe("C--Users-me-app");
    expect(encodePath("/Users/me/app")).toBe("-Users-me-app");
  });
});
